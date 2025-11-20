package api

import (
	"context"
	"errors"

	"connectrpc.com/connect"
	"go.uber.org/zap"

	"github.com/stroppy-io/stroppy-cloud-panel/internal/api/repositories"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/core/token"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/entity/claims"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/infrastructure/orm"
	postgres "github.com/stroppy-io/stroppy-cloud-panel/internal/infrastructure/postgresql"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/infrastructure/postgresql/sqlerr"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/infrastructure/postgresql/sqlexec"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/infrastructure/sqlc"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/proto/panel"
)

type WorkflowRepository interface {
	GetWorkflow(ctx context.Context, id *panel.Ulid) (*panel.Workflow, error)
	CreateWorkflow(ctx context.Context, workflow *panel.Workflow) error
	MarkWorkflowAsCanceled(ctx context.Context, id *panel.Ulid) (*panel.Workflow, error)
	//DeleteWorkflow(ctx context.Context, id *panel.Ulid) error
}

type PanelService struct {
	*panel.UnimplementedAccountServiceServer
	*panel.UnimplementedRunServiceServer
	*panel.UnimplementedWorkflowServiceServer
	*panel.UnimplementedTemplateServiceServer
	*panel.UnimplementedKvServiceServer

	logger     *zap.Logger
	executor   sqlexec.Executor
	txManager  postgres.TxManager
	tokenActor *token.Actor

	usersRepo          orm.UserRepository
	refreshTokensRepo  orm.RefreshTokensRepository
	runRecordRepo      orm.RunRecordRepository
	runRecordStepsRepo orm.RunRecordStepRepository
	workflowRepo       WorkflowRepository
	templateRepo       orm.TemplateRepository
	kvInfoRepo         orm.KvTableRepository

	sqlcRepo sqlc.Querier

	taskRetryConfig *TaskRetryConfig
}

func NewPanelService(
	logger *zap.Logger,
	executor sqlexec.Executor,
	txManager postgres.TxManager,
	tokenActor *token.Actor,
	taskRetryConfig *TaskRetryConfig,
) *PanelService {
	return &PanelService{
		UnimplementedAccountServiceServer:  &panel.UnimplementedAccountServiceServer{},
		UnimplementedRunServiceServer:      &panel.UnimplementedRunServiceServer{},
		UnimplementedWorkflowServiceServer: &panel.UnimplementedWorkflowServiceServer{},
		UnimplementedTemplateServiceServer: &panel.UnimplementedTemplateServiceServer{},
		UnimplementedKvServiceServer:       &panel.UnimplementedKvServiceServer{},

		logger: logger,

		executor:   executor,
		txManager:  txManager,
		tokenActor: tokenActor,

		usersRepo:          NewUsersRepository(executor),
		refreshTokensRepo:  NewRefreshTokensRepository(executor),
		runRecordRepo:      NewRunRecordRepository(executor),
		runRecordStepsRepo: NewRunRecordStepRepository(executor),
		workflowRepo:       repositories.NewWorkflowRepo(executor),
		templateRepo:       NewTemplateRepository(executor),
		kvInfoRepo:         NewKvTableRepository(executor),

		sqlcRepo: sqlc.New(executor),

		taskRetryConfig: taskRetryConfig,
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
