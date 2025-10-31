package api

import (
	"connectrpc.com/connect"
	"context"
	"errors"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/core/pwd"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/core/token"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/entity/claims"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/entity/ids"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/entity/timestamps"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/infrastructure/postgresql/sqlerr"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	emptypb "google.golang.org/protobuf/types/known/emptypb"

	"github.com/stroppy-io/stroppy-cloud-panel/internal/infrastructure/orm"
	postgres "github.com/stroppy-io/stroppy-cloud-panel/internal/infrastructure/postgresql"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/infrastructure/postgresql/sqlexec"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/proto/panel"
)

type PanelService struct {
	*panel.UnimplementedPanelServiceServer

	executor   sqlexec.Executor
	txManager  postgres.TxManager
	tokenActor *token.Actor

	usersRepo        orm.UserRepository
	stroppyStepsRepo orm.StroppyStepRepository
	resourcesRepo    orm.ResourceRepository
	runsRepo         orm.RunRepository
}

func NewPanelService(
	executor sqlexec.Executor,
	txManager postgres.TxManager,
	tokenActor *token.Actor,
) *PanelService {
	return &PanelService{
		UnimplementedPanelServiceServer: &panel.UnimplementedPanelServiceServer{},

		executor:   executor,
		txManager:  txManager,
		tokenActor: tokenActor,

		usersRepo:        NewUsersRepository(executor),
		stroppyStepsRepo: NewStroppyStepsRepository(executor),
		resourcesRepo:    NewResourcesRepository(executor),
		runsRepo:         NewRunsRepository(executor),
	}
}

func (p PanelService) Register(
	ctx context.Context,
	request *panel.RegisterRequest,
) (*emptypb.Empty, error) {
	return postgres.WithSerializableRet(ctx, p.txManager, func(ctx context.Context) (*emptypb.Empty, error) {
		pwdHash, err := pwd.ValidateAndHashPassword(request.Password)
		if err != nil {
			return nil, connect.NewError(connect.CodeInvalidArgument, err)
		}
		err = p.usersRepo.ScannerRepository().Insert(
			ctx,
			orm.UserToScanner(
				ids.UlidToStr,
				orm.WithUserPasswordHash(pwdHash),
			)(&panel.User{
				Id:     ids.NewUlid(),
				Email:  request.Email,
				Timing: timestamps.NewTiming(),
			}),
		)
		if err != nil {
			if sqlerr.IsConstraintError(err) {
				return nil, connect.NewError(connect.CodeAlreadyExists, err)
			}
			return nil, err
		}
		return empty()
	})
}

func (p PanelService) Login(
	ctx context.Context,
	request *panel.LoginRequest,
) (*panel.LoginResponse, error) {
	if request.GetEmail() == "" {
		return nil, status.Error(codes.InvalidArgument, "Login or email is empty")
	}
	return postgres.WithSerializableRet(ctx, p.txManager,
		func(ctx context.Context) (*panel.LoginResponse, error) {
			acc, err := p.usersRepo.ScannerRepository().GetBy(ctx, orm.User.SelectAll().Where(orm.User.Email.Eq(request.GetEmail())))
			if err != nil {
				if sqlerr.IsNotFound(err) {
					return nil, connect.NewError(connect.CodeNotFound, errors.New("user not found"))
				}
				return nil, err
			}
			if !pwd.CheckPasswordHash(request.GetPassword(), acc.PasswordHash) {
				return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("invalid credentials"))
			}
			tokenClaims, err := token.AccountToTokenClaims[claims.Claims](&claims.Claims{
				UserID: acc.Id,
			})
			if err != nil {
				return nil, connect.NewError(connect.CodeInternal, err)
			}
			refresh, err := token.NewRandomToken(38)
			if err != nil {
				return nil, connect.NewError(connect.CodeInternal, err)
			}
			access, err := p.tokenActor.NewAccessToken(acc.Id, tokenClaims)
			if err != nil {
				return nil, connect.NewError(connect.CodeInternal, err)
			}
			err = p.usersRepo.Exec(ctx, orm.User.Update().Set(orm.User.RefreshTokens.Set(append(acc.RefreshTokens, refresh))))
			if err != nil {
				return nil, connect.NewError(connect.CodeInternal, err)
			}
			//return access, nil
			return &panel.LoginResponse{
				AccessToken:  access,
				RefreshToken: refresh,
			}, nil
		})
}

func (p PanelService) RefreshTokens(
	ctx context.Context,
	request *panel.RefreshTokensRequest,
) (*panel.RefreshTokensResponse, error) {
	return postgres.WithSerializableRet(ctx, p.txManager,
		func(ctx context.Context) (*panel.RefreshTokensResponse, error) {
			acc, err := p.usersRepo.GetBy(ctx, orm.User.SelectAll().Where(orm.User.RefreshTokens.Any(request.RefreshToken)))
			if err != nil {
				if sqlerr.IsNotFound(err) {
					return nil, connect.NewError(connect.CodeNotFound, errors.New("refresh token not found"))
				}
				return nil, connect.NewError(connect.CodeInternal, err)
			}
			removedTokens := make([]string, 0)
			for _, tk := range acc.RefreshTokens {
				if tk != request.RefreshToken {
					removedTokens = append(removedTokens, tk)
				}
			}
			err := p.usersRepo.ScannerRepository().Update(ctx, acc, orm.User.RefreshTokens.Set(removedTokens))
			if err != nil {
				return nil, connect.NewError(connect.CodeInternal, err)
			}
			access, err := p.tokenActor.NewAccessToken(acc.Id, token.AccountToTokenClaims[claims.Claims](&claims.Claims{
				UserID: acc.Id,
			}))
			if err != nil {
				return nil, connect.NewError(connect.CodeInternal, err)
			}
			return &panel.RefreshTokensResponse{
				AccessToken: access,
			}, nil
		})
}

func (p PanelService) ListRuns(ctx context.Context, request *panel.ListRunsRequest) (*panel.Run_List, error) {
}

func (p PanelService) ListTopRuns(ctx context.Context, request *panel.ListRunsRequest) (*panel.Run_List, error) {
}

func (p PanelService) GetResource(ctx context.Context, ulid *panel.Ulid) (*panel.Resource_TreeNode, error) {
}
