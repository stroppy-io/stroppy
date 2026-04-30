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

const throughputInterval = time.Second

type txMetrics struct {
	mu sync.Mutex

	txCount     *k6metrics.Metric
	txTPS       *k6metrics.Metric
	runQueryQPS *k6metrics.Metric
	insertRows  *k6metrics.Metric
	insertRPS   *k6metrics.Metric
	tags        *k6metrics.TagSet

	txTotal    uint64
	queryTotal uint64

	txSampler    throughputSampler
	querySampler throughputSampler
}

type throughputSampler struct {
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
	if m.txCount != nil && m.txTPS != nil && m.runQueryQPS != nil &&
		m.insertRows != nil && m.insertRPS != nil {
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
	runQueryQPS, err := registry.NewMetric("run_query_qps", k6metrics.Trend)
	if err != nil {
		lg.Fatal("can't register run_query_qps metric", zap.Error(err))
	}
	insertRows, err := registry.NewMetric("insert_rows_total", k6metrics.Counter)
	if err != nil {
		lg.Fatal("can't register insert_rows_total metric", zap.Error(err))
	}
	insertRPS, err := registry.NewMetric("insert_rows_per_second", k6metrics.Trend)
	if err != nil {
		lg.Fatal("can't register insert_rows_per_second metric", zap.Error(err))
	}

	m.txCount = txCount
	m.txTPS = txTPS
	m.runQueryQPS = runQueryQPS
	m.insertRows = insertRows
	m.insertRPS = insertRPS
	m.tags = registry.RootTagSet()
}

func (m *txMetrics) recordQuery(vu modules.VU) {
	m.ensureRegistered(vu, rootModule.lg)

	if vu.State() == nil {
		return
	}

	atomic.AddUint64(&m.queryTotal, 1)
}

func (m *txMetrics) recordInsert(vu modules.VU, table string, rows int64, elapsed time.Duration) {
	m.ensureRegistered(vu, rootModule.lg)

	state := vu.State()
	if state == nil {
		return
	}

	insertRows, insertRPS, tags, ok := m.snapshotInsertMetrics()
	if !ok {
		return
	}
	if state.Tags != nil {
		tags = currentVUTags(state.Tags.GetCurrentValues(), tags)
	} else {
		tags = withCurrentStepTag(tags)
	}

	if table == "" {
		table = "unknown"
	}
	if rows < 0 {
		rows = 0
	}

	now := time.Now()
	tags = tags.With("table_name", table)

	k6metrics.PushIfNotDone(vu.Context(), state.Samples, k6metrics.Sample{
		TimeSeries: k6metrics.TimeSeries{
			Metric: insertRows,
			Tags:   tags,
		},
		Time:  now,
		Value: float64(rows),
	})

	rowsPerSecond := float64(0)
	if elapsed > 0 {
		rowsPerSecond = float64(rows) / elapsed.Seconds()
	}

	k6metrics.PushIfNotDone(vu.Context(), state.Samples, k6metrics.Sample{
		TimeSeries: k6metrics.TimeSeries{
			Metric: insertRPS,
			Tags:   tags,
		},
		Time:  now,
		Value: rowsPerSecond,
	})
}

func (m *txMetrics) record(vu modules.VU, action, name string, isolation stroppy.TxIsolationLevel) {
	m.ensureRegistered(vu, rootModule.lg)

	state := vu.State()
	if state == nil {
		return
	}

	m.start(state.Samples, rootModule.ctx)
	atomic.AddUint64(&m.txTotal, 1)

	txCount, tags, ok := m.snapshotCountMetric()
	if !ok {
		return
	}
	if state.Tags != nil {
		tags = currentVUTags(state.Tags.GetCurrentValues(), tags)
	} else {
		tags = withCurrentStepTag(tags)
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

	m.startSamplerLocked(&m.txSampler, &m.txTotal, ctx, samples, m.txTPS, m.tags)
	m.startSamplerLocked(&m.querySampler, &m.queryTotal, ctx, samples, m.runQueryQPS, m.tags)
}

func (m *txMetrics) stop() {
	m.stopSampler(&m.txSampler)
	m.stopSampler(&m.querySampler)
}

func (m *txMetrics) startSamplerLocked(
	sampler *throughputSampler,
	total *uint64,
	ctx context.Context,
	samples chan<- k6metrics.SampleContainer,
	metric *k6metrics.Metric,
	tags *k6metrics.TagSet,
) {
	if sampler.started || sampler.stopped || metric == nil || tags == nil {
		return
	}

	sampler.started = true
	sampler.stopCh = make(chan struct{})
	sampler.doneCh = make(chan struct{})
	go runThroughputSampler(ctx, samples, metric, tags, total, sampler.stopCh, sampler.doneCh)
}

func (m *txMetrics) stopSampler(sampler *throughputSampler) {
	m.mu.Lock()
	if !sampler.started || sampler.stopped {
		m.mu.Unlock()
		return
	}
	stopCh := sampler.stopCh
	doneCh := sampler.doneCh
	sampler.stopped = true
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

func (m *txMetrics) snapshotInsertMetrics() (
	*k6metrics.Metric,
	*k6metrics.Metric,
	*k6metrics.TagSet,
	bool,
) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.insertRows == nil || m.insertRPS == nil || m.tags == nil {
		return nil, nil, nil, false
	}

	return m.insertRows, m.insertRPS, m.tags, true
}

func currentVUTags(tagsAndMeta k6metrics.TagsAndMeta, fallback *k6metrics.TagSet) *k6metrics.TagSet {
	tags := fallback
	if tagsAndMeta.Tags == nil {
		return withCurrentStepTag(tags)
	}

	tags = tagsAndMeta.Tags
	return withCurrentStepTag(tags)
}

func withCurrentStepTag(tags *k6metrics.TagSet) *k6metrics.TagSet {
	if tags == nil {
		return nil
	}
	if _, ok := tags.Get("step"); ok {
		return tags
	}
	if step := rootModule.CurrentStep(); step != "" {
		return tags.With("step", step)
	}
	return tags
}

func runThroughputSampler(
	ctx context.Context,
	samples chan<- k6metrics.SampleContainer,
	metric *k6metrics.Metric,
	tags *k6metrics.TagSet,
	total *uint64,
	stopCh <-chan struct{},
	doneCh chan<- struct{},
) {
	defer close(doneCh)

	ticker := time.NewTicker(throughputInterval)
	defer ticker.Stop()

	prevTotal := atomic.LoadUint64(total)
	prevTime := time.Now()

	for {
		select {
		case now := <-ticker.C:
			prevTotal, prevTime = emitThroughput(ctx, samples, metric, tags, total, prevTotal, prevTime, now, true)
		case <-stopCh:
			return
		case <-ctx.Done():
			return
		}
	}
}

func emitThroughput(
	ctx context.Context,
	samples chan<- k6metrics.SampleContainer,
	metric *k6metrics.Metric,
	tags *k6metrics.TagSet,
	totalCounter *uint64,
	prevTotal uint64,
	prevTime time.Time,
	now time.Time,
	emitZero bool,
) (uint64, time.Time) {
	elapsed := now.Sub(prevTime)
	if elapsed <= 0 {
		return prevTotal, prevTime
	}

	total := atomic.LoadUint64(totalCounter)
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
