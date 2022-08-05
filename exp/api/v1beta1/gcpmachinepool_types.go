package v1beta1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"

	infrav1 "sigs.k8s.io/cluster-api-provider-gcp/api/v1beta1"
)

const (
	MachinePoolFinalizer = "gcpmachinepool.infrastructure.cluster.x-k8s.io"
)

type GCPInstanceTemplate struct {
	InstanceType string `json:"instanceType,omitempty"`

	AdditionalNetworkTags []string `json:"additionalNetworkTags,omitempty"`

	AdditionalLabels infrav1.Labels `json:"additionalLabels,omitempty"`

	Preemptible bool `json:"preemptile,omitempty"`

	IPForwarding *infrav1.IPForwarding `json:"ipForwarding,omitempty"`

	Image *string `json:"image,omitempty"`

	ImageFamily *string `json:"imageFamily,omitempty"`

	RootDeviceType *infrav1.DiskType `json:"rootDeviceType,omitempty"`

	RootDeviceSize int64 `json:"rootDeviceSize,omitempty"`

	// +listType=map
	// +listMapKey=key
	// +optional
	AdditionalMetadata []infrav1.MetadataItem `json:"additionalMetadata,omitempty"`

	ServiceAccount *infrav1.ServiceAccount `json:"serviceAccount,omitempty"`

	PublicIP *bool `json:"publicIP,omitempty"`

	Subnet *string `json:"subnet,omitempty"`
}

type GCPMachinePoolSpec struct {
	TargetSize int32 `json:"targetSize,omitempty"`

	GCPInstanceTemplate GCPInstanceTemplate `json:"gcpInstanceTemplate"`
}

type GCPMachinePoolStatus struct {
	Ready bool `json:"ready"`

	Replicas int32 `json:"replicas"`

	Conditions clusterv1.Conditions `json:"conditions,omitempty"`
}

// TODO(eac): more printcolumns

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:storageversion
// +kubebuilder:resource:path=gcpmachinepools,scope=Namespaced,categories=cluster-api,shortName=gcpmp
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.ready",description="Machine ready status"
// +kubebuilder:printcolumn:name="Replicas",type="integer",JSONPath=".status.replicas",description="Machine ready status"

type GCPMachinePool struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   GCPMachinePoolSpec   `json:"spec,omitempty"`
	Status GCPMachinePoolStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

type GCPMachinePoolList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []GCPMachinePool `json:"items"`
}

func init() {
	SchemeBuilder.Register(&GCPMachinePool{}, &GCPMachinePoolList{})
}

func (r *GCPMachinePool) GetConditions() clusterv1.Conditions {
	return r.Status.Conditions
}

func (r *GCPMachinePool) SetConditions(conditions clusterv1.Conditions) {
	r.Status.Conditions = conditions
}

func (r *GCPMachinePool) GetObjectKind() schema.ObjectKind {
	return &r.TypeMeta
}

func (r *GCPMachinePoolList) GetObjectKind() schema.ObjectKind {
	return &r.TypeMeta
}
