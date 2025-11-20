package workflow

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/samber/lo"
	"github.com/sourcegraph/conc/pool"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/core/build"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/infrastructure/postgres/sqlerr"
	"go.uber.org/atomic"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	postgres "github.com/stroppy-io/stroppy-cloud-panel/internal/infrastructure/postgres"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/proto/panel"
)

type taskRetryConfig struct {
	InitialInterval     time.Duration `mapstructure:"initial_interval" default:"10s"`
	MaxElapsedTime      time.Duration `mapstructure:"max_elapsed_time" default:"300s"`
	MaxInterval         time.Duration `mapstructure:"max_interval" default:"300s"`
	Multiplier          float64       `mapstructure:"multiplier" default:"30.0"`
	RandomizationFactor float64       `mapstructure:"randomization_factor" default:"0.1"`
	RetryStopDuration   time.Duration `mapstructure:"retry_stop_duration" default:"300s"`
}

func ValidateTaskRetryConfig(mp map[string]taskRetryConfig) error {
	for kind := range mp {
		if _, ok := panel.WorkflowTask_Type_value[kind]; !ok {
			return fmt.Errorf("unknown WorkflowTask_Type: %s", kind)
		}
	}
	return nil
}

type Config struct {
	PollInterval    time.Duration              `default:"10s" mapstructure:"poll_interval" validate:"required"`
	TaskLoggerLevel string                     `default:"debug" mapstructure:"task_logger_level" validate:"required"`
	TaskRetryConfig map[string]taskRetryConfig `mapstructure:"task_retries" validate:"required"`
}

type TaskRepository interface {
	// ListActualTasks returns all workflow tasks that are cleaned_up bool and have one of the given statuses
	// it must use select for update to prevent race conditions
	ListActualTasks(
		ctx context.Context,
		onWorker string,
		forCleanup bool,
		statues []panel.WorkflowTask_Status,
	) ([]*panel.WorkflowTask, error)
	SetWorkflowTaskOnWorker(
		ctx context.Context,
		tasksIds []string,
		onWorker string,
	) error
	EnsureWorkflowTaskInput(ctx context.Context, task *panel.WorkflowTask) error
	SaveWorkflowTask(ctx context.Context, task *panel.WorkflowTask) error
	SetChildrenTasksAsPending(ctx context.Context, task *panel.WorkflowTask) error
}
type TaskProcessor struct {
	config       *Config
	logger       *zap.Logger
	txManager    postgres.TxManager
	taskRepo     TaskRepository
	taskBuilders map[panel.WorkflowTask_Type]TaskWrapperBuilder
}

func NewTaskProcessor(
	config *Config,
	logger *zap.Logger,
	txManager postgres.TxManager,
	taskRepo TaskRepository,
	taskBuilders map[panel.WorkflowTask_Type]TaskWrapperBuilder,
) (*TaskProcessor, error) {
	level, parseErr := zapcore.ParseLevel(config.TaskLoggerLevel)
	if parseErr != nil {
		return nil, parseErr
	}
	logger = logger.WithOptions(zap.IncreaseLevel(level))
	return &TaskProcessor{
		config:       config,
		logger:       logger,
		txManager:    txManager,
		taskRepo:     taskRepo,
		taskBuilders: taskBuilders,
	}, nil
}

func (t *TaskProcessor) dispatchTaskWrapper(
	_ context.Context,
	task *panel.WorkflowTask,
) (TaskWrapper, error) {
	taskType := task.GetTaskType()
	taskWrapperBuilder, ok := t.taskBuilders[taskType]
	if !ok {
		return nil, fmt.Errorf("unknown task type: %v", taskType.String())
	}
	return taskWrapperBuilder(t.logger, task)
}

