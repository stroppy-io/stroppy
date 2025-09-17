package driver

import (
	"context"
	"errors"

	"github.com/hashicorp/go-plugin"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/stroppy-io/stroppy-core/pkg/logger"
	"github.com/stroppy-io/stroppy-core/pkg/plugins/common"
	stroppy "github.com/stroppy-io/stroppy-core/pkg/proto"
	"github.com/stroppy-io/stroppy-core/pkg/utils/errchan"
)

type server struct {
	impl Plugin
	*stroppy.UnimplementedDriverPluginServer
}

func newDriverServer(impl Plugin) *server {
	return &server{
		impl:                            impl,
		UnimplementedDriverPluginServer: &stroppy.UnimplementedDriverPluginServer{},
	}
}

func (s server) Initialize(
	ctx context.Context,
	context *stroppy.StepContext,
) (*emptypb.Empty, error) {
	return &emptypb.Empty{}, s.impl.Initialize(ctx, context)
}

func (s server) BuildTransactionsFromUnit(
	ctx context.Context,
	context *stroppy.UnitBuildContext,
) (*stroppy.DriverTransactionList, error) {
	return s.impl.BuildTransactionsFromUnit(ctx, context)
}

func (s server) BuildTransactionsFromUnitStream(
	context *stroppy.UnitBuildContext,
	stream grpc.ServerStreamingServer[stroppy.DriverTransaction],
) error {
	innerStream, err := s.impl.BuildTransactionsFromUnitStream(stream.Context(), context)
	if err != nil {
		return err
	}

	for {
		data, err := errchan.ReceiveCtx[stroppy.DriverTransaction](stream.Context(), innerStream)
		if err != nil {
			if errors.Is(err, errchan.ErrReceiveClosed) {
				return nil
			}

			return err
		}

		err = stream.Send(data)
		if err != nil {
			return err
		}
	}
}

func (s server) RunTransaction(
	ctx context.Context,
	transaction *stroppy.DriverTransaction,
) (*emptypb.Empty, error) {
	return &emptypb.Empty{}, s.impl.RunTransaction(ctx, transaction)
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
