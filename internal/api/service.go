package api

import (
	"context"

	emptypb "google.golang.org/protobuf/types/known/emptypb"

	"github.com/stroppy-io/stroppy-cloud-panel/internal/infrastructure/orm"
	postgres "github.com/stroppy-io/stroppy-cloud-panel/internal/infrastructure/postgresql"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/infrastructure/postgresql/sqlexec"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/proto/panel"
)

type PanelService struct {
	*panel.UnimplementedPanelServiceServer

	executor  sqlexec.Executor
	txManager postgres.TxManager

	usersRepo        orm.UserRepository
	stroppyStepsRepo orm.StroppyStepRepository
	resourcesRepo    orm.ResourceRepository
	runsRepo         orm.RunRepository
}

func NewPanelService(
	executor sqlexec.Executor,
	txManager postgres.TxManager,
) *PanelService {
	return &PanelService{
		UnimplementedPanelServiceServer: &panel.UnimplementedPanelServiceServer{},

		executor:  executor,
		txManager: txManager,

		usersRepo:        NewUsersRepository(executor),
		stroppyStepsRepo: NewStroppyStepsRepository(executor),
		resourcesRepo:    NewResourcesRepository(executor),
		runsRepo:         NewRunsRepository(executor),
	}
}

func (p PanelService) Register(ctx context.Context, request *panel.RegisterRequest) (*emptypb.Empty, error) {
}

func (p PanelService) Login(ctx context.Context, request *panel.LoginRequest) (*panel.LoginResponse, error) {
}

func (p PanelService) RefreshTokens(ctx context.Context, request *panel.RefreshTokensRequest) (*panel.RefreshTokensResponse, error) {
}

func (p PanelService) ListRuns(ctx context.Context, request *panel.ListRunsRequest) (*panel.Run_List, error) {
}

func (p PanelService) ListTopRuns(ctx context.Context, request *panel.ListRunsRequest) (*panel.Run_List, error) {
}

func (p PanelService) GetResource(ctx context.Context, ulid *panel.Ulid) (*panel.Resource_TreeNode, error) {
}