func (t *TaskProcessor) getNewBackoffState(task *panel.WorkflowTask) *panel.Retry_State {
	task.GetRetrySettings()
	switch task.GetRetrySettings().GetBackoff().GetBackoff().(type) {
	case *panel.Retry_Backoff_Constant_:
		back := backoff.NewConstantBackOff(task.GetRetrySettings().GetBackoff().GetConstant().GetDuration().AsDuration())
		next := back.NextBackOff()
		return &panel.Retry_State{
			BackoffDuration: durationpb.New(next),
			BackoffValue:    timestamppb.New(time.Now().Add(next)),
			Attempt:         task.GetRetryState().GetAttempt() + 1,
		}
	case *panel.Retry_Backoff_Exponential_:
		interval := task.GetRetryState().GetBackoffDuration().AsDuration()
		if interval == 0 {
			interval = task.GetRetrySettings().GetBackoff().GetExponential().GetInitialInterval().AsDuration()
		}
		exp := backoff.NewExponentialBackOff(
			backoff.WithInitialInterval(interval),
			backoff.WithMaxElapsedTime(task.GetRetrySettings().GetBackoff().GetExponential().GetMaxElapsedTime().AsDuration()),
			backoff.WithMaxInterval(task.GetRetrySettings().GetBackoff().GetExponential().GetMaxInterval().AsDuration()),
			backoff.WithMultiplier(task.GetRetrySettings().GetBackoff().GetExponential().GetMultiplier()),
			backoff.WithRandomizationFactor(task.GetRetrySettings().GetBackoff().GetExponential().GetRandomizationFactor()),
			backoff.WithRetryStopDuration(task.GetRetrySettings().GetBackoff().GetExponential().GetRetryStopDuration().AsDuration()),
		)
		exp.Reset()
		next := exp.NextBackOff()
		return &panel.Retry_State{
			BackoffDuration: durationpb.New(next),
			BackoffValue:    timestamppb.New(time.Now().Add(next)),
			Attempt:         task.GetRetryState().GetAttempt(),
		}
	default:
		panic("unknown backoff type")
	}
}

func (t *TaskProcessor) canRetry(task *panel.WorkflowTask) bool {
	if task.GetRetrySettings().GetMaxAttempts() == 0 {
		return true
	}
	return task.GetRetryState().GetAttempt() < task.GetRetrySettings().GetMaxAttempts()
}

func (t *TaskProcessor) processTaskInternal(
	ctx context.Context,
	task *panel.WorkflowTask,
	taskWrapper TaskWrapper,
) {
	switch task.GetStatus() {
	case panel.WorkflowTask_STATUS_PENDING,
		panel.WorkflowTask_STATUS_RETRYING:
		err := taskWrapper.Start(ctx)
		if err != nil {
			if errors.Is(err, ErrStatusTemproraryFailed) {
				taskWrapper.TaskLogger().Warn("temporary failed to start task, still pending for retry", zap.Error(err))
				task.Status = panel.WorkflowTask_STATUS_PENDING
				return
			}
			if t.canRetry(task) {
				taskWrapper.TaskLogger().Warn(
					"task failed, retrying",
					zap.Time("next_exec_in", task.GetRetryState().GetBackoffValue().AsTime()),
					zap.Uint32("attempt", task.GetRetryState().GetAttempt()),
					zap.Duration("backoff_duration", task.GetRetryState().GetBackoffDuration().AsDuration()),
					zap.Error(err),
				)
				task.Status = panel.WorkflowTask_STATUS_RETRYING
				task.RetryState.Attempt = task.GetRetryState().GetAttempt() + 1
				return
			} else {
				taskWrapper.TaskLogger().Error("task failed, no more retries", zap.Error(err))
				task.Status = panel.WorkflowTask_STATUS_FAILED
				return
			}
		}
		task.Status = panel.WorkflowTask_STATUS_RUNNING
		taskWrapper.TaskLogger().Info("task succeeded started")
		return
	case panel.WorkflowTask_STATUS_RUNNING:
		newStatus, err := taskWrapper.Status(ctx)
		if err != nil {
			if errors.Is(err, ErrStatusTemproraryFailed) {
				task.Status = panel.WorkflowTask_STATUS_RUNNING
				taskWrapper.TaskLogger().Warn("temporary failed to get task status, retrying")
				return
			} else {
				task.Status = panel.WorkflowTask_STATUS_FAILED
				taskWrapper.TaskLogger().Error("failed to get task status", zap.Error(err))
				return
			}
		}
		task.Status = newStatus
		taskWrapper.TaskLogger().Debug("task status ping complete")
		return
	case panel.WorkflowTask_STATUS_FAILED,
		panel.WorkflowTask_STATUS_CANCELLED,
		panel.WorkflowTask_STATUS_COMPLETED:
		if !task.GetCleanedUp() {
			err := taskWrapper.Cleanup(ctx)
			if err != nil {
				taskWrapper.TaskLogger().Error("failed to cleanup task", zap.Error(err))
				return
			}
			task.CleanedUp = true
		}
		return
	default:
		panic(fmt.Sprintf("unknown task status: %v", task.GetStatus().String()))
	}
}

