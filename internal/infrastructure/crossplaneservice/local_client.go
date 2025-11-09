package crossplaneservice

import (
	"context"
	"net"

	"go.uber.org/zap"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"

	"github.com/stroppy-io/stroppy-cloud-panel/internal/core/logger"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/proto/crossplane"
)

func NewLocalCrossplaneClient(
	serverImpl crossplane.CrossplaneServer,
) (crossplane.CrossplaneClient, context.CancelFunc) {
	buffer := 1024 * 1024
	listener := bufconn.Listen(buffer)

	s := grpc.NewServer()
	crossplane.RegisterCrossplaneServer(s, serverImpl)
	go func() {
		if err := s.Serve(listener); err != nil {
			panic(err)
		}
	}()
	conn, _ := grpc.NewClient(
		"passthrough:///bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
			return listener.Dial()
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)

	return crossplane.NewCrossplaneClient(conn), func() {
		err := conn.Close()
		if err != nil {
			logger.Error("failed to close crossplane client connection", zap.Error(err))
		}
	}
}
