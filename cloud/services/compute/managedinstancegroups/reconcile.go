package managedinstancegroups

import (
	"context"
	"fmt"

	"github.com/GoogleCloudPlatform/k8s-cloud-provider/pkg/cloud/meta"
	"google.golang.org/api/compute/v1"
	"k8s.io/utils/pointer"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	"sigs.k8s.io/cluster-api-provider-gcp/cloud/gcperrors"
)

func (s *Service) Reconcile(ctx context.Context) error {
	log := logf.FromContext(ctx)
	log.Info("reconciling managedinstancegroup resource")

	bootstrapData, err := s.scope.GetBootstrapData(ctx)
	if err != nil {
		return fmt.Errorf("getting bootstrap data: %w", err)
	}

	// create instancetemplate
	// TODO(eac): handle update instancetemplate
	instanceTemplate := s.scope.InstanceTemplateSpec()
	instanceTemplate.Properties.Metadata.Items = append(instanceTemplate.Properties.Metadata.Items, &compute.MetadataItems{
		Key:   "user-data",
		Value: pointer.StringPtr(bootstrapData),
	})

	if err := s.instancetemplates.Insert(ctx, meta.GlobalKey(instanceTemplate.Name), instanceTemplate); err != nil {
		return fmt.Errorf("inserting instancetemplate: %w", err)
	}

	// create mig
	// TODO(eac): handle create/update
	igm := s.scope.InstanceGroupManagerSpec()
	if err := s.instancegroupmanagers.Insert(ctx, meta.ZonalKey(igm.Name, s.scope.Zone()), igm); err != nil {
		return fmt.Errorf("inserting instancegroupmanager: %w", err)
	}

	// TODO(eac): set fields on scope?

	return nil
}

func (s *Service) Delete(ctx context.Context) error {
	log := logf.FromContext(ctx)
	log.Info("deleting managedinstancegroup resource")

	igm := s.scope.InstanceGroupManagerSpec()
	if err := s.instancegroupmanagers.Delete(ctx, meta.ZonalKey(igm.Name, s.scope.Zone())); gcperrors.IgnoreNotFound(err) != nil {
		return fmt.Errorf("deleting instancegroupmanager: %w", err)
	}

	instanceTemplate := s.scope.InstanceTemplateSpec()
	if err := s.instancetemplates.Delete(ctx, meta.GlobalKey(instanceTemplate.Name)); gcperrors.IgnoreNotFound(err) != nil {
		return fmt.Errorf("deleting instancetemplate: %w", err)
	}

	return nil
}
