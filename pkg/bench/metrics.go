package bench

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	k6metrics "go.k6.io/k6/metrics"
	"go.uber.org/zap"

	"github.com/stroppy-io/stroppy/pkg/common/proto/stroppy"
	"github.com/stroppy-io/stroppy/pkg/driver/insertprogress"
)

const throughputInterval = time.Second

type txMetrics struct {
	mu           sync.Mutex
	registered   atomic.Bool
	txCount      *k6metrics.Metric
	txTPS        *k6metrics.Metric
	runQueryQPS  *k6metrics.Metric
	insertRows   *k6metrics.Metric
	progressRows *k6metrics.Metric
	progressRPS  *k6metrics.Metric
	tags         *k6metrics.TagSet

	// Per-operation metrics mirroring the TS DriverX wrapper.
	runQueryDuration *k6metrics.Metric
	runQueryCount    *k6metrics.Metric
	runQueryErrRate  *k6metrics.Metric
	insertDuration   *k6metrics.Metric
	insertErrRate    *k6metrics.Metric
	iterationDur     *k6metrics.Metric
	iterations       *k6metrics.Metric
	txTotalDuration  *k6metrics.Metric
	txCommitRate     *k6metrics.Metric
	txErrorRate      *k6metrics.Metric
	txQueriesPerTx   *k6metrics.Metric

	txTotal    uint64
	queryTotal uint64

	txSampler    throughputSampler
	querySampler throughputSampler
}

type throughputSampler struct {
	started atomic.Bool
	stopped atomic.Bool
	stopCh  chan struct{}
	doneCh  chan struct{}
}

