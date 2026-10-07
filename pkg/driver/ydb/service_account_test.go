package ydb

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"net"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v4"
	"github.com/stretchr/testify/require"
	iam "github.com/yandex-cloud/go-genproto/yandex/cloud/iam/v1"
	ycauth "github.com/ydb-platform/ydb-go-yc"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/stroppy-io/stroppy/v6/pkg/config"
	"github.com/stroppy-io/stroppy/v6/pkg/driver"
)

type testIAM struct {
	iam.UnimplementedIamTokenServiceServer
	key   *rsa.PublicKey
	calls atomic.Int32
	fail  atomic.Bool
}

func (s *testIAM) Create(_ context.Context, req *iam.CreateIamTokenRequest) (*iam.CreateIamTokenResponse, error) {
	s.calls.Add(1)

	claims := &jwt.RegisteredClaims{}

	token, err := jwt.ParseWithClaims(req.GetJwt(), claims, func(token *jwt.Token) (any, error) {
		return s.key, nil
	}, jwt.WithValidMethods([]string{"PS256"}))
	if err != nil || !token.Valid || claims.Issuer != "test-service-account" ||
		!claims.VerifyAudience("https://iam.api.cloud.yandex.net/iam/v1/tokens", true) || token.Header["kid"] != "test-key" {
		return nil, status.Error(codes.Unauthenticated, "invalid test identity")
	}

	if s.fail.Load() {
		return nil, status.Error(codes.Unavailable, "test IAM temporarily unavailable")
	}

	value := "first-test-token"
	if s.calls.Load() > 1 {
		value = "renewed-test-token"
	}

	return &iam.CreateIamTokenResponse{IamToken: value, ExpiresAt: timestamppb.New(time.Now().Add(2 * time.Second))}, nil
}

func TestServiceAccountSDKRenewsCredentialsAndPreservesIdentity(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	keyJSON, err := json.Marshal(map[string]string{
		"id": "test-key", "service_account_id": "test-service-account",
		"private_key": string(pem.EncodeToMemory(&pem.Block{
			Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key),
		})),
	})
	require.NoError(t, err)
	keyFile := filepath.Join(t.TempDir(), "sa.json")
	require.NoError(t, os.WriteFile(keyFile, keyJSON, 0o600))

	service := &testIAM{key: &key.PublicKey}
	server := grpc.NewServer()
	iam.RegisterIamTokenServiceServer(server, service)
	t.Cleanup(server.Stop)
	endpoint := httptest.NewUnstartedServer(server)
	endpoint.EnableHTTP2 = true
	endpoint.StartTLS()
	t.Cleanup(endpoint.Close)

	pool := x509.NewCertPool()
	pool.AddCert(endpoint.Certificate())
	creds, err := ycauth.NewClient(ycauth.WithServiceFile(keyFile),
		ycauth.WithEndpoint(strings.TrimPrefix(endpoint.URL, "https://")),
		ycauth.WithCertPool(pool), ycauth.WithInsecureSkipVerify(false))
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()

	first, err := creds.Token(ctx)
	require.NoError(t, err)
	require.Equal(t, "first-test-token", first)

	cached, err := creds.Token(ctx)
	require.NoError(t, err)
	require.Equal(t, first, cached)
	require.EqualValues(t, 1, service.calls.Load())
	require.NotContains(t, fmt.Sprint(creds), "PRIVATE KEY")
	service.fail.Store(true)
	// The SDK renews at half of the issuer's TTL. No custom refresh loop.
	require.Eventually(t, func() bool {
		_, err := creds.Token(ctx)

		return err != nil
	}, 5*time.Second, 20*time.Millisecond)
	service.fail.Store(false)

	renewed, err := creds.Token(ctx)
	require.NoError(t, err)
	require.Equal(t, "renewed-test-token", renewed)
	require.GreaterOrEqual(t, service.calls.Load(), int32(3))
}

func TestExplicitServiceAccountKeyDoesNotFallBackToMetadata(t *testing.T) {
	keyFile := filepath.Join(t.TempDir(), "missing-key.json")
	_, err := NewDriver(t.Context(), driver.Options{
		Config: &config.DriverConfig{URL: "grpcs://127.0.0.1:1/?database=/test", ServiceAccountKeyFile: &keyFile},
		Logger: zap.NewNop(),
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "missing-key.json")
	require.NotContains(t, err.Error(), "yc metadata fallback")
}

func TestServiceAccountKeyRejectsUnsafeDatabaseTransportOrCredentials(t *testing.T) {
	for _, tt := range []struct {
		name       string
		url        string
		skipVerify bool
		want       string
	}{
		{name: "plaintext", url: "grpc://127.0.0.1:1/test", want: "grpcs://"},
		{name: "missing scheme", url: "127.0.0.1:1/test", want: "grpcs://"},
		{name: "wrong scheme", url: "https://127.0.0.1:1/test", want: "grpcs://"},
		{name: "missing host", url: "grpcs:///test", want: "grpcs://"},
		{name: "malformed", url: "grpcs://%/test", want: "grpcs://"},
		{name: "URL user", url: "grpcs://user@127.0.0.1:1/test", want: "URL-embedded credentials"},
		{name: "URL user and password", url: "grpcs://user:password@127.0.0.1:1/test", want: "URL-embedded credentials"},
		{name: "unverified TLS", url: "grpcs://127.0.0.1:1/test", skipVerify: true, want: "certificate verification"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			keyFile := filepath.Join(t.TempDir(), "must-not-read-key.json")

			var dials atomic.Int32

			_, err := NewDriver(t.Context(), driver.Options{
				Config: &config.DriverConfig{
					URL: tt.url, ServiceAccountKeyFile: &keyFile, TLSInsecureSkipVerify: &tt.skipVerify,
				},
				Logger: zap.NewNop(),
				DialFunc: func(context.Context, string, string) (net.Conn, error) {
					dials.Add(1)

					return nil, errors.New("unexpected network connection")
				},
			})
			require.ErrorContains(t, err, tt.want)
			require.NotContains(t, err.Error(), "must-not-read-key")
			require.Zero(t, dials.Load())
		})
	}
}
