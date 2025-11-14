package workflow

import "github.com/stroppy-io/stroppy-cloud-panel/internal/proto/panel"

type TaskHandler interface {
	Handle(*panel.WorkflowTaskNode) error
	Rollback(*panel.WorkflowTaskNode) error
}

type Workflow struct {
	*panel.Workflow
}
