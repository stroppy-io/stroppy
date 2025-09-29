package unit_queue

import (
	"context"
	"errors"
	"sync/atomic"

	"golang.org/x/sync/errgroup"
)

type generatorFunc[Seed, Result any] func(ctx context.Context, seed Seed) (Result, error)

type seedAndOpts[Seed any] struct {
	seed                      Seed
	workersCount, repeatCount int
}

// QueuedGenerator is up to infinite queued value generator.
type QueuedGenerator[Seed, Result any] struct {
	seeds     []seedAndOpts[Seed]
	ch        chan Result
	generator generatorFunc[Seed, Result]

	workersLimit, bufferSize int

	cancel context.CancelCauseFunc
	err    atomic.Value
	eg     errgroup.Group
	done   chan struct{}
}

// NewQueue creates new queue based on generator.
// workersLimit - limitation for warkers.
//
//   - == 1 - single worker, sequentially generation
//   - >= 2 - multible worker, generate asyncronously
//   - <= 0 - "unlimited", limit calculated to run all generator workers simultaneously
//
// bufferSize - size of queue.
//
//   - == 0 - blocking queue, no buffer.
//   - >= 1 - async queue, with buffer size.
//   - <=-1 - set buffer size to workersLimit (even if it set to 0).
func NewQueue[Seed, Result any](
	generator generatorFunc[Seed, Result],
	workersLimit, bufferSize int,
) *QueuedGenerator[Seed, Result] {
	return &QueuedGenerator[Seed, Result]{
		generator:    generator,
		ch:           nil,
		done:         make(chan struct{}),
		workersLimit: workersLimit,
		bufferSize:   bufferSize,
	}
}

// PrepareGenerator - second step, add generator seeds.
//
//   - seed - value passed to generator.
//   - workersCount - how many workers will started with that seed.
//   - repeatCount - how many times worker will run this generator with that seed.
func (uq *QueuedGenerator[Seed, Result]) PrepareGenerator(
	seed Seed,
	workersCount, repeatCount uint,
) {
	opts := seedAndOpts[Seed]{
		workersCount: 1,
		repeatCount:  1,
		seed:         seed,
	}
	if workersCount >= 1 {
		opts.workersCount = int(workersCount) //nolint:gosec // insane values may overflow
	}

	if repeatCount >= 1 {
		opts.repeatCount = int(repeatCount) //nolint:gosec // insane values may overflow
	}

	if uq.workersLimit <= 0 {
		uq.workersLimit -= opts.workersCount
	}

	uq.seeds = append(uq.seeds, opts)
}

// StartGeneration - third step, starts the generation of values.
func (uq *QueuedGenerator[Seed, Result]) StartGeneration(ctx context.Context) {
	ctx, uq.cancel = context.WithCancelCause(ctx)
	go uq.finalizer(ctx)

	if uq.workersLimit < 0 {
		uq.workersLimit = -uq.workersLimit
	}

	uq.eg.SetLimit(uq.workersLimit)

	if uq.bufferSize < 0 {
		uq.ch = make(chan Result, uq.workersLimit)
	} else {
		uq.ch = make(chan Result, uq.bufferSize)
	}

	go uq.infinitRunner(ctx)
}

// gracefully drains the queue at the end and closes the channel.
func (uq *QueuedGenerator[Seed, Result]) finalizer(ctx context.Context) {
	<-ctx.Done()

	go func() {
		for range uq.ch { //nolint: revive // this block empty, but it drains channel
		}
	}()
	<-uq.done
	close(uq.ch)
}

// infinitRunner - runs the generation until the stop.
func (uq *QueuedGenerator[Seed, Result]) infinitRunner(ctx context.Context) {
	for {
		for _, seed := range uq.seeds {
			for range seed.workersCount {
				uq.eg.Go(func() error {
					return uq.writer(ctx, seed)
				})
			}
		}

		if ctx.Err() != nil {
			break
		}

		err := uq.eg.Wait()
		if err != nil {
			uq.err.CompareAndSwap(nil, err)
		}

		if errors.Is(err, context.Canceled) ||
			errors.Is(err, context.DeadlineExceeded) ||
			errors.Is(err, ErrQueueIsStopped) {
			break
		}
	}

	close(uq.done)
}

func (uq *QueuedGenerator[Seed, Result]) writer(ctx context.Context, seed seedAndOpts[Seed]) error {
	for range seed.repeatCount {
		tx, err := uq.generator(ctx, seed.seed)
		if err != nil {
			return err
		}

		if ctx.Err() != nil {
			return ctx.Err()
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case uq.ch <- tx: // blocking here is fine
		}
	}

	return nil
}

var (
	ErrQueueIsDead    = errors.New("seed queue channel closed")
	ErrQueueIsStopped = errors.New("queue is stopped with .Stop()")
)

// GetNextElement - forth step, take as many elements as you want concurrently.
func (uq *QueuedGenerator[Seed, Result]) GetNextElement() (Result, error) { //nolint:ireturn // allow
	if err := uq.getError(); err != nil {
		return *new(Result), err
	}

	tx, ok := <-uq.ch
	if !ok {
		return *new(Result), ErrQueueIsDead
	}

	return tx, nil
}

func (uq *QueuedGenerator[Seed, Result]) getError() error {
	if err := uq.err.Load(); err != nil {
		return err.(error) //nolint: errcheck,forcetypeassert // is known type
	}

	return nil
}

// Stop - stops the generation.
func (uq *QueuedGenerator[Seed, Result]) Stop() error {
	uq.cancel(ErrQueueIsStopped)

	return uq.getError()
}
