package scope

import (
	"context"
	"fmt"
	"path"
	"sort"
	"strings"

	gcpcloud "github.com/GoogleCloudPlatform/k8s-cloud-provider/pkg/cloud"
	"github.com/GoogleCloudPlatform/k8s-cloud-provider/pkg/cloud/meta"
	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	"golang.org/x/mod/semver"
	"google.golang.org/api/compute/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2/klogr"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/cluster-api-provider-gcp/util/hash"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	expclusterv1 "sigs.k8s.io/cluster-api/exp/api/v1beta1"
	"sigs.k8s.io/cluster-api/util/patch"
	"sigs.k8s.io/controller-runtime/pkg/client"

	infrav1 "sigs.k8s.io/cluster-api-provider-gcp/api/v1beta1"
	"sigs.k8s.io/cluster-api-provider-gcp/cloud"
	expinfrav1 "sigs.k8s.io/cluster-api-provider-gcp/exp/api/v1beta1"
)

type MachinePoolScope struct {
	logr.Logger

	client      client.Client
	patchHelper *patch.Helper

	Cluster        *clusterv1.Cluster
	ClusterGetter  cloud.ClusterGetter
	MachinePool    *expclusterv1.MachinePool
	GCPMachinePool *expinfrav1.GCPMachinePool

	GCPServices
}

type MachinePoolScopeParams struct {
	GCPServices
	Client client.Client
	Logger *logr.Logger

	ClusterGetter  cloud.ClusterGetter
	Cluster        *clusterv1.Cluster
	MachinePool    *expclusterv1.MachinePool
	GCPMachinePool *expinfrav1.GCPMachinePool
}

func NewMachinePoolScope(params MachinePoolScopeParams) (*MachinePoolScope, error) {
	if params.Client == nil {
		return nil, errors.New("client is required when creating a MachinePoolScope")
	}
	if params.ClusterGetter == nil {
		return nil, fmt.Errorf("clustergetter is required when creating a MachinePoolScope")
	}
	if params.MachinePool == nil {
		return nil, errors.New("machinepool is required when creating a MachinePoolScope")
	}
	if params.Cluster == nil {
		return nil, errors.New("cluster is required when creating a MachinePoolScope")
	}
	if params.GCPMachinePool == nil {
		return nil, errors.New("gcpmachinepool is required when creating a MachinePoolScope")
	}
	if params.GCPServices.Compute == nil {
		return nil, errors.New("gcp compute client is required when creating MachinePoolScope")
	}

	if params.Logger == nil {
		log := klogr.New()
		params.Logger = &log
	}

	helper, err := patch.NewHelper(params.GCPMachinePool, params.Client)
	if err != nil {
		return nil, fmt.Errorf("creating patch helper: %w", err)
	}

	return &MachinePoolScope{
		Logger:         *params.Logger,
		client:         params.Client,
		patchHelper:    helper,
		Cluster:        params.Cluster,
		MachinePool:    params.MachinePool,
		ClusterGetter:  params.ClusterGetter,
		GCPMachinePool: params.GCPMachinePool,
		GCPServices:    params.GCPServices,
	}, nil
}

func (m *MachinePoolScope) Name() string {
	return m.GCPMachinePool.Name
}

func (m *MachinePoolScope) Namespace() string {
	return m.GCPMachinePool.Namespace
}

func (m *MachinePoolScope) Cloud() cloud.Cloud {
	return newCloud(m.Project(), m.GCPServices)
}

func (m *MachinePoolScope) Project() string {
	return m.ClusterGetter.Project()
}

func (m *MachinePoolScope) Role() string {
	return "node"
}

func (m *MachinePoolScope) Region() string {
	return m.ClusterGetter.Region()
}

// TODO(eac): figure out what we need to do for regional migs
func (m *MachinePoolScope) Zone() string {
	// TODO(eac): plumb through an override from the machinepool spec, this is wrong
	fd := m.ClusterGetter.FailureDomains()
	zones := make([]string, 0, len(fd))
	for zone := range fd {
		zones = append(zones, zone)
	}
	sort.Strings(zones)
	return zones[0]
}

