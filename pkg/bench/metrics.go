package bench

import (
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/zap"

	"github.com/stroppy-io/stroppy/pkg/common/proto/stroppy"
	"github.com/stroppy-io/stroppy/pkg/driver/insertprogress"
)

const (
	millisPerSecond = 1000.0
	percentScale    = 100.0
	medianP         = 0.5
	p90             = 0.9
	p95             = 0.95
	p99             = 0.99
)

type txMetrics struct {
	mu         sync.Mutex
	registered atomic.Bool

	transactions     *metric
	queryOperations  *metric
	queryErrors      *metric
	insertRows       *metric
	progressRows     *metric
	progressRPS      *metric
	queryDuration    *metric
	insertOperations *metric
	insertErrors     *metric
	insertDuration   *metric
	iterationDur     *metric
	iterations       *metric
	txTotalDuration  *metric
	txCommits        *metric
	txErrors         *metric
	txQueriesPerTx   *metric
	stepAttrs        attributeCache
	tableAttrs       attributeCache
	txAttrs          attributeCache
	progressAttrs    attributeCache
}

type attributeCache struct {
	values sync.Map
	size   atomic.Int64
}

type tableAttributeKey struct {
	step  string
	table string
}

type txAttributeKey struct {
	step      string
	action    string
	name      string
	isolation string
}

type progressAttributeKey struct {
	step, table, method, event, rowKind string
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

	m.transactions = newMetric("transactions_total", Counter)
	m.queryOperations = newMetric("run_query_operations_total", Counter)
	m.queryErrors = newMetric("run_query_errors_total", Counter)
	m.insertRows = newMetric("insert_rows_total", Counter)
	m.progressRows = newMetric("insert_progress_rows_total", Counter)
	m.progressRPS = newMetric("insert_progress_rows_per_second", Gauge)
	m.queryDuration = newMetric("run_query_duration", Trend)
	m.insertOperations = newMetric("insert_operations_total", Counter)
	m.insertErrors = newMetric("insert_errors_total", Counter)
	m.insertDuration = newMetric("insert_duration", Trend)
	m.iterationDur = newMetric("iteration_duration", Trend)
	m.iterations = newMetric("iterations_total", Counter)
	m.txTotalDuration = newMetric("tx_total_duration", Trend)
	m.txCommits = newMetric("tx_commits_total", Counter)
	m.txErrors = newMetric("tx_errors_total", Counter)
	m.txQueriesPerTx = newMetric("tx_queries_per_tx", Trend)
	m.registered.Store(true)
}

func (m *txMetrics) emit(vu *VU, metric *metric, value float64, attrs metricAttributes) {
	if metric != nil {
		metric.add(vu.Context(), value, attrs)
	}
}

func cachedAttributes(cache *attributeCache, key any, tags ...string) metricAttributes {
	if cached, ok := cache.values.Load(key); ok {
		attrs, valid := cached.(metricAttributes)
		if valid {
			return attrs
		}
	}

	attrs := attributes(tags...)

	if cache.size.Add(1) > metricCardinalityLimit {
		cache.size.Add(-1)

		return attrs
	}

	cached, loaded := cache.values.LoadOrStore(key, attrs)
	if loaded {
		cache.size.Add(-1)
	}

	result, valid := cached.(metricAttributes)
	if !valid {
		return attrs
	}

	return result
}

func (m *txMetrics) stepAttributes(step string) metricAttributes {
	if step == "" {
		return cachedAttributes(&m.stepAttrs, step)
	}

	return cachedAttributes(&m.stepAttrs, step, "step", step)
}

func (m *txMetrics) tableAttributes(step, table string) metricAttributes {
	key := tableAttributeKey{step: step, table: table}
	if step == "" {
		return cachedAttributes(&m.tableAttrs, key, "table_name", table)
	}

	return cachedAttributes(&m.tableAttrs, key, "step", step, "table_name", table)
}

