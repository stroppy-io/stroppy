package bench

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/zap"

	"github.com/stroppy-io/stroppy/pkg/common/proto/stroppy"
	"github.com/stroppy-io/stroppy/pkg/driver/insertprogress"
)

const throughputInterval = time.Second

// Named numeric constants for the magic-number linter (see metrics.go, retry.go,
// root.go, runtime.go, step.go). Kept package-local and purpose-named.
const (
	millisPerSecond       = 1000.0 // seconds→milliseconds conversion factor
	sampleChannelCapacity = 4096   // buffered sample channel depth
	samplerStopGrace      = 2 * time.Second
	percentScale          = 100.0 // ratio→percent
	medianP               = 0.5   // p50 percentile argument
	p90                   = 0.9
	p95                   = 0.95
	p99                   = 0.99
)

type txMetrics struct {
	mu           sync.Mutex
	registered   atomic.Bool
	txCount      *metric
	txTPS        *metric
	runQueryQPS  *metric
	insertRows   *metric
	progressRows *metric
	progressRPS  *metric
	tags         *TagSet

	// Per-operation metrics mirroring the TS DriverX wrapper.
	runQueryDuration *metric
	runQueryCount    *metric
	runQueryErrRate  *metric
	insertDuration   *metric
	insertErrRate    *metric
	iterationDur     *metric
	iterations       *metric
	txTotalDuration  *metric
	txCommitRate     *metric
	txErrorRate      *metric
	txQueriesPerTx   *metric

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

	r := vu.root.registry
	newMetric := func(name string, typ metricType) *metric {
		mm, err := r.NewMetric(name, typ)
		if err != nil {
			lg.Fatal("can't register "+name+" metric", zap.Error(err))
		}

		return mm
	}

	m.txCount = newMetric("tx_count", Counter)
	m.txTPS = newMetric("tx_tps", Trend)
	m.runQueryQPS = newMetric("run_query_qps", Trend)
	m.insertRows = newMetric("insert_rows_total", Counter)
	m.progressRows = newMetric("insert_progress_rows_total", Counter)
	m.progressRPS = newMetric("insert_progress_rows_per_second", Trend)
	m.runQueryDuration = newMetric("run_query_duration", Trend)
	m.runQueryCount = newMetric("run_query_count", Counter)
	m.runQueryErrRate = newMetric("run_query_error_rate", Rate)
	m.insertDuration = newMetric("insert_duration", Trend)
	m.insertErrRate = newMetric("insert_error_rate", Rate)
	m.iterationDur = newMetric("iteration_duration", Trend)
	m.iterations = newMetric("iterations", Counter)
	m.txTotalDuration = newMetric("tx_total_duration", Trend)
	m.txCommitRate = newMetric("tx_commit_rate", Rate)
	m.txQueriesPerTx = newMetric("tx_queries_per_tx", Trend)
	m.txErrorRate = newMetric("tx_error_rate", Rate)
	m.tags = r.RootTagSet()
	m.registered.Store(true)
}

func applyStepTag(tags *TagSet, step string) *TagSet {
	if step != "" {
		return tags.With("step", step)
	}

	return tags
}

func (m *txMetrics) emit(vu *VU, metric *metric, value float64, tags *TagSet) {
	if metric == nil {
		return
	}

	PushIfNotDone(vu.Context(), vu.root.samples, Sample{
		Metric: metric, Tags: tags,
		Time: time.Now(), Value: value,
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

	m.emit(vu, m.runQueryDuration, elapsed.Seconds()*millisPerSecond, tags)
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

	m.emit(vu, m.insertDuration, elapsed.Seconds()*millisPerSecond, tags)
	m.emit(vu, m.insertErrRate, 0, tags)
}

// recordIteration emits iteration_duration + iterations for one Iterate call.
func (m *txMetrics) recordIteration(vu *VU, elapsed time.Duration) {
	m.ensureRegistered(vu, root.lg)
	tags := applyStepTag(m.tags, vu.stepTag)
	m.emit(vu, m.iterationDur, elapsed.Seconds()*millisPerSecond, tags)
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

	m.emit(vu, m.txTotalDuration, elapsed.Seconds()*millisPerSecond, tags)
	m.emit(vu, m.txQueriesPerTx, float64(queries), tags)
}

func (m *txMetrics) recordInsertProgress(vu *VU, snapshot *insertprogress.Snapshot) {
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
		PushIfNotDone(vu.Context(), vu.root.samples, Sample{
			Metric: progressRows, Tags: tags,
			Time: now, Value: float64(snapshot.DeltaRows),
		})
	}

	PushIfNotDone(vu.Context(), vu.root.samples, Sample{
		Metric: progressRPS, Tags: tags,
		Time: now, Value: snapshot.CurrentRowsPerSecond,
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
	PushIfNotDone(vu.Context(), vu.root.samples, Sample{
		Metric: insertRows, Tags: tags,
		Time: now, Value: float64(rows),
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

	PushIfNotDone(vu.Context(), vu.root.samples, Sample{
		Metric: txCount, Tags: tags,
		Time: now, Value: 1,
	})
}

func (m *txMetrics) start(samples chan<- SampleContainer, ctx context.Context) {
	m.startSampler(&m.txSampler, &m.txTotal, ctx, samples, m.txTPS, m.tags)
	m.startSampler(&m.querySampler, &m.queryTotal, ctx, samples, m.runQueryQPS, m.tags)
}

func (m *txMetrics) stop() {
	m.stopSampler(&m.txSampler)
	m.stopSampler(&m.querySampler)
}

func (m *txMetrics) startSampler(
	sampler *throughputSampler, total *uint64, ctx context.Context,
	samples chan<- SampleContainer, metric *metric, tags *TagSet,
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
	case <-time.After(samplerStopGrace):
	}
}

func (m *txMetrics) snapshotCountMetric() (*metric, *TagSet, bool) {
	if !m.registered.Load() {
		return nil, nil, false
	}

	return m.txCount, m.tags, true
}

func (m *txMetrics) snapshotInsertMetrics() (*metric, *TagSet, bool) {
	if !m.registered.Load() {
		return nil, nil, false
	}

	return m.insertRows, m.tags, true
}

func (m *txMetrics) snapshotProgressMetrics() (rowsMetric, rpsMetric *metric, tags *TagSet, ok bool) {
	if !m.registered.Load() {
		return nil, nil, nil, false
	}

	return m.progressRows, m.progressRPS, m.tags, true
}

func runThroughputSampler(
	ctx context.Context, samples chan<- SampleContainer, metric *metric,
	tags *TagSet, total *uint64, stopCh <-chan struct{}, doneCh chan<- struct{},
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
	ctx context.Context, samples chan<- SampleContainer, metric *metric,
	tags *TagSet, totalCounter *uint64, prevTotal uint64, prevTime time.Time,
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

	PushIfNotDone(ctx, samples, Sample{
		Metric: metric, Tags: tags,
		Time: now, Value: float64(delta) / elapsed.Seconds(),
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
