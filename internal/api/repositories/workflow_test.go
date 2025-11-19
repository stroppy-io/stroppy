package repositories

import (
	"testing"
	"time"

	"github.com/stroppy-io/stroppy-cloud-panel/internal/infrastructure/orm"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/proto/panel"
)

func TestEnsureWorkflowTaskInput(t *testing.T) {
	task := &panel.WorkflowTask{
		Id:         "test-task-id",
		WorkflowId: "test-workflow-id",
	}
	q := orm.WorkflowTask.
		SelectAll().
		Where(
			orm.WorkflowTask.Id.AnyOf(
				orm.WorkflowEdge.Select(orm.WorkflowEdge.FromId).Where(
					orm.WorkflowEdge.WorkflowId.Eq(task.GetWorkflowId()),
					orm.WorkflowEdge.ToId.Eq(task.GetId()),
				),
			),
			orm.WorkflowTask.WorkflowId.Eq(task.GetWorkflowId()),
			orm.WorkflowTask.OnWorker.Eq(""),
		)
	sql, args := q.Build()
	t.Log(sql, args)
}

func TestWorkflowRepo_SetChildrenTasksAsPending(t *testing.T) {
	task := &panel.WorkflowTask{
		Id:         "test-task-id",
		WorkflowId: "test-workflow-id",
	}
	q := orm.WorkflowTask.Update().
		Set(
			orm.WorkflowTask.Status.Set(int32(panel.WorkflowTask_STATUS_PENDING)),
			orm.WorkflowTask.UpdatedAt.Set(time.Now()),
		).
		Where(
			orm.WorkflowTask.WorkflowId.Eq(task.GetWorkflowId()),
			orm.WorkflowTask.Id.AnyOf(
				orm.WorkflowEdge.Select(orm.WorkflowEdge.ToId).Where(
					orm.WorkflowEdge.WorkflowId.Eq(task.GetWorkflowId()),
					orm.WorkflowEdge.FromId.Eq(task.GetId()),
				),
			),
		)
	sql, args := q.Build()
	t.Log(sql, args)
}