func (m *txMetrics) txAttributes(step, action, name, isolation string) metricAttributes {
	key := txAttributeKey{step: step, action: action, name: name, isolation: isolation}

	return cachedAttributes(
		&m.txAttrs, key,
		"step", step,
		"tx_action", action,
		"tx_name", name,
		"tx_isolation", isolation,
	)
}

func (m *txMetrics) progressAttributes(snapshot *insertprogress.Snapshot, step string) metricAttributes {
	key := progressAttributeKey{
		step: step, table: snapshot.Table, method: snapshot.Method,
		event: string(snapshot.Event), rowKind: snapshot.RowKind,
	}

	return cachedAttributes(
		&m.progressAttrs, key,
		"step", step,
		"table_name", snapshot.Table,
		"method", snapshot.Method,
		"event", string(snapshot.Event),
		"row_kind", snapshot.RowKind,
	)
}

func (m *txMetrics) recordQueryResult(vu *VU, elapsed time.Duration, queryErr error) {
	m.ensureRegistered(vu, root.lg)
	attrs := m.stepAttributes(vu.stepTag)
	m.emit(vu, m.queryOperations, 1, attrs)

	if queryErr != nil {
		m.emit(vu, m.queryErrors, 1, attrs)

		return
	}

	m.emit(vu, m.queryDuration, elapsed.Seconds()*millisPerSecond, attrs)
}

func (m *txMetrics) recordInsertResult(vu *VU, table string, elapsed time.Duration, insertErr error) {
	m.ensureRegistered(vu, root.lg)

	if table == "" {
		table = "unknown"
	}

	attrs := m.tableAttributes(vu.stepTag, table)
	m.emit(vu, m.insertOperations, 1, attrs)

	if insertErr != nil {
		m.emit(vu, m.insertErrors, 1, attrs)

		return
	}

	m.emit(vu, m.insertDuration, elapsed.Seconds()*millisPerSecond, attrs)
}

func (m *txMetrics) recordIteration(vu *VU, elapsed time.Duration) {
	m.ensureRegistered(vu, root.lg)
	attrs := m.stepAttributes(vu.stepTag)
	m.emit(vu, m.iterationDur, elapsed.Seconds()*millisPerSecond, attrs)
	m.emit(vu, m.iterations, 1, attrs)
}

func (m *txMetrics) recordTxEnd(vu *VU, name string, elapsed time.Duration, queries int, committed bool) {
	m.ensureRegistered(vu, root.lg)

	attrs := m.txAttributes(vu.stepTag, "", name, "")
	if committed {
		m.emit(vu, m.txCommits, 1, attrs)
	} else {
		m.emit(vu, m.txErrors, 1, attrs)
	}

	m.emit(vu, m.txTotalDuration, elapsed.Seconds()*millisPerSecond, attrs)
	m.emit(vu, m.txQueriesPerTx, float64(queries), attrs)
}

func (m *txMetrics) recordInsertProgress(vu *VU, snapshot *insertprogress.Snapshot) {
	m.ensureRegistered(vu, root.lg)

	attrs := m.progressAttributes(snapshot, vu.stepTag)
	if snapshot.DeltaRows > 0 {
		m.emit(vu, m.progressRows, float64(snapshot.DeltaRows), attrs)
	}

	m.emit(vu, m.progressRPS, snapshot.CurrentRowsPerSecond, attrs)
}

func (m *txMetrics) recordInsert(vu *VU, table string, rows int64) {
	m.ensureRegistered(vu, root.lg)

	if table == "" {
		table = "unknown"
	}

	if rows < 0 {
		rows = 0
	}

	m.emit(vu, m.insertRows, float64(rows), m.tableAttributes(vu.stepTag, table))
}

func (m *txMetrics) record(vu *VU, action, name string, isolation stroppy.TxIsolationLevel) {
	m.ensureRegistered(vu, root.lg)
	m.emit(vu, m.transactions, 1, m.txAttributes(vu.stepTag, action, name, txIsolationName(isolation)))
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
