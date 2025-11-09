package api

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"

	"github.com/stroppy-io/stroppy-cloud-panel/internal/automate"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/core/logger"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/core/token"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/entity/claims"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/entity/resource"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/infrastructure/orm"
	postgres "github.com/stroppy-io/stroppy-cloud-panel/internal/infrastructure/postgresql"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/proto/crossplane"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/proto/panel"
	"github.com/stroppy-io/stroppy-cloud-panel/tools/sql/migrations"
)

const (
	testEmail          = "test@example.com"
	testPassword       = "Password123@"
	defaultPostgresUrl = "postgres://postgres:developer@localhost:5432/postgres"
)

type MockCrossplaneClient struct {
	resources map[string]*crossplane.ResourceWithStatus
}

func (m MockCrossplaneClient) CreateResource(ctx context.Context, in *crossplane.CreateResourceRequest, opts ...grpc.CallOption) (*crossplane.ResourceWithStatus, error) {
	res, ok := m.resources[in.GetResource().GetMetadata().GetName()]
	if !ok {
		return nil, fmt.Errorf("resource not found: %s", in.GetResource().GetMetadata().GetName())
	}
	return res, nil
}

func (m MockCrossplaneClient) CreateResourcesMany(ctx context.Context, in *crossplane.CreateResourcesManyRequest, opts ...grpc.CallOption) (*crossplane.CreateResourcesManyResponse, error) {
	responses := make([]*crossplane.ResourceWithStatus, len(in.GetResources()))
	for i, res := range in.GetResources() {
		resp, err := m.CreateResource(ctx, &crossplane.CreateResourceRequest{
			Resource: res,
		})
		if err != nil {
			return nil, err
		}
		responses[i] = resp
	}
	return &crossplane.CreateResourcesManyResponse{
		Responses: responses,
	}, nil
}

func (m MockCrossplaneClient) GetResourceStatus(ctx context.Context, in *crossplane.GetResourceStatusRequest, opts ...grpc.CallOption) (*crossplane.GetResourceStatusResponse, error) {
	return &crossplane.GetResourceStatusResponse{
		Synced:     true,
		Ready:      true,
		ExternalId: "test-id",
	}, nil
}

func (m MockCrossplaneClient) DeleteResource(ctx context.Context, in *crossplane.DeleteResourceRequest, opts ...grpc.CallOption) (*crossplane.DeleteResourceResponse, error) {
	return &crossplane.DeleteResourceResponse{
		Synced: false,
	}, nil
}

func (m MockCrossplaneClient) DeleteResourcesMany(ctx context.Context, in *crossplane.DeleteResourcesManyRequest, opts ...grpc.CallOption) (*crossplane.DeleteResourcesManyResponse, error) {
	responses := make([]*crossplane.DeleteResourceResponse, len(in.GetRefs()))
	for i := range in.GetRefs() {
		responses[i] = &crossplane.DeleteResourceResponse{
			Synced: false,
		}
	}
	return &crossplane.DeleteResourcesManyResponse{
		Responses: responses,
	}, nil
}

func newDevTestService(t *testing.T) (*PanelService, context.Context, *panel.User) {
	envUrl, exists := os.LookupEnv("STROPPY_CLOUD_PANEL_DB_URL")
	if !exists {
		envUrl = defaultPostgresUrl
	}
	pool, err := postgres.NewFromString(envUrl, "debug")
	require.NoError(t, err)
	_, err = pool.Exec(t.Context(), "DROP TABLE IF EXISTS atlas_migrations, users, run_records, cloud_resources, cloud_automations, stroppy_runs, stroppy_steps")
	require.NoError(t, err)
	err = postgres.MigrateAtlas(pool, migrations.Content)
	require.NoError(t, err)
	txExecutor, txManager, err := postgres.NewTxFlow(pool, postgres.ReadUncommittedSettings())
	require.NoError(t, err)
	actor := token.NewTokenActor(&token.Config{
		HmacSecretKey: "secret_key",
		LeewaySeconds: 30,
		Issuer:        "stroppy-cloud-panel",
		AccessExpire:  1 * time.Hour,
		RefreshExpire: 30 * 24 * time.Hour,
	})
	k8sConfig := &automate.K8SConfig{
		KubeconfigPath: "./kubeconfig.yaml",
		Crossplane: automate.CrossplaneConfig{
			YandexCloudProviderConfig: resource.YandexCloudProviderConfig{},
		},
	}

	service := NewPanelService(
		logger.Global(),
		txExecutor,
		txManager,
		actor,
		k8sConfig,
		&CloudAutomationConfig{
			AutomationTTL:   4 * time.Hour,
			CreationTimeout: 15 * time.Minute,
		},
		&MockCrossplaneClient{},
	)

	err = service.usersRepo.Exec(t.Context(), orm.User.Delete())
	require.NoError(t, err)
	_, err = service.Register(t.Context(), &panel.RegisterRequest{
		Email:    testEmail,
		Password: testPassword,
	})
	require.NoError(t, err)

	resp, err := service.Login(t.Context(), &panel.LoginRequest{
		Email:    testEmail,
		Password: testPassword,
	})
	require.NoError(t, err)
	require.NotNil(t, resp.AccessToken)

	user, err := service.usersRepo.GetBy(t.Context(), orm.User.SelectAll().Where(orm.User.Email.Eq(testEmail)))
	require.NoError(t, err)

	return service, token.CtxWithAccount[claims.Claims](t.Context(), &claims.Claims{
		UserID: user.GetId().GetId(),
	}), user
}
