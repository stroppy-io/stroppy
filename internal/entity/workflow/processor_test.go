package workflow

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/avito-tech/go-transaction-manager/trm"
	"github.com/samber/lo"
	"github.com/stretchr/testify/require"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/core/logger"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/proto/crossplane"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/proto/panel"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type mockWorkflowTaskRepository struct {
	tasks map[string]*panel.WorkflowTask
}

func (m *mockWorkflowTaskRepository) ListActualTasks(ctx context.Context, onWorker string, cleanedUp bool, statues []panel.WorkflowTask_Status) ([]*panel.WorkflowTask, error) {
	return lo.Values(m.tasks), nil
}

func (m *mockWorkflowTaskRepository) SetChildrenTasksAsPending(ctx context.Context, task *panel.WorkflowTask) error {
	return nil
}

func (m *mockWorkflowTaskRepository) EnsureWorkflowTaskInput(ctx context.Context, task *panel.WorkflowTask) error {
	return nil
}

func (m *mockWorkflowTaskRepository) SetWorkflowTaskOnWorker(ctx context.Context, tasksIds []string, onWorker string) error {
	return nil
}

func (m *mockWorkflowTaskRepository) SaveWorkflowTask(ctx context.Context, task *panel.WorkflowTask) error {
	m.tasks[task.GetId()] = task
	return nil
}

type mockTaskHandler struct {
	attempt uint32
}

func (m *mockTaskHandler) Start(
	ctx context.Context,
	input *panel.WorkflowTask_DeployDatabase_Input,
) (*panel.WorkflowTask_DeployDatabase_Output, error) {
	if m.attempt < 2 {
		m.attempt++
		return nil, fmt.Errorf("mock task failed")
	}
	return &panel.WorkflowTask_DeployDatabase_Output{
		DatabaseDeployment: &crossplane.Deployment{
			Id: "test-deployment-id",
		},
	}, nil
}

type deployDatabaseTaskState = TaskState[*panel.WorkflowTask_DeployDatabase_Input, *panel.WorkflowTask_DeployDatabase_Output]

func (m *mockTaskHandler) Status(ctx context.Context, state deployDatabaseTaskState) (panel.WorkflowTask_Status, error) {
	state.GetOutput().GetDatabaseDeployment().Id = "test-deployment-id-status"
	return panel.WorkflowTask_STATUS_COMPLETED, nil
}

func (m *mockTaskHandler) Cleanup(ctx context.Context, state deployDatabaseTaskState) error {
	return nil
}

type mockTxManager struct{}

func (m mockTxManager) Do(ctx context.Context, f func(ctx context.Context) error) error {
	return f(ctx)
}

func (m mockTxManager) DoWithSettings(ctx context.Context, settings trm.Settings, f func(ctx context.Context) error) error {
	return f(ctx)
}

func TestTaskProcessor(t *testing.T) {
	repo := &mockWorkflowTaskRepository{tasks: make(map[string]*panel.WorkflowTask)}
	processor, err := NewTaskProcessor(
		&Config{},
		logger.Global(),
		mockTxManager{},
		repo,
		map[panel.WorkflowTask_Type]TaskWrapperBuilder{
			panel.WorkflowTask_TYPE_COLLECT_RUN_RESULTS: NewTaskBuilder(&mockTaskHandler{}),
		},
	)
	require.NoError(t, err)
	require.NotNil(t, processor)

	task := &panel.WorkflowTask{
		Id:       "test-task-id",
		TaskType: panel.WorkflowTask_TYPE_COLLECT_RUN_RESULTS,
		Status:   panel.WorkflowTask_STATUS_PENDING,
		Timing: &panel.Timing{
			CreatedAt: timestamppb.Now(),
		},
		RetrySettings: &panel.Retry{
			Backoff: &panel.Retry_Backoff{
				Backoff: &panel.Retry_Backoff_Exponential_{
					Exponential: &panel.Retry_Backoff_Exponential{
						InitialInterval:     durationpb.New(10 * time.Second),
						MaxElapsedTime:      durationpb.New(300 * time.Second),
						MaxInterval:         durationpb.New(300 * time.Second),
						Multiplier:          30.0,
						RandomizationFactor: 0.1,
						RetryStopDuration:   durationpb.New(300 * time.Second),
					},
				},
			},
			MaxAttempts: 3,
		},
		RetryState: &panel.Retry_State{
			BackoffDuration: durationpb.New(0 * time.Second),
			BackoffValue:    timestamppb.New(time.Now()),
			Attempt:         0,
		},
		CleanedUp: false,
		Input:     lo.Must(anypb.New(&panel.WorkflowTask_DeployDatabase_Input{})),
		Output:    lo.Must(anypb.New(&panel.WorkflowTask_DeployDatabase_Output{})),
	}
	err = processor.processWorkflowTask(context.Background(), task)
	require.NoError(t, err)
	require.NotNil(t, repo.tasks[task.GetId()])
	require.Equal(t, panel.WorkflowTask_STATUS_RETRYING, task.GetStatus())
	err = processor.processWorkflowTask(context.Background(), task)
	require.NoError(t, err)
	require.NotNil(t, repo.tasks[task.GetId()])
	require.Equal(t, panel.WorkflowTask_STATUS_RETRYING, task.GetStatus())
	err = processor.processWorkflowTask(context.Background(), task)
	require.NoError(t, err)
	require.NotNil(t, repo.tasks[task.GetId()])
	output1 := &panel.WorkflowTask_DeployDatabase_Output{}
	lo.Must0(anypb.UnmarshalTo(task.GetOutput(), output1, proto.UnmarshalOptions{}))
	require.Equal(t, "test-deployment-id", output1.GetDatabaseDeployment().GetId())
	require.Equal(t, panel.WorkflowTask_STATUS_RUNNING, task.GetStatus())
	err = processor.processWorkflowTask(context.Background(), task)
	require.NoError(t, err)
	require.NotNil(t, repo.tasks[task.GetId()])
	require.Equal(t, panel.WorkflowTask_STATUS_COMPLETED, task.GetStatus())
	output := &panel.WorkflowTask_DeployDatabase_Output{}
	lo.Must0(anypb.UnmarshalTo(task.GetOutput(), output, proto.UnmarshalOptions{}))
	require.Equal(t, "test-deployment-id-status", output.GetDatabaseDeployment().GetId())
}
