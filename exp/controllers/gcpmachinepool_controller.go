package controllers

import (
	"context"
	"fmt"

	"google.golang.org/api/compute/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/record"
	infrav1 "sigs.k8s.io/cluster-api-provider-gcp/api/v1beta1"
	"sigs.k8s.io/cluster-api-provider-gcp/cloud/scope"
	"sigs.k8s.io/cluster-api-provider-gcp/cloud/services/compute/managedinstancegroups"
	expinfrav1 "sigs.k8s.io/cluster-api-provider-gcp/exp/api/v1beta1"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/bootstrap/kubeadm/api/v1beta1"
	expclusterv1 "sigs.k8s.io/cluster-api/exp/api/v1beta1"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/conditions"
	"sigs.k8s.io/cluster-api/util/predicates"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

type GCPMachinePoolReconciler struct {
	client.Client
	Recorder         record.EventRecorder
	WatchFilterValue string
}

// +kubebuilder:rbac:groups="",resources=events,verbs=get;list;watch;create;update;patch
// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=machinepools;machinepools/status,verbs=get;list;watch
// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=gcpmachinepools,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=gcpmachinepools/status,verbs=get;update;patch

func (r *GCPMachinePoolReconciler) Reconcile(ctx context.Context, req ctrl.Request) (_ ctrl.Result, reterr error) {
	log := ctrl.LoggerFrom(ctx)

	gcpMachinePool := &expinfrav1.GCPMachinePool{}
	if err := r.Get(ctx, req.NamespacedName, gcpMachinePool); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, fmt.Errorf("getting gcpmachinepool: %w", err)
	}

	// TODO(eac): turn into a "util" function, shared with GetClusterFromMetadata
	machinePool, err := getOwnerMachinePool(ctx, r.Client, gcpMachinePool.ObjectMeta)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("getting owner machinepool: %w", err)
	}
	if machinePool == nil {
		log.Info("MachinePool Controller has yet to set OwnerRef")
		return ctrl.Result{}, nil
	}
	log = log.WithValues("machinePool", machinePool.Name)

	cluster, err := util.GetClusterFromMetadata(ctx, r.Client, machinePool.ObjectMeta)
	if err != nil {
		log.Info("MachinePool is missing cluster label or cluster does not exist")
		return ctrl.Result{}, nil
	}

	log = log.WithValues("cluster", cluster.Name)

	infraCluster, err := r.getInfraCluster(ctx, cluster, gcpMachinePool)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("getting infra cluster: %w", err)
	}
	if infraCluster == nil {
		log.Info("GCPCluster is not ready yet")
		return ctrl.Result{}, nil
	}

	// initialize GCP compute service client
	computeSvc, err := compute.NewService(context.TODO())
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to create gcp compute client: %v", err)
	}

	machinePoolScope, err := scope.NewMachinePoolScope(scope.MachinePoolScopeParams{
		Client:         r.Client,
		Logger:         &log,
		ClusterGetter:  infraCluster,
		MachinePool:    machinePool,
		Cluster:        cluster,
		GCPMachinePool: gcpMachinePool,
		GCPServices:    scope.GCPServices{Compute: computeSvc},
	})
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("creating scope: %w", err)
	}

	defer func() {
		//conditions.SetSummary(machinePoolScope.GCPMachinePool)// set conditions?

		if err := machinePoolScope.Close(); err != nil && reterr == nil {
			reterr = err
		}
	}()

	if wasDeleted(gcpMachinePool) {
		return r.reconcileDelete(ctx, machinePoolScope)
	}

	return r.reconcileNormal(ctx, machinePoolScope)
}

func wasDeleted(obj client.Object) bool {
	return !obj.GetDeletionTimestamp().IsZero()
}

func (r *GCPMachinePoolReconciler) SetupWithManager(ctx context.Context, mgr ctrl.Manager, options controller.Options) error {
	return ctrl.NewControllerManagedBy(mgr).
		WithOptions(options).
		For(&expinfrav1.GCPMachinePool{}).
		Watches(
			&source.Kind{Type: &expclusterv1.MachinePool{}},
			handler.EnqueueRequestsFromMapFunc(machinePoolToInfrastructureMapFunc(expinfrav1.GroupVersion.WithKind("GCPMachinePool"))),
		).
		// TODO(eac): watch cluster, etc?
		WithEventFilter(predicates.ResourceNotPausedAndHasFilterLabel(ctrl.LoggerFrom(ctx), r.WatchFilterValue)).
		Complete(r)
}

