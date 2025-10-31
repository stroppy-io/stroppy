package utils

import (
	"context"
	"errors"

	"github.com/sourcegraph/conc/pool"
)

type Asyncer interface {
	Go(fn func(ctx context.Context) error)
	Wait() error
}

type NoopAsyncer struct {
	ctx           context.Context //nolint: containedctx // no alternative for context save
	cancelOnError bool
	err           error
}

func (n *NoopAsyncer) Go(f func(ctx context.Context) error) {
	if n.cancelOnError && n.err != nil {
		return
	}

	err := f(n.ctx)
	if err != nil {
		n.err = errors.Join(n.err, err) //nolint: staticcheck // false positive
	}
}

func (n *NoopAsyncer) Wait() error {
	return n.err
}

func NewAsyncerFromExecType( //nolint: ireturn // need as lib part
	ctx context.Context,
	async bool,
	size int,
	cancelOnError bool,
) Asyncer {
	if !async {
		return &NoopAsyncer{ctx: ctx, cancelOnError: cancelOnError}
	}

	asyncPool := pool.New().
		WithContext(ctx).
		WithMaxGoroutines(size)

	if cancelOnError {
		asyncPool = asyncPool.WithCancelOnError().WithFirstError()
	}

	return asyncPool
}
