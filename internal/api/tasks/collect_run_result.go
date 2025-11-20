package tasks

import (
	"context"
	"errors"
	"fmt"

	"github.com/stroppy-io/stroppy-cloud-panel/internal/entity/workflow"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/infrastructure/postgres/sqlerr"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/proto/panel"
	"github.com/stroppy-io/stroppy/pkg/common/proto/stroppy"
)

type collectRunResultTaskState = workflow.TaskState[*panel.WorkflowTask_CollectRunResults_Input, *panel.WorkflowTask_CollectRunResults_Output]
type CollectRunResultTaskHandler struct {
	stroppyRunRepo RunRecordRepository
}

func NewCollectRunResultTaskHandler(
	stroppyRunRepo RunRecordRepository,
) *CollectRunResultTaskHandler {
	return &CollectRunResultTaskHandler{
		stroppyRunRepo: stroppyRunRepo,
	}
}

func (c *CollectRunResultTaskHandler) Start(
	_ context.Context,
	_ *panel.WorkflowTask_CollectRunResults_Input,
) (*panel.WorkflowTask_CollectRunResults_Output, error) {
	return &panel.WorkflowTask_CollectRunResults_Output{}, nil
}

func (c *CollectRunResultTaskHandler) Status(
	ctx context.Context,
	state collectRunResultTaskState,
) (panel.WorkflowTask_Status, error) {
	runRecord, err := c.stroppyRunRepo.FindRunRecord(ctx, state.GetInput().GetStroppyRunId().GetId())
	if err != nil {
		if sqlerr.IsNotFound(err) {
			return panel.WorkflowTask_STATUS_PENDING,
				errors.Join(
					err,
					workflow.ErrStatusTemproraryFailed,
					fmt.Errorf("run record not found"),
				)
		}
		return panel.WorkflowTask_STATUS_FAILED, err
	}
	if runRecord.GetStatus() != stroppy.Status_STATUS_IDLE {
		return panel.WorkflowTask_STATUS_COMPLETED, nil
	}
	return panel.WorkflowTask_STATUS_RUNNING, nil
}

func (c *CollectRunResultTaskHandler) Cleanup(
	_ context.Context,
	_ collectRunResultTaskState,
) error {
	return nil
}
