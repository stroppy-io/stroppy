package unit_queue

import (
	"context"
	"errors"
	"sync/atomic"

	"golang.org/x/sync/errgroup"

	"github.com/stroppy-io/stroppy/pkg/core/proto"
)

type Driver interface {
	GenerateNextUnit(
		ctx context.Context,
		unitDesc *proto.UnitDescriptor,
	) (*proto.DriverTransaction, error)
}

// UnitQueue is an infinite *proto.DriverTransaction generator.
// It requires *proto.StepDescriptor and driver.
// Descripter defines generated sequence.
// Driver is the actual source of new data.
// UnitQueue wraps and bufferize the driver to reduce latencies in cuncurrent scenarios.
//
// TODO: make generic queue, polish, mb publish...
type UnitQueue struct {
	step   *proto.StepDescriptor
	ch     chan *proto.DriverTransaction
	driver Driver

	cancel context.CancelFunc
	err    atomic.Value
	eg     errgroup.Group
	done   chan struct{}
}

func NewUnitQueue(
	driver Driver,
	step *proto.StepDescriptor,
) *UnitQueue {
	const unitAsyncFactor = 10

	return newUnitQueue(driver, step, len(step.GetUnits())*unitAsyncFactor)
}

func newUnitQueue(
	drv Driver,
	step *proto.StepDescriptor,
	bufferSize int,
) *UnitQueue {
	return &UnitQueue{
		driver: drv,
		step:   step,
		ch:     make(chan *proto.DriverTransaction, bufferSize),
		done:   make(chan struct{}),
	}
}

func (uq *UnitQueue) StartGeneration(ctx context.Context) {
	ctx, cancel := context.WithCancel(ctx)
	uq.cancel = cancel

	go uq.finalizer(ctx)

	async := uq.step.GetAsync()

	poolSize := len(uq.step.GetUnits())
	if !async {
		poolSize = 1
	}

	uq.eg.SetLimit(poolSize)

	go uq.infinitStepRunner(ctx)
}

func (uq *UnitQueue) infinitStepRunner(ctx context.Context) {
	for {
		for _, unit := range uq.step.GetUnits() {
			uq.eg.Go(func() error {
				return uq.writer(ctx, unit)
			})
		}

		if ctx.Err() != nil {
			break
		}

		err := uq.eg.Wait()
		if err != nil {
			uq.err.CompareAndSwap(nil, err)
		}

		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			break
		}
	}

	close(uq.done)
}

func (uq *UnitQueue) finalizer(ctx context.Context) {
	<-ctx.Done()

	go func() {
		for range uq.ch { //nolint: revive // this block empty, but it drains channel
		}
	}()
	<-uq.done
	close(uq.ch)
}

func (uq *UnitQueue) writer(ctx context.Context, unit *proto.StepUnitDescriptor) error {
	for range unit.GetCount() {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		tx, err := uq.driver.GenerateNextUnit(ctx, unit.GetDescriptor_())
		if err != nil {
			return err
		}
		uq.ch <- tx // blocking here is fine
	}

	return nil
}

var ErrQueueIsDead = errors.New("unit queue channel closed")

func (uq *UnitQueue) GetNextUnit() (*proto.DriverTransaction, error) {
	if err := uq.getError(); err != nil {
		return nil, err
	}

	tx, ok := <-uq.ch
	if !ok {
		return nil, ErrQueueIsDead
	}

	return tx, nil
}

func (uq *UnitQueue) Stop() error {
	uq.cancel()

	return uq.getError()
}

func (uq *UnitQueue) getError() error {
	if err := uq.err.Load(); err != nil {
		return err.(error) //nolint: errcheck,forcetypeassert // is known type
	}

	return nil
}
