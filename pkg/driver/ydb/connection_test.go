package ydb

import (
	"context"
	"errors"
	"net"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	authservice "github.com/ydb-platform/ydb-go-genproto/Ydb_Auth_V1"
	discoveryservice "github.com/ydb-platform/ydb-go-genproto/Ydb_Discovery_V1"
	"github.com/ydb-platform/ydb-go-genproto/protos/Ydb"
	"github.com/ydb-platform/ydb-go-genproto/protos/Ydb_Auth"
	"github.com/ydb-platform/ydb-go-genproto/protos/Ydb_Discovery"
	"github.com/ydb-platform/ydb-go-genproto/protos/Ydb_Operations"
	ydbsdk "github.com/ydb-platform/ydb-go-sdk/v3"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/stroppy-io/stroppy/v6/pkg/config"
	"github.com/stroppy-io/stroppy/v6/pkg/driver"
)

type rejectedDiscovery struct {
	discoveryservice.UnimplementedDiscoveryServiceServer
	calls         atomic.Int32
	token         atomic.Value
	transportCode codes.Code
	operationCode Ydb.StatusIds_StatusCode
	blockFallback bool
}

func (s *rejectedDiscovery) ListEndpoints(
	ctx context.Context, _ *Ydb_Discovery.ListEndpointsRequest,
) (*Ydb_Discovery.ListEndpointsResponse, error) {
	s.calls.Add(1)

	md, _ := metadata.FromIncomingContext(ctx)
	token := strings.Join(md.Get("x-ydb-auth-ticket"), ",")
	s.token.Store(token)

	if s.blockFallback && token == "test-metadata-token" {
		<-ctx.Done()

		return nil, status.FromContextError(ctx.Err()).Err()
	}

	if s.operationCode != Ydb.StatusIds_STATUS_CODE_UNSPECIFIED {
		return &Ydb_Discovery.ListEndpointsResponse{
			Operation: &Ydb_Operations.Operation{Ready: true, Status: s.operationCode},
		}, nil
	}

	code := s.transportCode
	if code == codes.OK {
		code = codes.PermissionDenied
	}

	return nil, status.Error(code, "explicit identity rejected")
}

type rejectedLogin struct {
	authservice.UnimplementedAuthServiceServer
}

func (*rejectedLogin) Login(context.Context, *Ydb_Auth.LoginRequest) (*Ydb_Auth.LoginResponse, error) {
	return nil, status.Error(codes.Unauthenticated, "explicit login rejected")
}

func startRejectedDiscovery(t *testing.T, service *rejectedDiscovery) string {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	server := grpc.NewServer()
	discoveryservice.RegisterDiscoveryServiceServer(server, service)
	authservice.RegisterAuthServiceServer(server, &rejectedLogin{})

	t.Cleanup(server.Stop)
	go func() { _ = server.Serve(listener) }()

	return listener.Addr().String()
}

func TestExplicitTokenDoesNotFallBackToMetadata(t *testing.T) {
	service := &rejectedDiscovery{}
	address := startRejectedDiscovery(t, service)

	token := "test-explicit-token"

	_, err := NewDriver(t.Context(), driver.Options{
		Config: &config.DriverConfig{URL: "grpc://" + address + "/?database=/test", AuthToken: &token},
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

func TestTransportFailureDoesNotFallBackToMetadata(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name          string
		scheme        string
		code          codes.Code
		operationCode Ydb.StatusIds_StatusCode
	}{
		{name: "TLS to plaintext", scheme: "grpcs"},
		{name: "unavailable", scheme: "grpc", code: codes.Unavailable},
		{name: "internal", scheme: "grpc", code: codes.Internal},
		{name: "YDB unavailable", scheme: "grpc", operationCode: Ydb.StatusIds_UNAVAILABLE},
		{name: "YDB bad request", scheme: "grpc", operationCode: Ydb.StatusIds_BAD_REQUEST},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			address := startRejectedDiscovery(t, &rejectedDiscovery{
				transportCode: tc.code, operationCode: tc.operationCode,
			})

			ctx, cancel := context.WithTimeout(t.Context(), 2*primaryConnectTimeout)
			defer cancel()

			var fallbacks int

			_, err := newDriver(ctx, driver.Options{
				Config: &config.DriverConfig{URL: tc.scheme + "://" + address + "/?database=/test"},
				Logger: zap.NewNop(),
			}, func() ydbsdk.Option {
				fallbacks++

				return ydbsdk.WithAccessTokenCredentials("test-metadata-token")
			})
			if err == nil || fallbacks != 0 || strings.Contains(err.Error(), "yc metadata fallback") {
				t.Fatalf("transport failure changed identity: fallbacks=%d err=%v", fallbacks, err)
			}
		})
	}
}

