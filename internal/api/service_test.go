package api

import (
	"context"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/core/logger"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/entity/claims"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/proto/panel"
	"github.com/stroppy-io/stroppy-cloud-panel/tools/sql/migrations"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/core/token"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/infrastructure/orm"
	postgres "github.com/stroppy-io/stroppy-cloud-panel/internal/infrastructure/postgresql"
)

const (
	testEmail          = "test@example.com"
	testPassword       = "Password123@"
	defaultPostgresUrl = "postgres://postgres:developer@localhost:5432/postgres"
)

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
	service := NewPanelService(
		logger.Global(),
		txExecutor,
		txManager,
		actor,
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
