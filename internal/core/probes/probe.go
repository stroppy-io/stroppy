package probes

import (
	"context"
	"slices"
	"sync"
	"sync/atomic"

	"github.com/sourcegraph/conc/pool"
)

type ProbeStatus uint8

const (
	SuccessProbe ProbeStatus = 0
	FailureProbe ProbeStatus = 1
	TimeoutProbe ProbeStatus = 2
)

type Probe interface {
	// HealthCheck performs a health check on the probe.
	//
	// ctx is the context for the health check.
	// Returns the status of the health check as a ProbeStatus.
	HealthCheck(ctx context.Context) ProbeStatus
}

type ProbeFunc func(ctx context.Context) ProbeStatus

func (f ProbeFunc) HealthCheck(ctx context.Context) ProbeStatus {
	return f(ctx)
}

func ErrCheckProbe(f func(ctx context.Context) error) Probe {
	return ProbeFunc(func(ctx context.Context) ProbeStatus {
		err := f(ctx)
		if err != nil {
			return FailureProbe
		}
		return SuccessProbe
	})
}

type BoolProbe struct {
	healthy atomic.Bool
}

// NewBoolProbe creates a new instance of the BoolProbe struct.
//
// It returns a pointer to the newly created BoolProbe.
func NewBoolProbe() *BoolProbe {
	return &BoolProbe{healthy: atomic.Bool{}}
}

// Set sets the health state of the BoolProbe.
//
// state is the new health state of the probe.
// No return value.
func (p *BoolProbe) Set(state bool) {
	p.healthy.Store(state)
}

// HealthCheck performs a health check on the BoolProbe.
//
// If the probe is in a healthy state, it returns SuccessProbe.
// Otherwise, it returns FailureProbe.
//
// The provided context is ignored.
func (p *BoolProbe) HealthCheck(_ context.Context) ProbeStatus {
	if p.healthy.Load() {
		return SuccessProbe
	}

	return FailureProbe
}

type Asyncer interface {
	// Go calls the provided function asynchronously.
	// The function is called immediately if possible, or scheduled to be called
	// when the asyncer is ready. The provided function is called with the
	// context of the asyncer, which may be different from the context of the
	// caller. The function is called exactly once, unless the asyncer is
	// closed, in which case the function is not called at all.
	Go(fn func())
	// Wait waits until all asynchronous functions scheduled via Go have returned.
	Wait()
}

type NopeAsyncer struct{}

func NewNopeAsyncer() *NopeAsyncer { return &NopeAsyncer{} }

// Go is a no-op implementation of the asyncer interface's Go method.
func (a *NopeAsyncer) Go(f func()) { f() }

// Wait is a no-op implementation of the asyncer interface's Wait method.
func (a *NopeAsyncer) Wait() {}

type AsyncerFactory func() Asyncer

func NewAsyncerFactory(asyncer Asyncer) AsyncerFactory {
	return func() Asyncer { return asyncer }
}

func NewNopeAsyncerFactory() AsyncerFactory { return func() Asyncer { return NewNopeAsyncer() } }

type AsyncListProbe struct {
	AsyncerFactory
	probes []Probe
	sync.Mutex
}

// NewAsyncListProbe creates a new asynchronous list probe.
//
// asyncer is the asyncer that will be used to execute the probes asynchronously.
// probes are the probes that will be executed by the async list probe.
// Returns a pointer to the newly created AsyncListProbe.
func NewAsyncListProbe(asyncer AsyncerFactory, probes ...Probe) *AsyncListProbe {
	return &AsyncListProbe{probes: probes, AsyncerFactory: asyncer, Mutex: sync.Mutex{}}
}

// NewAsyncListProbeWithNope creates a new asynchronous list probe with a nope asyncer.
//
// probes are the probes that will be executed by the async list probe.
// Returns a pointer to the newly created AsyncListProbe.
func NewAsyncListProbeWithNope(probes ...Probe) *AsyncListProbe {
	return NewAsyncListProbe(NewNopeAsyncerFactory(), probes...)
}

// NewAsyncListProbeWithPool creates a new asynchronous list probe with a pool asyncer.
//
// probes are the probes that will be executed by the async list probe.
// Returns a pointer to the newly created AsyncListProbe.
func NewAsyncListProbeWithPool(probes ...Probe) *AsyncListProbe {
	return NewAsyncListProbe(func() Asyncer { return pool.New() }, probes...)
}

// HealthCheck performs a health check on the AsyncListProbe.
//
// It executes the HealthCheck method of each probe in the AsyncListProbe
// asynchronously using the provided asyncer. The results of the health
// checks are stored in the 'statuses' slice. The asyncer's 'Wait' method
// is called to wait for all asynchronous calls to complete. If all probes
// return a 'SuccessProbe' status, the function returns 'SuccessProbe'.
// Otherwise, it returns 'FailureProbe'.
//
// ctx: The context for the health check.
// Returns: The status of the health check as a ProbeStatus.
func (p *AsyncListProbe) HealthCheck(ctx context.Context) ProbeStatus {
	p.Lock()
	defer p.Unlock()
	statuses := make([]ProbeStatus, len(p.probes))
	async := p.AsyncerFactory()

	for i, probe := range p.probes {
		idx := i

		async.Go(func() {
			statuses[idx] = probe.HealthCheck(ctx)
		})
	}

	async.Wait()

	if slices.Max(statuses) == SuccessProbe {
		return SuccessProbe
	}

	return FailureProbe
}
