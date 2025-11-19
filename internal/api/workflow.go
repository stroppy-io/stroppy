package api

import (
	"context"

	"github.com/stroppy-io/stroppy-cloud-panel/internal/proto/panel"
)

func (p *PanelService) GetWorkflow(ctx context.Context, ulid *panel.Ulid) (*panel.Workflow, error) {
	return p.workflowRepo.GetWorkflow(ctx, ulid)
}

func (p *PanelService) CancelWorkflow(ctx context.Context, ulid *panel.Ulid) (*panel.Workflow, error) {
	// TODO: think about something else
	return p.workflowRepo.MarkWorkflowAsCanceled(ctx, ulid)
}
