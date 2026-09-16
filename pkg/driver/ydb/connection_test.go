package ydb

import (
	"context"
	"net"
	"strings"
	"sync/atomic"
	"testing"

	discoveryservice "github.com/ydb-platform/ydb-go-genproto/Ydb_Discovery_V1"
	"github.com/ydb-platform/ydb-go-genproto/protos/Ydb_Discovery"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/stroppy-io/stroppy/pkg/config"
	"github.com/stroppy-io/stroppy/pkg/driver"
)

type rejectedDiscovery struct {
	discoveryservice.UnimplementedDiscoveryServiceServer
	calls atomic.Int32
	token atomic.Value
}

func (s *rejectedDiscovery) ListEndpoints(
	ctx context.Context, _ *Ydb_Discovery.ListEndpointsRequest,
) (*Ydb_Discovery.ListEndpointsResponse, error) {
	s.calls.Add(1)

	md, _ := metadata.FromIncomingContext(ctx)
	s.token.Store(strings.Join(md.Get("x-ydb-auth-ticket"), ","))

	return nil, status.Error(codes.PermissionDenied, "explicit identity rejected")
}

func TestExplicitTokenDoesNotFallBackToMetadata(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	service := &rejectedDiscovery{}
	server := grpc.NewServer()
	discoveryservice.RegisterDiscoveryServiceServer(server, service)

	t.Cleanup(server.Stop)
	go func() { _ = server.Serve(listener) }()

	token := "test-explicit-token"

	_, err = NewDriver(t.Context(), driver.Options{
		Config: &config.DriverConfig{URL: "grpc://" + listener.Addr().String() + "/?database=/test", AuthToken: &token},
		Logger: zap.NewNop(),
	})
	if err == nil || !strings.Contains(err.Error(), "explicit identity rejected") {
		t.Fatalf("expected original discovery failure, got %v", err)
	}

	if strings.Contains(err.Error(), "metadata") {
		t.Fatalf("changed explicit identity: %v", err)
	}

	if service.calls.Load() != 1 || service.token.Load() != token {
		t.Fatalf("calls=%d token=%v", service.calls.Load(), service.token.Load())
	}
}

func TestYandexDedicatedEndpointCASelection(t *testing.T) {
	for _, tc := range []struct {
		dsn  string
		want bool
	}{
		{"grpcs://lb.etn123.ydb.mdb.yandexcloud.net:2135/?database=/Root/db", true},
		{"grpcs://LB.ETN123.YDB.MDB.YANDEXCLOUD.NET.:2135", true},
		{"grpc://lb.etn123.ydb.mdb.yandexcloud.net:2135", false},
		{"grpcs://ydb.serverless.yandexcloud.net:2135", false},
		{"grpcs://lb.etn123.ydb.mdb.yandexcloud.net.attacker.example:2135", false},
		{"grpcs://ydb.mdb.yandexcloud.net@attacker.example:2135", false},
		{"grpcs://private.example:2135/?database=.ydb.mdb.yandexcloud.net", false},
		{"%invalid", false},
	} {
		t.Run(tc.dsn, func(t *testing.T) {
			if got := isYandexDedicatedEndpoint(tc.dsn); got != tc.want {
				t.Fatalf("got %v want %v", got, tc.want)
			}
		})
	}
}