func TestAuthFallbackHasOwnDeadline(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name          string
		transportCode codes.Code
		operationCode Ydb.StatusIds_StatusCode
	}{
		{name: "unauthenticated", transportCode: codes.Unauthenticated},
		{name: "permission denied", transportCode: codes.PermissionDenied},
		{name: "YDB unauthorized", operationCode: Ydb.StatusIds_UNAUTHORIZED},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			service := &rejectedDiscovery{
				transportCode: tc.transportCode, operationCode: tc.operationCode, blockFallback: true,
			}
			address := startRejectedDiscovery(t, service)

			ctx, cancel := context.WithTimeout(t.Context(), 2*primaryConnectTimeout+time.Second)
			defer cancel()

			var fallbacks int

			started := time.Now()

			_, err := newDriver(ctx, driver.Options{
				Config: &config.DriverConfig{URL: "grpc://" + address + "/?database=/test"},
				Logger: zap.NewNop(),
			}, func() ydbsdk.Option {
				fallbacks++

				return ydbsdk.WithAccessTokenCredentials("test-metadata-token")
			})
			if err == nil || fallbacks != 1 || !strings.Contains(err.Error(), "yc metadata fallback") {
				t.Fatalf("expected failed metadata attempt: fallbacks=%d err=%v", fallbacks, err)
			}

			if !strings.Contains(err.Error(), "explicit identity rejected") &&
				!ydbsdk.IsOperationError(err, Ydb.StatusIds_UNAUTHORIZED) {
				t.Fatalf("lost primary auth error: %v", err)
			}

			if elapsed := time.Since(started); elapsed > primaryConnectTimeout+2*time.Second {
				t.Fatalf("fallback used caller deadline instead of its own: elapsed=%v err=%v", elapsed, err)
			}

			if service.token.Load() != "test-metadata-token" {
				t.Fatalf("fallback did not reach server: token=%v err=%v", service.token.Load(), err)
			}
		})
	}
}

func TestExplicitCredentialsDoNotFallBackToMetadata(t *testing.T) {
	t.Parallel()

	user, password, token := "test-user", "test-password", "test-token"
	for _, tc := range []struct {
		name        string
		credentials config.DriverConfig
		urlUser     string
	}{
		{name: "token", credentials: config.DriverConfig{AuthToken: &token}},
		{name: "static", credentials: config.DriverConfig{AuthUser: &user, AuthPassword: &password}},
		{name: "password only", credentials: config.DriverConfig{AuthPassword: &password}},
		{name: "URL user", urlUser: user + "@"},
		{name: "URL password", urlUser: user + ":" + password + "@"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			address := startRejectedDiscovery(t, &rejectedDiscovery{})
			cfg := tc.credentials
			cfg.URL = "grpc://" + tc.urlUser + address + "/?database=/test"

			ctx, cancel := context.WithTimeout(t.Context(), 2*primaryConnectTimeout)
			defer cancel()

			var fallbacks int

			_, err := newDriver(ctx, driver.Options{Config: &cfg, Logger: zap.NewNop()}, func() ydbsdk.Option {
				fallbacks++

				return ydbsdk.WithAccessTokenCredentials("test-metadata-token")
			})
			if err == nil || fallbacks != 0 || strings.Contains(err.Error(), "yc metadata fallback") {
				t.Fatalf("explicit identity changed: fallbacks=%d err=%v", fallbacks, err)
			}
		})
	}
}

func TestRefusedConnectionDoesNotFallBackToMetadata(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 2*primaryConnectTimeout)
	defer cancel()

	var fallbacks int

	_, err := newDriver(ctx, driver.Options{
		Config: &config.DriverConfig{URL: "grpc://unused.invalid:2136/?database=/test"},
		Logger: zap.NewNop(),
		DialFunc: func(context.Context, string, string) (net.Conn, error) {
			return nil, errors.New("test connection refused")
		},
	}, func() ydbsdk.Option {
		fallbacks++

		return ydbsdk.WithAccessTokenCredentials("test-metadata-token")
	})
	if err == nil || fallbacks != 0 || strings.Contains(err.Error(), "yc metadata fallback") {
		t.Fatalf("refused connection changed identity: fallbacks=%d err=%v", fallbacks, err)
	}
}

func TestAuthFallbackRespectsCallerCancellation(t *testing.T) {
	service := &rejectedDiscovery{blockFallback: true}
	address := startRejectedDiscovery(t, service)

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	var fallbacks int

	_, err := newDriver(ctx, driver.Options{
		Config: &config.DriverConfig{URL: "grpc://" + address + "/?database=/test"},
		Logger: zap.NewNop(),
	}, func() ydbsdk.Option {
		fallbacks++

		cancel()

		return ydbsdk.WithAccessTokenCredentials("test-metadata-token")
	})
	if !errors.Is(err, context.Canceled) || fallbacks != 1 {
		t.Fatalf("fallback ignored cancellation: fallbacks=%d err=%v", fallbacks, err)
	}
}

func TestCanceledConnectionDoesNotFallBackToMetadata(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	var fallbacks int

	_, err := newDriver(ctx, driver.Options{
		Config: &config.DriverConfig{URL: "grpc://unused.invalid:2136/?database=/test"},
		Logger: zap.NewNop(),
		DialFunc: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return nil, ctx.Err()
		},
	}, func() ydbsdk.Option {
		fallbacks++

		return ydbsdk.WithAccessTokenCredentials("test-metadata-token")
	})
	if !errors.Is(err, context.Canceled) || fallbacks != 0 {
		t.Fatalf("cancellation changed identity: fallbacks=%d err=%v", fallbacks, err)
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