func (m *MachinePoolScope) InstanceTemplateSpec(ctx context.Context) (*compute.InstanceTemplate, error) {
	bootstrapData, err := m.GetBootstrapData(ctx)
	if err != nil {
		return nil, fmt.Errorf("getting bootstrap data: %w", err)
	}

	template := &compute.InstanceTemplate{
		Properties: &compute.InstanceProperties{
			Metadata: &compute.Metadata{
				Items: []*compute.MetadataItems{
					{
						Key:   "user-data",
						Value: pointer.StringPtr(bootstrapData),
					},
				},
			},
			MachineType: m.GCPMachinePool.Spec.GCPInstanceTemplate.InstanceType,
			Tags: &compute.Tags{
				Items: append(
					m.GCPMachinePool.Spec.GCPInstanceTemplate.AdditionalNetworkTags,
					fmt.Sprintf("%s-%s", m.ClusterGetter.Name(), m.Role()),
					m.ClusterGetter.Name(),
				),
			},
			Labels: infrav1.Build(infrav1.BuildParams{
				ClusterName: m.ClusterGetter.Name(),
				// TODO(eac): does this technically apply to MIG-owned resources?
				Lifecycle:  infrav1.ResourceLifecycleOwned,
				Role:       pointer.StringPtr(m.Role()),
				Additional: m.ClusterGetter.AdditionalLabels().AddLabels(m.GCPMachinePool.Spec.GCPInstanceTemplate.AdditionalLabels),
			}),
			Scheduling: &compute.Scheduling{
				Preemptible: m.GCPMachinePool.Spec.GCPInstanceTemplate.Preemptible,
			},
		},
	}

	template.Properties.CanIpForward = true
	if ipfw := m.GCPMachinePool.Spec.GCPInstanceTemplate.IPForwarding; ipfw != nil && *ipfw == infrav1.IPForwardingDisabled {
		template.Properties.CanIpForward = false
	}

	template.Properties.Disks = append(template.Properties.Disks, m.InstanceImageSpec())
	// TODO(eac): additional disks
	template.Properties.Metadata = m.InstanceAdditionalMetadataSpec()
	template.Properties.ServiceAccounts = append(template.Properties.ServiceAccounts, m.InstanceServiceAccountsSpec())
	template.Properties.NetworkInterfaces = append(template.Properties.NetworkInterfaces, m.InstanceNetworkInterfaceSpec())

	// instance templates are immutable, so we generate a unique name based on a hash of the spec contents
	name, err := instanceTemplateName(template)
	if err != nil {
		return nil, fmt.Errorf("generating instance template name: %w", err)
	}
	template.Name = name

	return template, nil
}

// return a  63 char, RFC1035 compliant name
func instanceTemplateName(template *compute.InstanceTemplate) (string, error) {
	raw, err := template.MarshalJSON()
	if err != nil {
		return "", fmt.Errorf("marshalling instance template spec to json: %w", err)
	}
	hash, err := hash.Base36TruncatedHash(string(raw), 62)
	if err != nil {
		return "", fmt.Errorf("hashing instance template spec: %w", err)
	}
	// guarantee that name starts with a letter (name must be 1-63 characters long and
	// match the regular expression `[a-z]([-a-z0-9]*[a-z0-9])?`)
	return "a" + hash, nil
}

func (m *MachinePoolScope) InstanceImageSpec() *compute.AttachedDisk {
	version := ""
	if v := m.MachinePool.Spec.Template.Spec.Version; v != nil {
		version = *v
	}

	// TODO(eac): can we do something better here?
	image := "capi-ubuntu-1804-k8s-" + strings.ReplaceAll(semver.MajorMinor(version), ".", "-")
	sourceImage := path.Join("projects", m.ClusterGetter.Project(), "global", "images", "family", image)
	if m.GCPMachinePool.Spec.GCPInstanceTemplate.Image != nil {
		sourceImage = *m.GCPMachinePool.Spec.GCPInstanceTemplate.Image
	} else if m.GCPMachinePool.Spec.GCPInstanceTemplate.ImageFamily != nil {
		sourceImage = *m.GCPMachinePool.Spec.GCPInstanceTemplate.ImageFamily
	}

	diskType := infrav1.PdStandardDiskType
	if t := m.GCPMachinePool.Spec.GCPInstanceTemplate.RootDeviceType; t != nil {
		diskType = *t
	}

	return &compute.AttachedDisk{
		AutoDelete: true,
		Boot:       true,
		InitializeParams: &compute.AttachedDiskInitializeParams{
			DiskSizeGb:  m.GCPMachinePool.Spec.GCPInstanceTemplate.RootDeviceSize,
			DiskType:    string(diskType),
			SourceImage: sourceImage,
		},
	}
}

