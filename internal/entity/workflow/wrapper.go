package workflow

import (
	"context"

	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"

	"github.com/stroppy-io/stroppy-cloud-panel/internal/core/protohelp"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/proto/panel"
)

type serializerImpl[I, O proto.Message] struct{}

func NewSerializer[I, O proto.Message]() IOSerializer[I, O] {
	return &serializerImpl[I, O]{}
}

func (s *serializerImpl[I, O]) ToInput(ctx context.Context, any *anypb.Any) (I, error) {
	input := protohelp.ProtoNew[I]()
	err := anypb.UnmarshalTo(any, input, proto.UnmarshalOptions{})
	return input, err
}

func (s *serializerImpl[I, O]) ToOutput(ctx context.Context, any *anypb.Any) (O, error) {
	output := protohelp.ProtoNew[O]()
	err := anypb.UnmarshalTo(any, output, proto.UnmarshalOptions{})
	return output, err
}

type taskStateImpl[I, O proto.Message] struct {
	input  I
	output O
}

func (t *taskStateImpl[I, O]) GetInput() I {
	return t.input
}

func (t *taskStateImpl[I, O]) GetOutput() O {
	return t.output
}

func NewTaskBuilder[I, O proto.Message](handler TaskHandler[I, O]) TaskWrapperBuilder {
	return func(logger *zap.Logger, task *panel.WorkflowTask) (TaskWrapper, error) {
		wrapper, err := newTaskHandlerWrapper[I, O](logger, task, handler)
		if err != nil {
			return nil, err
		}
		return wrapper, nil
	}
}

type taskHandlerWrapper[I, O proto.Message] struct {
	task    *panel.WorkflowTask
	state   TaskState[I, O]
	handler TaskHandler[I, O]
	logger  *TaskLogger
}

func newTaskHandlerWrapper[I, O proto.Message](
	logger *zap.Logger,
	task *panel.WorkflowTask,
	handler TaskHandler[I, O],
) (*taskHandlerWrapper[I, O], error) {
	taskLogger := NewTaskLogger(logger, task)
	serializer := NewSerializer[I, O]()
	input, err := serializer.ToInput(context.Background(), task.GetInput())
	if err != nil {
		return nil, err
	}
	output, err := serializer.ToOutput(context.Background(), task.GetOutput())
	if err != nil {
		return nil, err
	}
	return &taskHandlerWrapper[I, O]{
		task: task,
		state: &taskStateImpl[I, O]{
			input:  input,
			output: output,
		},
		handler: handler,
		logger:  taskLogger,
	}, nil
}

func (t *taskHandlerWrapper[I, O]) Start(ctx context.Context) error {
	output, err := t.handler.Start(ctx, t.state.GetInput())
	if err != nil {
		return err
	}
	t.state = &taskStateImpl[I, O]{
		input:  t.state.GetInput(),
		output: output,
	}
	return nil
}

func (t *taskHandlerWrapper[I, O]) Status(ctx context.Context) (panel.WorkflowTask_Status, error) {
	return t.handler.Status(ctx, t.state)
}

func (t *taskHandlerWrapper[I, O]) Cleanup(ctx context.Context) error {
	return t.handler.Cleanup(ctx, t.state)
}

func (t *taskHandlerWrapper[I, O]) TaskLogger() *TaskLogger {
	return t.logger
}

func (t *taskHandlerWrapper[I, O]) State() (TaskState[*anypb.Any, *anypb.Any], error) {
	input, err := anypb.New(t.state.GetInput())
	if err != nil {
		return nil, err
	}
	output, err := anypb.New(t.state.GetOutput())
	if err != nil {
		return nil, err
	}
	return &taskStateImpl[*anypb.Any, *anypb.Any]{
		input:  input,
		output: output,
	}, nil
}