func (r *GCPMachinePoolReconciler) reconcileNormal(ctx context.Context, machinePoolScope *scope.MachinePoolScope) (ctrl.Result, error) {
	log := log.FromContext(ctx)
	log.Info("reconciling GCPMachinePool")

	// TODO(eac): handle failure state

	// add finalizer
	controllerutil.AddFinalizer(machinePoolScope.GCPMachinePool, expinfrav1.MachinePoolFinalizer)
	if err := machinePoolScope.PatchObject(); err != nil {
		return ctrl.Result{}, fmt.Errorf("patching finalizer: %w", err)
	}

	// check that cluster infrastructure ready
	if !machinePoolScope.Cluster.Status.InfrastructureReady {
		machinePoolScope.Info("Cluster infrastructure is not ready yet")
		conditions.MarkFalse(machinePoolScope.GCPMachinePool, expinfrav1.MIGReadyCondition, v1beta1.WaitingForClusterInfrastructureReason, clusterv1.ConditionSeverityInfo, "")
		return ctrl.Result{}, nil
	}

	//check that bootstrap data is available and populated
	if machinePoolScope.MachinePool.Spec.Template.Spec.Bootstrap.DataSecretName == nil {
		machinePoolScope.Info("Bootstrap data secret reference is not yet available")
		conditions.MarkFalse(machinePoolScope.GCPMachinePool, expinfrav1.MIGReadyCondition, expinfrav1.WaitingForBootstrapDataReason, clusterv1.ConditionSeverityInfo, "")
		return ctrl.Result{}, nil
	}

	// initialize MIG client
	migsvc := managedinstancegroups.New(machinePoolScope)

	if err := migsvc.Reconcile(ctx); err != nil {
		// TODO(eac): record event?
		return ctrl.Result{}, fmt.Errorf("reconciling managedinstancegroup resources: %w", err)
	}

	// TODO(eac): set conditions?
	// TODO(eac):     ^ intermediate condition for instance template readiness

	return ctrl.Result{}, nil
}

func (r *GCPMachinePoolReconciler) reconcileDelete(ctx context.Context, machinePoolScope *scope.MachinePoolScope) (ctrl.Result, error) {
	log := log.FromContext(ctx)
	log.Info("handling deleted gcpmachinepool")

	if err := managedinstancegroups.New(machinePoolScope).Delete(ctx); err != nil {
		return ctrl.Result{}, fmt.Errorf("deleting managedinstancegroup resources: %w", err)
	}

	controllerutil.RemoveFinalizer(machinePoolScope.GCPMachinePool, expinfrav1.MachinePoolFinalizer)
	// TODO(eac): record event
	return ctrl.Result{}, nil
}

func (r *GCPMachinePoolReconciler) getInfraCluster(
	ctx context.Context,
	cluster *clusterv1.Cluster,
	gcpMachinePool *expinfrav1.GCPMachinePool,
) (*scope.ClusterScope, error) {
	gcpCluster := &infrav1.GCPCluster{}
	infraClusterName := client.ObjectKey{
		Namespace: gcpMachinePool.Namespace,
		Name:      cluster.Spec.InfrastructureRef.Name,
	}

	if err := r.Client.Get(ctx, infraClusterName, gcpCluster); err != nil {
		// GCPCluster is not ready
		return nil, nil // nolint?
	}

	clusterScope, err := scope.NewClusterScope(scope.ClusterScopeParams{
		// TODO(eac): plumb me thru
		GCPServices: scope.GCPServices{},
		Client:      r.Client,
		Cluster:     cluster,
		GCPCluster:  gcpCluster,
		// TODO(eac): controller name?
		// TODO(eac): logger?
	})
	if err != nil {
		return nil, fmt.Errorf("constructing cluster scope: %w", err)
	}

	return clusterScope, nil
}

func getOwnerMachinePool(ctx context.Context, c client.Client, obj metav1.ObjectMeta) (*expclusterv1.MachinePool, error) {
	for _, ref := range obj.OwnerReferences {
		if ref.Kind != "MachinePool" {
			continue
		}

		gv, err := schema.ParseGroupVersion(ref.APIVersion)
		if err != nil {
			return nil, fmt.Errorf("parsing groupversion: %w", err)
		}

		if gv.Group == expclusterv1.GroupVersion.Group {
			return getMachinePoolByName(ctx, c, obj.Namespace, ref.Name)
		}
	}

	return nil, nil
}

func getMachinePoolByName(ctx context.Context, c client.Client, namespace, name string) (*expclusterv1.MachinePool, error) {
	m := &expclusterv1.MachinePool{}
	key := client.ObjectKey{Name: name, Namespace: namespace}
	if err := c.Get(ctx, key, m); err != nil {
		return nil, err
	}
	return m, nil
}

func machinePoolToInfrastructureMapFunc(gvk schema.GroupVersionKind) handler.MapFunc {
	return func(o client.Object) []reconcile.Request {
		m, ok := o.(*expclusterv1.MachinePool)
		if !ok {
			panic(fmt.Sprintf("Expected a MachinePool but got a %T", o))
		}

		gk := gvk.GroupKind()
		// Return early if the GroupKind doesn't match what we expect
		infraGK := m.Spec.Template.Spec.InfrastructureRef.GroupVersionKind().GroupKind()
		if gk != infraGK {
			return nil
		}

		return []reconcile.Request{
			{
				NamespacedName: client.ObjectKey{
					Namespace: m.Namespace,
					Name:      m.Spec.Template.Spec.InfrastructureRef.Name,
				},
			},
		}
	}
}
