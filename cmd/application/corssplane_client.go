package application

import (
	"context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"

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
		"",
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)

	return crossplane.NewCrossplaneClient(conn), cancel
}
