package xk6air

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/stroppy-io/stroppy/pkg/common/proto/stroppy"
	"go.k6.io/k6/js/modules"
	k6metrics "go.k6.io/k6/metrics"
	"go.uber.org/zap"
)

const txTPSInterval = time.Second

type txMetrics struct {
	mu sync.Mutex

	txCount *k6metrics.Metric
	txTPS   *k6metrics.Metric
	tags    *k6metrics.TagSet

	total uint64

	started bool
	stopped bool
	stopCh  chan struct{}
	doneCh  chan struct{}
}

func (m *txMetrics) ensureRegistered(vu modules.VU, lg *zap.Logger) {
	initEnv := vu.InitEnv()
	if initEnv == nil {
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if m.txCount != nil && m.txTPS != nil {
		return
	}

	registry := initEnv.Registry
	txCount, err := registry.NewMetric("tx_count", k6metrics.Counter)
	if err != nil {
		lg.Fatal("can't register tx_count metric", zap.Error(err))
	}
	txTPS, err := registry.NewMetric("tx_tps", k6metrics.Trend)
	if err != nil {
		lg.Fatal("can't register tx_tps metric", zap.Error(err))
	}

	m.txCount = txCount
	m.txTPS = txTPS
	m.tags = registry.RootTagSet()
}

func (m *txMetrics) record(vu modules.VU, action, name string, isolation stroppy.TxIsolationLevel) {
	m.ensureRegistered(vu, rootModule.lg)

	state := vu.State()
	if state == nil {
		return
	}

	m.start(state.Samples, rootModule.ctx)
	atomic.AddUint64(&m.total, 1)

	txCount, tags, ok := m.snapshotCountMetric()
	if !ok {
		return
	}
	now := time.Now()
	tags = tags.With("tx_action", action)
	if name != "" {
		tags = tags.With("tx_name", name)
	}
	if iso := txIsolationName(isolation); iso != "" {
		tags = tags.With("tx_isolation", iso)
	}

	k6metrics.PushIfNotDone(vu.Context(), state.Samples, k6metrics.Sample{
		TimeSeries: k6metrics.TimeSeries{
			Metric: txCount,
			Tags:   tags,
		},
		Time:  now,
		Value: 1,
	})
}

func (m *txMetrics) start(samples chan<- k6metrics.SampleContainer, ctx context.Context) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.started || m.stopped || m.txTPS == nil || m.tags == nil {
		return
	}

	m.started = true
	m.stopCh = make(chan struct{})
	m.doneCh = make(chan struct{})
	go m.runSampler(ctx, samples, m.txTPS, m.tags, m.stopCh, m.doneCh)
}

func (m *txMetrics) stop() {
	m.mu.Lock()
	if !m.started || m.stopped {
		m.mu.Unlock()
		return
	}
	stopCh := m.stopCh
	doneCh := m.doneCh
	m.stopped = true
	close(stopCh)
	m.mu.Unlock()

	select {
	case <-doneCh:
	case <-time.After(2 * time.Second):
	}
}

func (m *txMetrics) snapshotCountMetric() (*k6metrics.Metric, *k6metrics.TagSet, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.txCount == nil || m.tags == nil {
		return nil, nil, false
	}
	return m.txCount, m.tags, true
}

func (m *txMetrics) runSampler(
	ctx context.Context,
	samples chan<- k6metrics.SampleContainer,
	metric *k6metrics.Metric,
	tags *k6metrics.TagSet,
	stopCh <-chan struct{},
	doneCh chan<- struct{},
) {
	defer close(doneCh)

	ticker := time.NewTicker(txTPSInterval)
	defer ticker.Stop()

	prevTotal := atomic.LoadUint64(&m.total)
	prevTime := time.Now()

	for {
		select {
		case now := <-ticker.C:
			prevTotal, prevTime = m.emitTPS(ctx, samples, metric, tags, prevTotal, prevTime, now, true)
		case <-stopCh:
			return
		case <-ctx.Done():
			return
		}
	}
}

func (m *txMetrics) emitTPS(
	ctx context.Context,
	samples chan<- k6metrics.SampleContainer,
	metric *k6metrics.Metric,
	tags *k6metrics.TagSet,
	prevTotal uint64,
	prevTime time.Time,
	now time.Time,
	emitZero bool,
) (uint64, time.Time) {
	elapsed := now.Sub(prevTime)
	if elapsed <= 0 {
		return prevTotal, prevTime
	}

	total := atomic.LoadUint64(&m.total)
	delta := total - prevTotal
	if delta == 0 && !emitZero {
		return total, now
	}

	k6metrics.PushIfNotDone(ctx, samples, k6metrics.Sample{
		TimeSeries: k6metrics.TimeSeries{
			Metric: metric,
			Tags:   tags,
		},
		Time:  now,
		Value: float64(delta) / elapsed.Seconds(),
	})

	return total, now
}

func txIsolationName(isolation stroppy.TxIsolationLevel) string {
	switch isolation {
	case stroppy.TxIsolationLevel_UNSPECIFIED:
		return "db_default"
	case stroppy.TxIsolationLevel_READ_UNCOMMITTED:
		return "read_uncommitted"
	case stroppy.TxIsolationLevel_READ_COMMITTED:
		return "read_committed"
	case stroppy.TxIsolationLevel_REPEATABLE_READ:
		return "repeatable_read"
	case stroppy.TxIsolationLevel_SERIALIZABLE:
		return "serializable"
	case stroppy.TxIsolationLevel_CONNECTION_ONLY:
		return "conn"
	case stroppy.TxIsolationLevel_NONE:
		return "none"
	default:
		return ""
	}
}
