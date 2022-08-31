package managedinstancegroups

import (
	"context"

	"github.com/GoogleCloudPlatform/k8s-cloud-provider/pkg/cloud/filter"
	"github.com/GoogleCloudPlatform/k8s-cloud-provider/pkg/cloud/meta"
	"google.golang.org/api/compute/v1"

	"sigs.k8s.io/cluster-api-provider-gcp/cloud"
)

type instancetemplatesInterface interface {
	Get(ctx context.Context, key *meta.Key) (*compute.InstanceTemplate, error)
	List(ctx context.Context, fl *filter.F) ([]*compute.InstanceTemplate, error)
	Insert(ctx context.Context, key *meta.Key, obj *compute.InstanceTemplate) error
	Delete(ctx context.Context, key *meta.Key) error
}

type instancegroupmanagersInterface interface {
	Get(ctx context.Context, key *meta.Key) (*compute.InstanceGroupManager, error)
	List(ctx context.Context, zone string, fl *filter.F) ([]*compute.InstanceGroupManager, error)
	Insert(ctx context.Context, key *meta.Key, obj *compute.InstanceGroupManager) error
	Delete(ctx context.Context, key *meta.Key) error
	CreateInstances(context.Context, *meta.Key, *compute.InstanceGroupManagersCreateInstancesRequest) error
	DeleteInstances(context.Context, *meta.Key, *compute.InstanceGroupManagersDeleteInstancesRequest) error
	Resize(context.Context, *meta.Key, int64) error
	SetInstanceTemplate(context.Context, *meta.Key, *compute.InstanceGroupManagersSetInstanceTemplateRequest) error
}

type Scope interface {
	cloud.MachinePool
	InstanceTemplateSpec(ctx context.Context) (*compute.InstanceTemplate, error)
	InstanceGroupManagerSpec(ctx context.Context) (*compute.InstanceGroupManager, error)
}

type Service struct {
	scope                 Scope
	instancetemplates     instancetemplatesInterface
	instancegroupmanagers instancegroupmanagersInterface
}

var _ cloud.Reconciler = &Service{}

func New(scope Scope) *Service {
	return &Service{
		scope:                 scope,
		instancetemplates:     scope.Cloud().InstanceTemplates(),
		instancegroupmanagers: scope.Cloud().InstanceGroupManagers(),
	}
}
