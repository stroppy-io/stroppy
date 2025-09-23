package unit_queue

import (
	"context"
	"errors"
	"sync/atomic"

	"github.com/stroppy-io/stroppy/pkg/core/proto"
	"golang.org/x/sync/errgroup"
)

type Driver interface {
	GenerateNextUnit(context.Context, *proto.UnitDescriptor) (*proto.DriverTransaction, error)
}

// UnitQueue is an infinite *proto.DriverTransaction generator.
// It requires *proto.StepDescriptor and driver.
// Descripter defines generated sequence.
// Driver is the actual source of new data.
// UnitQueue wraps and bufferize the driver to reduce latencies in cuncurrent scenarious.
//
// TODO: make generic queue, polish, mb publish...
type UnitQueue struct {
	step   *proto.StepDescriptor
	ch     chan *proto.DriverTransaction
	driver Driver

	ctx    context.Context
	err    atomic.Value
	cancel context.CancelFunc
	eg     errgroup.Group
	done   chan struct{}
}

func NewUnitQueue(
	ctx context.Context,
	driver Driver,
	step *proto.StepDescriptor,
) *UnitQueue {
	return newUnitQueue(ctx, driver, step, len(step.GetUnits())*3)
}

func newUnitQueue(
	ctx context.Context,
	drv Driver,
	step *proto.StepDescriptor,
	bufferSize int,
) *UnitQueue {
	uq := &UnitQueue{}

	uq.driver = drv
	uq.step = step

	uq.ch = make(chan *proto.DriverTransaction, bufferSize)
	uq.ctx, uq.cancel = context.WithCancel(ctx)
	uq.done = make(chan struct{})

	return uq
}

func (uq *UnitQueue) StartGeneration() {

	go func() {
		<-uq.ctx.Done()
		go func() {
			for range uq.ch {
			}
		}()
		<-uq.done
		close(uq.ch)
	}()

	ctx := uq.ctx

	async := uq.step.GetAsync()

	poolSize := len(uq.step.GetUnits())
	if !async {
		poolSize = 1
	}

	uq.eg.SetLimit(poolSize)

	go func() {
		for {
			for _, unit := range uq.step.GetUnits() {
				uq.eg.Go(func() error {
					return uq.writer(ctx, unit)
				})
			}

			if uq.ctx.Err() != nil {
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
	}()
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
		return err.(error)
	}
	return nil
}
