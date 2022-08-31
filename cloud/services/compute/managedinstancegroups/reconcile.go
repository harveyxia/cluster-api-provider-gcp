package managedinstancegroups

import (
	"context"
	"fmt"

	"github.com/GoogleCloudPlatform/k8s-cloud-provider/pkg/cloud/meta"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	"sigs.k8s.io/cluster-api-provider-gcp/cloud/gcperrors"
)

func (s *Service) Reconcile(ctx context.Context) error {
	log := logf.FromContext(ctx)
	log.Info("reconciling managedinstancegroup resource")

	if err := s.reconcileInstanceTemplate(ctx); err != nil {
		return &InstanceTemplateError{Err: err}
	}

	// create igm
	igm, err := s.scope.InstanceGroupManagerSpec(ctx)
	if err != nil {
		return &InstanceGroupError{Err: fmt.Errorf("building instance group manager spec: %w", err)}
	}
	if err := s.instancegroupmanagers.Insert(ctx, meta.ZonalKey(igm.Name, s.scope.Zone()), igm); gcperrors.IgnoreAlreadyExists(err) != nil {
		return &InstanceGroupError{Err: fmt.Errorf("inserting instancegroupmanager: %w", err)}
	}

	// TODO(eac): set fields on scope?

	return nil
}

func (s *Service) reconcileInstanceTemplate(ctx context.Context) error {
	instanceTemplateSpec, err := s.scope.InstanceTemplateSpec(ctx)
	if err != nil {
		return fmt.Errorf("building instance template spec: %w", err)
	}

	instanceTemplate, err := s.instancetemplates.Get(ctx, meta.GlobalKey(instanceTemplateSpec.Name))
	if gcperrors.IgnoreNotFound(err) != nil {
		return fmt.Errorf("getting instance template: %w", err)
	}

	// create if not exist
	if instanceTemplate == nil {
		if err := s.instancetemplates.Insert(ctx, meta.GlobalKey(instanceTemplateSpec.Name), instanceTemplateSpec); err != nil {
			return fmt.Errorf("inserting instancetemplate: %w", err)
		}
	}

	// TODO(harvey): handle instance template deletion
	// deletion of in-use instance templates will result in an error

	return nil
}

func (s *Service) Delete(ctx context.Context) error {
	log := logf.FromContext(ctx)
	log.Info("deleting managedinstancegroup resource")

	// delete MIG
	igm, err := s.scope.InstanceGroupManagerSpec(ctx)
	if err != nil {
		return fmt.Errorf("building instance group manager spec: %w", err)
	}

	// NOTE: call blocks until MIG has completed termination
	if err := s.instancegroupmanagers.Delete(ctx, meta.ZonalKey(igm.Name, s.scope.Zone())); gcperrors.IgnoreNotFound(err) != nil {
		return fmt.Errorf("deleting instancegroupmanager: %w", err)
	}

	instanceTemplate, err := s.scope.InstanceTemplateSpec(ctx)
	if err != nil {
		return fmt.Errorf("building instance template spec: %w", err)
	}

	// delete InstanceTemplate
	// ignore not found errors, this means a previous reconcile already deleted it
	// ignore resource in use by another resource errors, this means another instance group is using the template
	if err := s.instancetemplates.Delete(ctx, meta.GlobalKey(instanceTemplate.Name)); gcperrors.IgnoreIsInUse(gcperrors.IgnoreNotFound(err)) != nil {
		return fmt.Errorf("deleting instancetemplate: %w", err)
	}

	return nil
}
