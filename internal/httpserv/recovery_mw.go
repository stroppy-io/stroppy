package httpserv

import (
	"context"
	"fmt"
	"runtime"

	"connectrpc.com/connect"
)

type RecoveryHandlerFuncContext func(ctx context.Context, p any) (err error)

func NoRecoveryHandlerFuncContext(_ context.Context, _ any) (err error) {
	return nil
}

func recoverFrom(ctx context.Context, p any, r RecoveryHandlerFuncContext) error {
	if r != nil {
		return r(ctx, p)
	}
	stack := make([]byte, 64<<10)
	stack = stack[:runtime.Stack(stack, false)]
	return &PanicError{Panic: p, Stack: stack}
}

type PanicError struct {
	Panic any
	Stack []byte
}

func (e *PanicError) Error() string {
	return fmt.Sprintf("panic caught: %v\n\n%s", e.Panic, e.Stack)
}

func recoveryMiddleware(recoveryHandlerFunc RecoveryHandlerFuncContext) connect.UnaryInterceptorFunc {
	return func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (res connect.AnyResponse, err error) {
			defer func() {
				if r := recover(); r != nil {
					err = recoverFrom(ctx, r, recoveryHandlerFunc)
				}
			}()
			return next(ctx, req)
		}
	}
}