func (m *MachinePoolScope) InstanceAdditionalMetadataSpec() *compute.Metadata {
	metadata := &compute.Metadata{}
	for _, additionalMetadata := range m.GCPMachinePool.Spec.GCPInstanceTemplate.AdditionalMetadata {
		metadata.Items = append(metadata.Items, &compute.MetadataItems{
			Key:   additionalMetadata.Key,
			Value: additionalMetadata.Value,
		})
	}
	return metadata
}

func (m *MachinePoolScope) InstanceServiceAccountsSpec() *compute.ServiceAccount {
	serviceAccount := &compute.ServiceAccount{
		Email: "default",
		Scopes: []string{
			compute.CloudPlatformScope,
		},
	}

	if sa := m.GCPMachinePool.Spec.GCPInstanceTemplate.ServiceAccount; sa != nil {
		serviceAccount.Email = sa.Email
		serviceAccount.Scopes = sa.Scopes
	}

	return serviceAccount
}

func (m *MachinePoolScope) InstanceNetworkInterfaceSpec() *compute.NetworkInterface {
	networkInterface := &compute.NetworkInterface{
		Network: path.Join("projects", m.ClusterGetter.Project(), "global", "networks", m.ClusterGetter.NetworkName()),
	}

	if pubip := m.GCPMachinePool.Spec.GCPInstanceTemplate.PublicIP; pubip != nil {
		networkInterface.AccessConfigs = []*compute.AccessConfig{
			{
				Type: "ONE_TO_ONE_NAT",
				Name: "External NAT",
			},
		}
	}

	if subnet := m.GCPMachinePool.Spec.GCPInstanceTemplate.Subnet; subnet != nil {
		networkInterface.Subnetwork = path.Join("regions", m.ClusterGetter.Region(), "subnetworks", *subnet)
	}

	return networkInterface
}

func (m *MachinePoolScope) InstanceGroupManagerSpec(ctx context.Context) (*compute.InstanceGroupManager, error) {
	template, err := m.InstanceTemplateSpec(ctx)
	if err != nil {
		return nil, fmt.Errorf("getting instance template name: %w", err)
	}

	igm := &compute.InstanceGroupManager{
		Name:             m.Name(),
		BaseInstanceName: m.Name(),
		InstanceTemplate: gcpcloud.RelativeResourceName(m.Project(), "instanceTemplates", meta.GlobalKey(template.Name)),
		TargetSize:       int64(m.GCPMachinePool.Spec.TargetSize),
	}

	return igm, nil
}

func (m *MachinePoolScope) GetBootstrapData(ctx context.Context) (string, error) {
	mpTemplate := m.MachinePool.Spec.Template

	// TODO(eac): look at awsmachinepool to confirm this
	if mpTemplate.Spec.Bootstrap.DataSecretName == nil {
		return "", errors.New("error retrieving bootstrap data: linked MachinePoolTemplate's bootstrap.dataSecretName is nil")
	}

	secret := &corev1.Secret{}
	key := types.NamespacedName{Namespace: m.Namespace(), Name: *mpTemplate.Spec.Bootstrap.DataSecretName}
	if err := m.client.Get(ctx, key, secret); err != nil {
		return "", fmt.Errorf("getting bootstrap datasecret %q: %w", key, err)
	}

	value, ok := secret.Data["value"]
	if !ok {
		return "", errors.New("getting bootstrap data: secret key \"value\" is missing")
	}

	return string(value), nil
}

func (m *MachinePoolScope) PatchObject() error {
	return m.patchHelper.Patch(
		context.TODO(),
		m.GCPMachinePool,
		// TODO(eac): withOwnedConditions?
	)
}

func (m *MachinePoolScope) Close() error {
	return m.PatchObject()
}
