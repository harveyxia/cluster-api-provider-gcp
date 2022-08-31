package v1beta1

import clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"

const (
	// MIGReadyCondition reports the current status of the managed instance group. Ready indicates the instance group is provisioned.
	MIGReadyCondition clusterv1.ConditionType = "MIGReady"

	// InstanceTemplateReadyCondition represents the status of a GCPMachinePool's associated Instance Template.
	InstanceTemplateReadyCondition clusterv1.ConditionType = "InstanceTemplateReady"
)

const (
	// WaitingForBootstrapDataReason used when machine is waiting for bootstrap data to be ready before proceeding.
	// TODO import from cluster-api once version is bumped
	WaitingForBootstrapDataReason = "WaitingForBootstrapData"

	// MIGReadyNotReadyReason indicates that the instance group was not created successfully.
	MIGReadyNotReadyReason = "MIGNotReady"

	// InstanceTemplateNotReadyReason indicates that the instance template was not created successfully.
	InstanceTemplateNotReadyReason = "InstanceTemplateNotReady"
)
