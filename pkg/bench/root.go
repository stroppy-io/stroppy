package bench

import (
	"context"
	"errors"
	"io"
	"net"
	"os"
	"sync"
	"time"

	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"go.uber.org/zap"

	"github.com/stroppy-io/stroppy/v6/pkg/driver"
)

const metricsShutdownTimeout = 10 * time.Second

// RootState holds engine state for one Go-workload run.
type RootState struct {
	lg  *zap.Logger
	ctx context.Context //nolint:containedctx // engine lifecycle ctx stored for async teardown/cancellation

	dialer *net.Dialer

	registry      *Registry
	meterProvider *sdkmetric.MeterProvider
	manualReader  *sdkmetric.ManualReader
	metricsPrefix string
	onSummary     func(metricdata.ResourceMetrics)
	quietSummary  bool
	summaryWriter io.Writer

	throughput    throughput
	txMetrics     *txMetrics
	errorReporter *errorReporter

	sharedMu    sync.Mutex
	sharedSlots map[uint64]*sharedDriverSlot

	stepFilter *stepFilterState

	reportMu   sync.Mutex
	reportData map[string]string
}

type sharedDriverSlot struct {
	drv driver.Driver
}

func newRootState(
	lg *zap.Logger,
	ctx context.Context,
	steps, noSteps []string,
	metricsConfig *MetricsConfig,
) (*RootState, error) {
	provider, reader, prefix, err := newMeterProvider(ctx, metricsConfig)
	if err != nil {
		return nil, err
	}

	var onSummary func(metricdata.ResourceMetrics)

	var quiet bool

	summaryWriter := io.Writer(os.Stderr)
	if metricsConfig != nil {
		onSummary = metricsConfig.OnSummary
		quiet = metricsConfig.Quiet
		if metricsConfig.SummaryWriter != nil {
			summaryWriter = metricsConfig.SummaryWriter
		}
	}

	state := &RootState{
		lg:            lg,
		ctx:           ctx,
		dialer:        &net.Dialer{},
		registry:      NewRegistry(provider.Meter("github.com/stroppy-io/stroppy/v6/pkg/bench"), prefix),
		meterProvider: provider,
		manualReader:  reader,
		metricsPrefix: prefix,
		onSummary:     onSummary,
		quietSummary:  quiet,
		summaryWriter: summaryWriter,
		txMetrics:     &txMetrics{},
		sharedSlots:   make(map[uint64]*sharedDriverSlot),
		stepFilter:    newStepFilter(steps, noSteps),
	}
	state.errorReporter = newErrorReporter(lg, errorReportInterval)

	return state, nil
}

// NotifyStep is a no-op at the floor (cloud notification deferred).
func (r *RootState) NotifyStep(name string, status int32) {}

// Teardown closes all shared drivers. (Workload.Teardown is invoked separately by Run.)
func (r *RootState) Teardown() error {
	var err error

	r.sharedMu.Lock()
	for _, slot := range r.sharedSlots {
		if slot.drv != nil {
			err = errors.Join(err, slot.drv.Teardown(r.ctx))
		}
	}
	r.sharedMu.Unlock()

	return err
}

func (r *RootState) shutdownMetrics() {
	ctx, cancel := context.WithTimeout(context.Background(), metricsShutdownTimeout)
	defer cancel()

	if err := r.meterProvider.Shutdown(ctx); err != nil {
		r.lg.Error("shutting down metrics", zap.Error(err))
	}
}
