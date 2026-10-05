package bench

import (
	"context"
	"io"
	"net"
	"os"
	"sync"
	"time"

	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.uber.org/zap"

	"github.com/stroppy-io/stroppy/v6/pkg/report"
)

const metricsShutdownTimeout = 10 * time.Second

// rootState holds engine state for one Go-workload run.
type rootState struct {
	lg  *zap.Logger
	ctx context.Context //nolint:containedctx // engine lifecycle ctx stored for async teardown/cancellation

	dialer *net.Dialer

	registry      *metricRegistry
	meterProvider *sdkmetric.MeterProvider
	manualReader  *sdkmetric.ManualReader
	metricsPrefix string
	onSummary     func(map[string]report.Metric)
	quietSummary  bool
	summaryWriter io.Writer

	throughput    throughput
	txMetrics     *txMetrics
	errorReporter *errorReporter

	stepFilter *stepFilterState

	reportMu   sync.Mutex
	reportData map[string]string
}

func newrootState(
	lg *zap.Logger,
	ctx context.Context,
	steps, noSteps []string,
	metricsConfig *MetricsConfig,
) (*rootState, error) {
	provider, reader, prefix, err := newMeterProvider(ctx, metricsConfig)
	if err != nil {
		return nil, err
	}

	var onSummary func(map[string]report.Metric)

	var quiet bool

	summaryWriter := io.Writer(os.Stderr)

	if metricsConfig != nil {
		onSummary = metricsConfig.OnSummary
		quiet = metricsConfig.Quiet

		if metricsConfig.SummaryWriter != nil {
			summaryWriter = metricsConfig.SummaryWriter
		}
	}

	state := &rootState{
		lg:            lg,
		ctx:           ctx,
		dialer:        &net.Dialer{},
		registry:      newmetricRegistry(provider.Meter("github.com/stroppy-io/stroppy/v6/pkg/bench"), prefix),
		meterProvider: provider,
		manualReader:  reader,
		metricsPrefix: prefix,
		onSummary:     onSummary,
		quietSummary:  quiet,
		summaryWriter: summaryWriter,
		txMetrics:     &txMetrics{},
		stepFilter:    newStepFilter(steps, noSteps),
	}
	state.errorReporter = newErrorReporter(lg, errorReportInterval)

	return state, nil
}

func (r *rootState) shutdownMetrics() {
	ctx, cancel := context.WithTimeout(context.Background(), metricsShutdownTimeout)
	defer cancel()

	if err := r.meterProvider.Shutdown(ctx); err != nil {
		r.lg.Error("shutting down metrics", zap.Error(err))
	}
}
