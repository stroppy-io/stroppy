package xk6air

import (
	"context"

	"github.com/stroppy-io/stroppy/pkg/common/proto/stroppy"
	"github.com/stroppy-io/stroppy/pkg/common/proto/stroppy/stroppyconnect"
	"go.uber.org/zap"
	"google.golang.org/protobuf/types/known/emptypb"
)

// cloudClientWrapper wraps the cloud client to log and handle errors
type cloudClientWrapper struct {
	client stroppyconnect.CloudStatusServiceClient
	lg     *zap.Logger
}

var _ stroppyconnect.CloudStatusServiceClient = (*cloudClientWrapper)(nil)

func (w *cloudClientWrapper) NotifyRun(ctx context.Context, run *stroppy.StroppyRun) (*emptypb.Empty, error) {
	w.lg.Info("notifying run status",
		zap.String("run_id", run.Id.Value),
		zap.String("status", run.Status.String()),
	)

	resp, err := w.client.NotifyRun(ctx, run)
	if err != nil {
		w.lg.Error("failed to notify run",
			zap.String("run_id", run.Id.Value),
			zap.String("status", run.Status.String()),
			zap.Error(err),
		)
		return nil, err
	}
	return resp, nil
}

func (w *cloudClientWrapper) NotifyStep(ctx context.Context, step *stroppy.StroppyStepRun) (*emptypb.Empty, error) {
	w.lg.Info("notifying step status",
		zap.String("step_id", step.Id.Value),
		zap.String("status", step.Status.String()),
	)

	resp, err := w.client.NotifyStep(ctx, step)
	if err != nil {
		w.lg.Error("failed to notify step",
			zap.String("step_id", step.Id.Value),
			zap.String("status", step.Status.String()),
			zap.Error(err),
		)
		return nil, err
	}
	return resp, nil
}

// noopCloudClient is a no-op implementation of the cloud client interface
type noopCloudClient struct{}

var _ stroppyconnect.CloudStatusServiceClient = (*noopCloudClient)(nil)

func (n *noopCloudClient) NotifyRun(ctx context.Context, run *stroppy.StroppyRun) (*emptypb.Empty, error) {
	return &emptypb.Empty{}, nil
}

func (n *noopCloudClient) NotifyStep(ctx context.Context, step *stroppy.StroppyStepRun) (*emptypb.Empty, error) {
	return &emptypb.Empty{}, nil
}
