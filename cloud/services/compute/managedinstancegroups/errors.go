package managedinstancegroups

import "fmt"

// InstanceTemplateError is returned if an error was encountered when reconciling an instance template
type InstanceTemplateError struct {
	Err error
}

func (e *InstanceTemplateError) Error() string {
	return fmt.Errorf("reconciling instance template: %w", e.Err).Error()
}

// InstanceGroupError is returned if an error was encountered when reconciling an instance template
type InstanceGroupError struct {
	Err error
}

func (e *InstanceGroupError) Error() string {
	return fmt.Errorf("reconciling instance group: %w", e.Err).Error()
}
