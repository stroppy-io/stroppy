package workflow

import (
	"context"
	"fmt"

	"github.com/stroppy-io/stroppy-cloud-panel/internal/proto/panel"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
)

var (
	ErrStatusTemproraryFailed = fmt.Errorf("status status temprorary failed")
)

type IOSerializer[I, O proto.Message] interface {
	ToInput(ctx context.Context, input *anypb.Any) (I, error)
	ToOutput(ctx context.Context, output *anypb.Any) (O, error)
}

type TaskState[I, O proto.Message] interface {
	GetInput() I
	GetOutput() O
}

type TaskHandler[I, O proto.Message] interface {
	Start(ctx context.Context, input I) (output O, err error)
	Status(ctx context.Context, state TaskState[I, O]) (panel.WorkflowTask_Status, error)
	Cleanup(ctx context.Context, state TaskState[I, O]) (err error)
}

type TaskWrapperBuilder func(logger *zap.Logger, task *panel.WorkflowTask) (TaskWrapper, error)

type TaskWrapper interface {
	Start(ctx context.Context) error
	Status(ctx context.Context) (panel.WorkflowTask_Status, error)
	Cleanup(ctx context.Context) error
	State() (TaskState[*anypb.Any, *anypb.Any], error)
	TaskLogger() *TaskLogger
}