func (t *TaskProcessor) processWorkflowTask(ctx context.Context, task *panel.WorkflowTask) error {
	return postgres.WithReadCommitted(ctx, t.txManager, func(ctx context.Context) error {
		taskWrapper, err := t.dispatchTaskWrapper(ctx, task)
		if err != nil {
			return err
		}
		err = t.taskRepo.EnsureWorkflowTaskInput(ctx, task)
		if err != nil {
			t.logger.Error(".ProcessTask failed to ensure task input", zap.Error(err))
			return err
		}
		newBackoffState := t.getNewBackoffState(task)
		task.RetryState = newBackoffState
		t.processTaskInternal(ctx, task, taskWrapper)
		newTaskState, err := taskWrapper.State()
		if err != nil {
			return err
		}
		task.Timing.UpdatedAt = timestamppb.Now()
		task.Input = newTaskState.GetInput()
		task.Output = newTaskState.GetOutput()
		task.OnWorker = ""
		err = t.taskRepo.SaveWorkflowTask(ctx, task)
		if err != nil {
			t.logger.Error(".ProcessTask failed to save task", zap.Error(err))
			return err
		}
		if task.GetStatus() == panel.WorkflowTask_STATUS_COMPLETED {
			err = t.taskRepo.SetChildrenTasksAsPending(ctx, task)
			if err != nil {
				t.logger.Error(".ProcessTask failed to set children tasks as pending", zap.Error(err))
				return err
			}
		}
		return nil
	})
}

const emptyWorkerMarker = ""

func (t *TaskProcessor) processDbTasks(ctx context.Context) error {
	tasks, err := postgres.WithReadCommittedRet(ctx, t.txManager,
		func(ctx context.Context) ([]*panel.WorkflowTask, error) {
			actualTasks, err := t.taskRepo.ListActualTasks(
				ctx,
				emptyWorkerMarker,
				false,
				[]panel.WorkflowTask_Status{
					panel.WorkflowTask_STATUS_PENDING,
					panel.WorkflowTask_STATUS_RETRYING,
					panel.WorkflowTask_STATUS_RUNNING,
				},
			)
			if err != nil && !sqlerr.IsNotFound(err) {
				return nil, err
			}
			cleanedUpTasks, err := t.taskRepo.ListActualTasks(
				ctx,
				emptyWorkerMarker,
				true,
				[]panel.WorkflowTask_Status{
					panel.WorkflowTask_STATUS_COMPLETED,
					panel.WorkflowTask_STATUS_FAILED,
					panel.WorkflowTask_STATUS_CANCELLED,
				},
			)
			if err != nil && !sqlerr.IsNotFound(err) {
				return nil, err
			}
			actualTasks = append(actualTasks, cleanedUpTasks...)
			err = t.taskRepo.SetWorkflowTaskOnWorker(
				ctx,
				lo.Map(actualTasks, func(task *panel.WorkflowTask, _ int) string { return task.GetId() }),
				build.GlobalInstanceId,
			)
			if err != nil {
				return nil, err
			}
			return actualTasks, nil
		})
	if err != nil {
		return err
	}
	taskPool := pool.New().WithContext(ctx)
	for _, task := range tasks {
		taskPool.Go(func(ctx context.Context) error {
			return t.processWorkflowTask(ctx, task)
		})
	}
	return taskPool.Wait()
}

func (t *TaskProcessor) Start() context.CancelFunc {
	ctx, cancel := context.WithCancel(context.Background())
	running := atomic.NewBool(false)
	go func() {
		ticker := time.NewTicker(t.config.PollInterval)
		for {
			select {
			case <-ctx.Done():
				return
			case runTime := <-ticker.C:
				if !running.Swap(true) {
					t.logger.Debug("checking automation status", zap.Time("run_time", runTime))
					err := t.processDbTasks(ctx)
					if err != nil {
						t.logger.Error("failed to check automation status", zap.Error(err))
					}
					running.Store(false)
				} else {
					t.logger.Debug("already running, skipping timer", zap.Time("run_time", runTime))
				}
			}
		}
	}()
	return cancel
}
