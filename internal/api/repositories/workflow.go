package repositories

import (
	"context"
	"time"

	"github.com/samber/lo"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/core/protohelp"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/entity/ids"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/infrastructure/orm"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/infrastructure/postgresql/sqlerr"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/infrastructure/postgresql/sqlexec"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/proto/panel"
)

type WorkflowRepo struct {
	workflowRepo     orm.WorkflowRepository
	workflowEdgeRepo orm.WorkflowEdgeRepository
	workflowTaskRepo orm.WorkflowTaskRepository
}

func NewWorkflowRepo(
	executor sqlexec.Executor,
) *WorkflowRepo {
	getter := func(ctx context.Context, operation orm.SqlOpType) orm.DB {
		return executor
	}
	return &WorkflowRepo{
		workflowRepo: orm.NewWorkflowRepository(
			getter,
			ids.UlidFromString,
			ids.UlidToStr,
		),
		workflowEdgeRepo: orm.NewWorkflowEdgeRepository(
			getter,
			ids.UlidFromString,
			ids.UlidToStr,
		),
		workflowTaskRepo: orm.NewWorkflowTaskRepository(
			getter,
		),
	}
}

func (w *WorkflowRepo) GetWorkflow(ctx context.Context, id *panel.Ulid) (*panel.Workflow, error) {
	wrk, err := w.workflowRepo.GetBy(
		ctx,
		orm.Workflow.SelectAll().Where(orm.Workflow.Id.Eq(id.GetId())),
	)
	if err != nil {
		return nil, err
	}
	nodes, err := w.workflowTaskRepo.ListBy(
		ctx,
		orm.WorkflowTask.SelectAll().Where(orm.WorkflowTask.WorkflowId.Eq(id.GetId())),
	)
	if err != nil {
		return nil, err
	}
	wrk.Tasks = nodes
	wrk.Edges, err = w.workflowEdgeRepo.ListBy(
		ctx,
		orm.WorkflowEdge.SelectAll().Where(orm.WorkflowEdge.WorkflowId.Eq(id.GetId())),
	)
	if err != nil {
		return nil, err
	}
	return wrk, nil
}

//func (w *WorkflowRepo) DeleteWorkflow(ctx context.Context, id *panel.Ulid) error {
//	err := w.workflowRepo.Exec(ctx, orm.Workflow.Delete().Where(orm.Workflow.Id.Eq(id.GetId())))
//	if err != nil {
//		return err
//	}
//	err = w.workflowTaskRepo.Exec(ctx,
//		orm.WorkflowTask.Delete().Where(orm.WorkflowTask.WorkflowId.Eq(id.GetId())))
//	if err != nil {
//		return err
//	}
//	err = w.workflowEdgeRepo.Exec(ctx,
//		orm.WorkflowEdge.Delete().Where(orm.WorkflowEdge.WorkflowId.Eq(id.GetId())))
//	if err != nil {
//		return err
//	}
//	return nil
//}

func (w *WorkflowRepo) MarkWorkflowAsCanceled(ctx context.Context, id *panel.Ulid) (*panel.Workflow, error) {
	err := w.workflowTaskRepo.Exec(ctx, orm.WorkflowTask.Update().
		Set(orm.WorkflowTask.Status.Set(int32(panel.WorkflowTask_STATUS_CANCELLED))).
		Where(orm.WorkflowTask.WorkflowId.Eq(id.GetId())))
	if err != nil {
		return nil, err
	}
	return w.GetWorkflow(ctx, id)
}

func (w *WorkflowRepo) CreateWorkflow(ctx context.Context, workflow *panel.Workflow) error {
	err := w.workflowRepo.ScannerRepository().Insert(
		ctx,
		orm.WorkflowToScanner(ids.UlidToStr)(workflow),
	)
	if err != nil {
		return err
	}
	for _, node := range workflow.GetTasks() {
		err := w.workflowTaskRepo.ScannerRepository().Insert(
			ctx,
			orm.WorkflowTaskToScanner()(node),
		)
		if err != nil {
			return err
		}
	}
	for _, edge := range workflow.GetEdges() {
		err := w.workflowEdgeRepo.ScannerRepository().Insert(
			ctx,
			orm.WorkflowEdgeToScanner(
				ids.UlidToStr,
				orm.WithWorkflowEdgeWorkflowId(workflow.GetId().GetId()),
			)(edge),
		)
		if err != nil {
			return err
		}
	}
	return nil
}

// ListActualTasks and SetWorkflowTaskOnWorker doing in one transaction
func (w *WorkflowRepo) ListActualTasks(
	ctx context.Context,
	onWorker string,
	forCleanup bool,
	statues []panel.WorkflowTask_Status,
) ([]*panel.WorkflowTask, error) {
	statuesInt32 := lo.Map(statues, func(s panel.WorkflowTask_Status, _ int) int32 {
		return int32(s)
	})
	q := orm.WorkflowTask.SelectAll().Where(
		orm.WorkflowTask.OnWorker.Eq(onWorker),
		orm.WorkflowTask.Status.Any(statuesInt32...),
	).ForUpdate()
	if forCleanup {
		q = q.Where(orm.WorkflowTask.WorkflowId.AnyOf(
			orm.WorkflowTask.Select(orm.WorkflowTask.WorkflowId).
				GroupBy(orm.WorkflowTask.WorkflowId).
				Having(orm.WorkflowTask.Raw(
					"BOOL_AND(status IN ?)",
					statues,
				)),
		))
	}
	return w.workflowTaskRepo.ListBy(ctx, q)
}

func (w *WorkflowRepo) SetWorkflowTaskOnWorker(
	ctx context.Context,
	tasksIds []string,
	onWorker string,
) error {
	err := w.workflowTaskRepo.Exec(ctx, orm.WorkflowTask.Update().
		Set(
			orm.WorkflowTask.OnWorker.Set(onWorker),
			orm.WorkflowTask.UpdatedAt.Set(time.Now()),
		).
		Where(orm.WorkflowTask.Id.Any(tasksIds...)),
	)
	return err
}

func (w *WorkflowRepo) SetChildrenTasksAsPending(
	ctx context.Context,
	task *panel.WorkflowTask,
) error {
	err := w.workflowTaskRepo.Exec(ctx, orm.WorkflowTask.Update().
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
		),
	)
	return err
}

func (w *WorkflowRepo) EnsureWorkflowTaskInput(
	ctx context.Context,
	task *panel.WorkflowTask,
) error {
	previousTasks, err := w.workflowTaskRepo.ListBy(ctx, orm.WorkflowTask.
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
		))
	if err != nil {
		if sqlerr.IsNotFound(err) {
			return nil
		}
		return err
	}
	for _, prevTask := range previousTasks {
		newTaskInput, err := protohelp.CopyCommonFieldsFromAnypbMessages(
			prevTask.GetOutput(),
			task.GetInput(),
		)
		if err != nil {
			return err
		}
		task.Input = newTaskInput
	}
	return nil
}

func (w *WorkflowRepo) SaveWorkflowTask(
	ctx context.Context,
	task *panel.WorkflowTask,
) error {
	fieldsSetters := orm.GetFieldsSetters[orm.WorkflowTaskField, *orm.WorkflowTaskScanner](
		orm.WorkflowTaskToScanner()(task),
	)
	err := w.workflowTaskRepo.Exec(ctx, orm.WorkflowTask.Insert().From(fieldsSetters...).
		OnConflict(orm.WorkflowTask.Id).
		DoUpdate(orm.WorkflowTask.AllFieldsExcept(orm.WorkflowTask.WorkflowId)...),
	)
	if err != nil {
		return err
	}
	return nil
}
