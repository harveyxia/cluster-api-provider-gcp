package v1beta1

import clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"

const (
	// MIGReadyCondition reports the current status of the managed instance group. Ready indicates the instance group is provisioned.
	MIGReadyCondition clusterv1.ConditionType = "MIGReady"
)

const (
	// TODO import from cluster-api once version is bumped
	// WaitingForBootstrapDataReason used when machine is waiting for bootstrap data to be ready before proceeding.
	WaitingForBootstrapDataReason = "WaitingForBootstrapData"
)