func (m *txMetrics) ensureRegistered(vu *VU, lg *zap.Logger) {
	if m.registered.Load() {
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if m.registered.Load() {
		return
	}

	registry := vu.root.registry

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

	progressRows, err := registry.NewMetric("insert_progress_rows_total", k6metrics.Counter)
	if err != nil {
		lg.Fatal("can't register insert_progress_rows_total metric", zap.Error(err))
	}

	progressRPS, err := registry.NewMetric("insert_progress_rows_per_second", k6metrics.Trend)
	if err != nil {
		lg.Fatal("can't register insert_progress_rows_per_second metric", zap.Error(err))
	}

	runQueryDuration, err := registry.NewMetric("run_query_duration", k6metrics.Trend)
	if err != nil {
		lg.Fatal("can't register run_query_duration metric", zap.Error(err))
	}

	runQueryCount, err := registry.NewMetric("run_query_count", k6metrics.Counter)
	if err != nil {
		lg.Fatal("can't register run_query_count metric", zap.Error(err))
	}

	runQueryErrRate, err := registry.NewMetric("run_query_error_rate", k6metrics.Rate)
	if err != nil {
		lg.Fatal("can't register run_query_error_rate metric", zap.Error(err))
	}

	insertDuration, err := registry.NewMetric("insert_duration", k6metrics.Trend)
	if err != nil {
		lg.Fatal("can't register insert_duration metric", zap.Error(err))
	}

	insertErrRate, err := registry.NewMetric("insert_error_rate", k6metrics.Rate)
	if err != nil {
		lg.Fatal("can't register insert_error_rate metric", zap.Error(err))
	}

	iterationDur, err := registry.NewMetric("iteration_duration", k6metrics.Trend)
	if err != nil {
		lg.Fatal("can't register iteration_duration metric", zap.Error(err))
	}

	iterations, err := registry.NewMetric("iterations", k6metrics.Counter)
	if err != nil {
		lg.Fatal("can't register iterations metric", zap.Error(err))
	}

	txTotalDuration, err := registry.NewMetric("tx_total_duration", k6metrics.Trend)
	if err != nil {
		lg.Fatal("can't register tx_total_duration metric", zap.Error(err))
	}

	txCommitRate, err := registry.NewMetric("tx_commit_rate", k6metrics.Rate)
	if err != nil {
		lg.Fatal("can't register tx_commit_rate metric", zap.Error(err))
	}

	txQueriesPerTx, err := registry.NewMetric("tx_queries_per_tx", k6metrics.Trend)
	if err != nil {
		lg.Fatal("can't register tx_queries_per_tx metric", zap.Error(err))
	}

	txErrorRate, err := registry.NewMetric("tx_error_rate", k6metrics.Rate)
	if err != nil {
		lg.Fatal("can't register tx_error_rate metric", zap.Error(err))
	}

	m.txCount = txCount
	m.txTPS = txTPS
	m.runQueryQPS = runQueryQPS
	m.insertRows = insertRows
	m.progressRows = progressRows
	m.progressRPS = progressRPS
	m.runQueryDuration = runQueryDuration
	m.runQueryCount = runQueryCount
	m.runQueryErrRate = runQueryErrRate
	m.insertDuration = insertDuration
	m.insertErrRate = insertErrRate
	m.iterationDur = iterationDur
	m.iterations = iterations
	m.txTotalDuration = txTotalDuration
	m.txCommitRate = txCommitRate
	m.txErrorRate = txErrorRate
	m.txQueriesPerTx = txQueriesPerTx
	m.tags = registry.RootTagSet()
	m.registered.Store(true)
}

func applyStepTag(tags *k6metrics.TagSet, step string) *k6metrics.TagSet {
	if step != "" {
		return tags.With("step", step)
	}

	return tags
}

func (m *txMetrics) emit(vu *VU, metric *k6metrics.Metric, value float64, tags *k6metrics.TagSet) {
	if metric == nil {
		return
	}

	k6metrics.PushIfNotDone(vu.Context(), vu.root.samples, k6metrics.Sample{
		TimeSeries: k6metrics.TimeSeries{Metric: metric, Tags: tags},
		Time:       time.Now(), Value: value,
	})
}

// recordQueryResult emits the per-query metrics (duration/count/error_rate),
// mirroring the TS DriverX wrapper. elapsed is the driver-reported duration;
// on error only the error rate is recorded (no count/duration).
func (m *txMetrics) recordQueryResult(vu *VU, elapsed time.Duration, queryErr error) {
	m.ensureRegistered(vu, root.lg)
	m.start(vu.root.samples, vu.root.ctx)
	atomic.AddUint64(&m.queryTotal, 1)

	tags := applyStepTag(m.tags, vu.stepTag)
	if queryErr != nil {
		m.emit(vu, m.runQueryErrRate, 1, tags)
		return
	}

	m.emit(vu, m.runQueryDuration, elapsed.Seconds()*1000, tags)
	m.emit(vu, m.runQueryErrRate, 0, tags)
	m.emit(vu, m.runQueryCount, 1, tags)
}

// recordInsertResult emits insert_duration / insert_error_rate for one InsertSpec
// call. insert_rows_total is emitted separately by recordInsert.
func (m *txMetrics) recordInsertResult(vu *VU, table string, elapsed time.Duration, insertErr error) {
	m.ensureRegistered(vu, root.lg)

	if table == "" {
		table = "unknown"
	}

	tags := applyStepTag(m.tags, vu.stepTag).With("table_name", table)
	if insertErr != nil {
		m.emit(vu, m.insertErrRate, 1, tags)
		return
	}

	m.emit(vu, m.insertDuration, elapsed.Seconds()*1000, tags)
	m.emit(vu, m.insertErrRate, 0, tags)
}

// recordIteration emits iteration_duration + iterations for one Iterate call.
func (m *txMetrics) recordIteration(vu *VU, elapsed time.Duration) {
	m.ensureRegistered(vu, root.lg)
	tags := applyStepTag(m.tags, vu.stepTag)
	m.emit(vu, m.iterationDur, elapsed.Seconds()*1000, tags)
	m.emit(vu, m.iterations, 1, tags)
}

// recordTxEnd emits the per-transaction summary metrics mirroring the TS TxX:
// total wall duration, commit success rate, and query count per transaction.
func (m *txMetrics) recordTxEnd(vu *VU, name string, elapsed time.Duration, queries int, committed bool) {
	m.ensureRegistered(vu, root.lg)
	tags := applyStepTag(m.tags, vu.stepTag)
	if name != "" {
		tags = tags.With("tx_name", name)
	}
	if committed {
		m.emit(vu, m.txCommitRate, 1, tags)
		m.emit(vu, m.txErrorRate, 0, tags)
	} else {
		m.emit(vu, m.txCommitRate, 0, tags)
		m.emit(vu, m.txErrorRate, 1, tags)
	}
	m.emit(vu, m.txTotalDuration, elapsed.Seconds()*1000, tags)
	m.emit(vu, m.txQueriesPerTx, float64(queries), tags)
}

func (m *txMetrics) recordInsertProgress(vu *VU, snapshot insertprogress.Snapshot) {
	m.ensureRegistered(vu, root.lg)

	progressRows, progressRPS, tags, ok := m.snapshotProgressMetrics()
	if !ok {
		return
	}

	tags = applyStepTag(tags, vu.stepTag)
	tags = tags.With("table_name", snapshot.Table).
		With("method", snapshot.Method).
		With("event", string(snapshot.Event)).
		With("row_kind", snapshot.RowKind)

	now := time.Now()
	if snapshot.DeltaRows > 0 {
		k6metrics.PushIfNotDone(vu.Context(), vu.root.samples, k6metrics.Sample{
			TimeSeries: k6metrics.TimeSeries{Metric: progressRows, Tags: tags},
			Time:       now, Value: float64(snapshot.DeltaRows),
		})
	}

	k6metrics.PushIfNotDone(vu.Context(), vu.root.samples, k6metrics.Sample{
		TimeSeries: k6metrics.TimeSeries{Metric: progressRPS, Tags: tags},
		Time:       now, Value: snapshot.CurrentRowsPerSecond,
	})
}

func (m *txMetrics) recordInsert(vu *VU, table string, rows int64) {
	m.ensureRegistered(vu, root.lg)

	insertRows, tags, ok := m.snapshotInsertMetrics()
	if !ok {
		return
	}

	tags = applyStepTag(tags, vu.stepTag)

	if table == "" {
		table = "unknown"
	}

	if rows < 0 {
		rows = 0
	}

	now := time.Now()
	tags = tags.With("table_name", table)
	k6metrics.PushIfNotDone(vu.Context(), vu.root.samples, k6metrics.Sample{
		TimeSeries: k6metrics.TimeSeries{Metric: insertRows, Tags: tags},
		Time:       now, Value: float64(rows),
	})
}

func (m *txMetrics) record(vu *VU, action, name string, isolation stroppy.TxIsolationLevel) {
	m.ensureRegistered(vu, root.lg)
	m.start(vu.root.samples, vu.root.ctx)
	atomic.AddUint64(&m.txTotal, 1)

	txCount, tags, ok := m.snapshotCountMetric()
	if !ok {
		return
	}

	tags = applyStepTag(tags, vu.stepTag)
	now := time.Now()

	tags = tags.With("tx_action", action)
	if name != "" {
		tags = tags.With("tx_name", name)
	}

	if iso := txIsolationName(isolation); iso != "" {
		tags = tags.With("tx_isolation", iso)
	}

	k6metrics.PushIfNotDone(vu.Context(), vu.root.samples, k6metrics.Sample{
		TimeSeries: k6metrics.TimeSeries{Metric: txCount, Tags: tags},
		Time:       now, Value: 1,
	})
}

func (m *txMetrics) start(samples chan<- k6metrics.SampleContainer, ctx context.Context) {
	m.startSampler(&m.txSampler, &m.txTotal, ctx, samples, m.txTPS, m.tags)
	m.startSampler(&m.querySampler, &m.queryTotal, ctx, samples, m.runQueryQPS, m.tags)
}

func (m *txMetrics) stop() {
	m.stopSampler(&m.txSampler)
	m.stopSampler(&m.querySampler)
}

func (m *txMetrics) startSampler(
	sampler *throughputSampler, total *uint64, ctx context.Context,
	samples chan<- k6metrics.SampleContainer, metric *k6metrics.Metric, tags *k6metrics.TagSet,
) {
	if metric == nil || tags == nil || sampler.stopped.Load() {
		return
	}

	if !sampler.started.CompareAndSwap(false, true) {
		return
	}

	sampler.stopCh = make(chan struct{})

	sampler.doneCh = make(chan struct{})
	go runThroughputSampler(ctx, samples, metric, tags, total, sampler.stopCh, sampler.doneCh)
}

func (m *txMetrics) stopSampler(sampler *throughputSampler) {
	if !sampler.stopped.CompareAndSwap(false, true) {
		return
	}

	if !sampler.started.Load() {
		return
	}

	close(sampler.stopCh)

	select {
	case <-sampler.doneCh:
	case <-time.After(2 * time.Second):
	}
}

func (m *txMetrics) snapshotCountMetric() (*k6metrics.Metric, *k6metrics.TagSet, bool) {
	if !m.registered.Load() {
		return nil, nil, false
	}

	return m.txCount, m.tags, true
}

func (m *txMetrics) snapshotInsertMetrics() (*k6metrics.Metric, *k6metrics.TagSet, bool) {
	if !m.registered.Load() {
		return nil, nil, false
	}

	return m.insertRows, m.tags, true
}

func (m *txMetrics) snapshotProgressMetrics() (*k6metrics.Metric, *k6metrics.Metric, *k6metrics.TagSet, bool) {
	if !m.registered.Load() {
		return nil, nil, nil, false
	}

	return m.progressRows, m.progressRPS, m.tags, true
}

func runThroughputSampler(
	ctx context.Context, samples chan<- k6metrics.SampleContainer, metric *k6metrics.Metric,
	tags *k6metrics.TagSet, total *uint64, stopCh <-chan struct{}, doneCh chan<- struct{},
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
	ctx context.Context, samples chan<- k6metrics.SampleContainer, metric *k6metrics.Metric,
	tags *k6metrics.TagSet, totalCounter *uint64, prevTotal uint64, prevTime time.Time,
	now time.Time, emitZero bool,
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
		TimeSeries: k6metrics.TimeSeries{Metric: metric, Tags: tags},
		Time:       now, Value: float64(delta) / elapsed.Seconds(),
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
