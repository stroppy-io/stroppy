package api

import (
	"connectrpc.com/connect"
	"context"
	"errors"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/automate"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/core/token"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/entity/claims"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/infrastructure/orm"
	postgres "github.com/stroppy-io/stroppy-cloud-panel/internal/infrastructure/postgresql"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/infrastructure/postgresql/sqlerr"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/infrastructure/postgresql/sqlexec"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/infrastructure/sqlc"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/proto/crossplane"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/proto/panel"
	"go.uber.org/zap"
)

type PanelService struct {
	*panel.UnimplementedAccountServiceServer
	*panel.UnimplementedAutomateServiceServer
	*panel.UnimplementedRunServiceServer
	*panel.UnimplementedResourcesServiceServer

	logger     *zap.Logger
	executor   sqlexec.Executor
	txManager  postgres.TxManager
	tokenActor *token.Actor

	usersRepo        orm.UserRepository
	stroppyStepsRepo orm.StroppyStepRepository
	runRecordRepo    orm.RunRecordRepository

	cloudAutomationRepo orm.CloudAutomationRepository
	cloudResourceRepo   orm.CloudResourceRepository
	stroppyRunRepo      orm.StroppyRunRepository
	stroppyStepRepo     orm.StroppyStepRepository

	sqlcRepo          sqlc.Querier
	k8sConfig         *automate.K8SConfig
	automateConfig    *CloudAutomationConfig
	crossplaneService crossplane.CrossplaneClient
}

func NewPanelService(
	logger *zap.Logger,
	executor sqlexec.Executor,
	txManager postgres.TxManager,
	tokenActor *token.Actor,
	k8sConfig *automate.K8SConfig,
	automateConfig *CloudAutomationConfig,
	crossplaneService crossplane.CrossplaneClient,
) *PanelService {
	return &PanelService{
		UnimplementedAccountServiceServer:   &panel.UnimplementedAccountServiceServer{},
		UnimplementedAutomateServiceServer:  &panel.UnimplementedAutomateServiceServer{},
		UnimplementedRunServiceServer:       &panel.UnimplementedRunServiceServer{},
		UnimplementedResourcesServiceServer: &panel.UnimplementedResourcesServiceServer{},
		logger:                              logger,

		executor:   executor,
		txManager:  txManager,
		tokenActor: tokenActor,

		usersRepo:           NewUsersRepository(executor),
		stroppyStepsRepo:    NewStroppyStepsRepository(executor),
		runRecordRepo:       NewRunRecordRepository(executor),
		cloudAutomationRepo: NewCloudAutomationRepository(executor),
		cloudResourceRepo:   NewCloudResourceRepository(executor),
		stroppyRunRepo:      NewStroppyRunRepository(executor),
		stroppyStepRepo:     NewStroppyStepRepository(executor),
		sqlcRepo:            sqlc.New(executor),

		k8sConfig:         k8sConfig,
		automateConfig:    automateConfig,
		crossplaneService: crossplaneService,
	}
}

func (p *PanelService) getUserFromCtx(ctx context.Context) (*panel.User, error) {
	claim, err := token.AccountFromCtx[claims.Claims](ctx)
	if err != nil {
		return nil, err
	}
	user, err := p.usersRepo.GetBy(ctx, orm.User.SelectAll().Where(orm.User.Id.Eq(claim.UserID)))
	if err != nil {
		if sqlerr.IsNotFound(err) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("user not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return user, nil
}
