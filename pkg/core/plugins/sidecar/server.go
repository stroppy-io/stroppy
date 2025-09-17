package sidecar

import (
	"context"

	"github.com/hashicorp/go-plugin"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/stroppy-io/stroppy/pkg/core/logger"
	"github.com/stroppy-io/stroppy/pkg/core/plugins/common"
	stroppy "github.com/stroppy-io/stroppy/pkg/core/proto"
)

type server struct {
	impl Plugin
	*stroppy.UnimplementedSidecarPluginServer
}

func newDriverServer(impl Plugin) *server {
	return &server{
		impl:                             impl,
		UnimplementedSidecarPluginServer: &stroppy.UnimplementedSidecarPluginServer{},
	}
}

func (s server) Initialize(
	ctx context.Context,
	context *stroppy.StepContext,
) (*emptypb.Empty, error) {
	return &emptypb.Empty{}, s.impl.Initialize(ctx, context)
}

func (s server) OnStepStart(
	ctx context.Context,
	event *stroppy.StepContext,
) (*emptypb.Empty, error) {
	err := s.impl.OnStepStart(ctx, event)

	return &emptypb.Empty{}, err
}

func (s server) OnStepEnd(
	ctx context.Context,
	event *stroppy.StepContext,
) (*emptypb.Empty, error) {
	err := s.impl.OnStepEnd(ctx, event)

	return &emptypb.Empty{}, err
}

func (s server) Teardown(
	ctx context.Context,
	_ *emptypb.Empty,
) (*emptypb.Empty, error) {
	return &emptypb.Empty{}, s.impl.Teardown(ctx)
}

func ServePlugin(impl Plugin) {
	plugin.Serve(&plugin.ServeConfig{
		HandshakeConfig: PluginHandshake,
		Plugins: map[string]plugin.Plugin{
			PluginName: NewSharedPlugin(impl),
		},
		// A non-nil value here enables gRPC serving for this plugin...
		GRPCServer: plugin.DefaultGRPCServer,
		Logger:     common.NewLogger(logger.NewFromEnv()),
	})
}
