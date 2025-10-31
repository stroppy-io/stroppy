package httpserv

import (
	"connectrpc.com/connect"
	"context"
	"fmt"
	"google.golang.org/grpc"
	"strings"

	"github.com/stroppy-io/stroppy-cloud-panel/internal/core/token"
)

type TokenProtector interface {
	ValidateToken(tokenStr string) (*token.Claims, error)
}

type protectOptions struct {
	excluding []string
}

type ProtectOption func(*protectOptions)

func WithExcluding(excluding ...string) ProtectOption {
	return func(o *protectOptions) {
		o.excluding = append(o.excluding, excluding...)
	}
}

func protectedGrpcService(
	tokenProtector TokenProtector,
	services []grpc.ServiceDesc,
	opts ...ProtectOption,
) map[string]TokenProtector {
	options := &protectOptions{}
	for _, opt := range opts {
		opt(options)
	}
	protected := make(map[string]TokenProtector)
	for _, service := range services {
		for _, method := range service.Methods {
			fullPath := fmt.Sprintf(
				"/%s/%s",
				service.ServiceName,
				method.MethodName,
			)
			contains := false
			for _, exclude := range options.excluding {
				if fullPath == exclude {
					contains = true
					break
				}
			}
			if !contains {
				protected[fullPath] = tokenProtector
			}
		}
	}
	return protected
}

func authMiddleware[T any](protectedPaths map[string]TokenProtector) connect.UnaryInterceptorFunc {
	return func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (res connect.AnyResponse, err error) {
			validator, ok := protectedPaths[req.Spec().Procedure]
			if !ok {
				return next(ctx, req)
			}
			tokenStr := req.Header().Get("Authorization")
			if tokenStr == "" {
				return nil, connect.NewError(connect.CodeUnauthenticated, fmt.Errorf("missing token"))
			}
			tokenStr = strings.TrimPrefix(tokenStr, "Bearer ")

			userClaims, err := validator.ValidateToken(tokenStr)
			if err != nil {
				return nil, connect.NewError(connect.CodeUnauthenticated, fmt.Errorf("invalid auth token: %w", err))
			}
			accountModel, err := token.AccountFromClaims[T](userClaims)
			if err != nil {
				return nil, connect.NewError(connect.CodeUnauthenticated, fmt.Errorf("invalid auth auth: %w", err))
			}
			newCtx := token.CtxWithAccount[T](
				token.CtxWithAccountToken(ctx, tokenStr),
				accountModel,
			)
			return next(newCtx, req)
		}
	}
}
