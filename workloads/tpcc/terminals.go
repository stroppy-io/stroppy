package tpcc

import (
	"container/heap"
	"context"
	"errors"
	"fmt"
	"math"
	"math/rand/v2"
	"sync"
	"time"

	"github.com/stroppy-io/stroppy/v6/pkg/bench"
)

var errStrictTerminals = errors.New(
	"tpcc: strict terminal model requires constant-vus, transactional isolation, and fixed warehouses",
)

type terminalModel struct {
	mu          sync.Mutex
	changed     chan struct{}
	ready       terminalQueue
	active      int
	started     time.Time
	measurement time.Time
	end         time.Time
	warmup      time.Duration
	duration    time.Duration
	workers     int
	terminals   int
	resultsPath string
	results     *deliveryLog
	deliveries  map[int64][]*terminalJob
	stats       strictStatistics
	cutoff      time.Time
	pace        *pacing
	idle        []*terminalJob
}

type terminal struct {
	state *vuState
	think *rand.Rand
	deck  [23]int
	card  int
}

type terminalJob struct {
	terminal    *terminal
	transaction int
	ready       time.Time
	delivery    *deliveryResult
	keyingStart time.Time
	thinkStart  time.Time
	thinkEnd    time.Time
	previous    int
}

type terminalQueue []*terminalJob

func (q terminalQueue) Len() int           { return len(q) }
func (q terminalQueue) Less(i, j int) bool { return q[i].ready.Before(q[j].ready) }
func (q terminalQueue) Swap(i, j int)      { q[i], q[j] = q[j], q[i] }
func (q *terminalQueue) Push(value any) {
	job, _ := value.(*terminalJob)
	*q = append(*q, job)
}

func (q *terminalQueue) Pop() any {
	old := *q
	job := old[len(old)-1]
	old[len(old)-1] = nil
	*q = old[:len(old)-1]

	return job
}

func (v *vuState) stockDistrict() int64 {
	if v.stockDID != 0 {
		return v.stockDID
	}

	return v.ri(v.slDID, 1, districtsPerWarehouse)
}

func (w *workload) defineTerminals(d *bench.Def, settings bench.RunSettings) error {
	mode, _ := d.Param.String("terminal-model", "stress",
		"TPC-C terminal model; strict derives ten terminals per warehouse.", bench.OneOf("stress", "strict"))
	path, _ := d.Param.String("delivery-results", "tpcc-delivery.jsonl",
		"Exclusive output file for strict deferred Delivery results.")

	if mode != "strict" {
		return nil
	}

	if settings.Executor != "constant-vus" || w.spreadWarehouses || w.driverType == bench.DriverPicodata ||
		w.iso == bench.IsoNone || w.iso == bench.IsoConn {
		return errStrictTerminals
	}

	if w.warehouses > int64(math.MaxInt/districtsPerWarehouse) || w.warehouseStart < 1 ||
		w.warehouseStart > math.MaxInt64-w.warehouses+1 || path == "" {
		return fmt.Errorf("%w: invalid warehouse range or delivery-results path", errStrictTerminals)
	}

	if w.variant == "procs" {
		if _, found := w.sql.Lookup("workload_procs", "delivery_results"); !found {
			return fmt.Errorf("%w: procedure SQL requires workload_procs/delivery_results", errStrictTerminals)
		}
	}

	w.pacing = true
	w.strict = &terminalModel{
		workers: settings.Workers, terminals: int(w.warehouses) * districtsPerWarehouse,
		warmup: w.warmup, duration: settings.Duration, resultsPath: path, changed: make(chan struct{}),
	}

	return nil
}

func (w *workload) runTerminals(d *bench.Def, settings bench.RunSettings) (err error) {
	model := w.strict
	defer func() { err = errors.Join(err, model.close()) }()

	if model.warmup > time.Duration(math.MaxInt64)-model.duration {
		return fmt.Errorf("%w: warmup and duration exceed timer capacity", errStrictTerminals)
	}

	d.Execution.Step("initialize_terminals", func(ctx context.Context, b *bench.Bench) error {
		if err := w.initializeHistory(ctx, b); err != nil {
			return err
		}

		return model.initialize(w)
	})
	d.Execution.Step("workload", w.terminalIteration,
		bench.ConstantWorkers(model.workers, model.warmup+model.duration, settings.Drain))

	d.Execution.Step("delivery_results", func(context.Context, *bench.Bench) error {
		return model.close()
	}, bench.Always(30*time.Second))

	return d.Execution.Err()
}

func (m *terminalModel) initialize(w *workload) error {
	log, err := newDeliveryLog(m.resultsPath)
	if err != nil {
		return err
	}

	m.results = log
	m.pace = w.pace
	m.deliveries = make(map[int64][]*terminalJob)

	w.vuStates = make([]*vuState, m.terminals)
	for i := range m.terminals {
		state := w.vuState(uint64(i)+1, w.warehouseStart, w.warehouses)
		state.homeWID = w.warehouseStart + int64(i/districtsPerWarehouse)
		state.stockDID = int64(i%districtsPerWarehouse) + 1
		t := &terminal{state: state, think: terminalRandom(uint64(i) + 1), card: 23}
		m.ready = append(m.ready, &terminalJob{terminal: t})
	}

	return nil
}

func terminalRandom(id uint64) *rand.Rand {
	//nolint:gosec // workload timing is not security-sensitive.
	return rand.New(rand.NewPCG(seedOf("terminal.think", id), id))
}

func (t *terminal) selectTransaction() int {
	if t.card == len(t.deck) {
		for i := range t.deck {
			switch {
			case i < 10:
				t.deck[i] = 0
			case i < 20:
				t.deck[i] = 1
			default:
				t.deck[i] = i - 18
			}
		}

		t.state.picker.Shuffle(len(t.deck), func(i, j int) { t.deck[i], t.deck[j] = t.deck[j], t.deck[i] })
		t.card = 0
	}

	transaction := t.deck[t.card]
	t.card++

	return transaction
}

func (m *terminalModel) signal() {
	close(m.changed)
	m.changed = make(chan struct{})
}

func (m *terminalModel) start(now time.Time) {
	m.started = now
	m.measurement = now.Add(m.warmup)

	m.end = m.measurement.Add(m.duration)
	if m.pace != nil {
		m.pace.window(m.measurement, m.end)
	}

	if m.cutoff.IsZero() || m.end.Before(m.cutoff) {
		m.cutoff = m.end
	}

	for _, job := range m.ready {
		job.transaction = job.terminal.selectTransaction()

		job.keyingStart = now
		job.ready = now.Add(time.Duration(keyingTime[txNames[job.transaction]]) * time.Second)
	}

	kept := m.ready[:0]
	for _, job := range m.ready {
		if job.ready.Before(m.end) {
			kept = append(kept, job)
		} else {
			m.idle = append(m.idle, job)
		}
	}

	m.ready = kept
	heap.Init(&m.ready)
}

//nolint:nilnil // nil job marks a fully drained terminal population.
func (m *terminalModel) next(ctx context.Context) (*terminalJob, error) {
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		m.mu.Lock()

		now := time.Now()
		if m.started.IsZero() {
			m.start(now)
		}

		if len(m.ready) > 0 && !m.ready[0].ready.After(now) {
			job, _ := heap.Pop(&m.ready).(*terminalJob)
			m.active++
			m.mu.Unlock()

			return job, nil
		}

		if !now.Before(m.end) && len(m.ready) == 0 && m.active == 0 {
			m.mu.Unlock()

			return nil, nil
		}

		wake := m.end
		if len(m.ready) > 0 && m.ready[0].ready.Before(wake) {
			wake = m.ready[0].ready
		}

		changed := m.changed
		m.mu.Unlock()

		if !wake.After(now) {
			wake = now.Add(time.Hour)
		}

		timer := time.NewTimer(time.Until(wake))
		select {
		case <-ctx.Done():
			timer.Stop()

			return nil, ctx.Err()
		case <-changed:
			timer.Stop()
		case <-timer.C:
		}
	}
}

func (m *terminalModel) reschedule(job *terminalJob, now time.Time) {
	t := job.terminal
	think := thinkTime(t.think, thinkTimeMean[txNames[job.transaction]])
	next := t.selectTransaction()

	thinkEnd := now.Add(time.Duration(think * float64(time.Second)))

	nextJob := &terminalJob{
		terminal: t, transaction: next, previous: job.transaction, thinkStart: now, thinkEnd: thinkEnd,
		keyingStart: thinkEnd, ready: thinkEnd.Add(time.Duration(keyingTime[txNames[next]]) * time.Second),
	}
	if nextJob.ready.Before(m.end) {
		heap.Push(&m.ready, nextJob)
	} else {
		m.idle = append(m.idle, nextJob)
	}
}

func (w *workload) terminalIteration(ctx context.Context, b *bench.Bench) (err error) {
	m := w.strict
	canceled := make(chan struct{})

	stop := context.AfterFunc(ctx, func() {
		m.stopMeasurement(time.Now())
		close(canceled)
	})
	defer func() {
		if !stop() {
			<-canceled
		}

		if bench.IsFatalError(err) {
			m.stopMeasurement(time.Now())
		}
	}()

	if m.results == nil {
		return bench.Fatal(fmt.Errorf("%w: initialize_terminals must run before strict traffic", errStrictTerminals))
	}

	for {
		job, err := m.next(ctx)
		if err != nil {
			return err
		}

		if job == nil {
			return nil
		}

		started := job.ready
		m.recordWaits(ctx, job, job.ready)

		switch {
		case job.delivery != nil:
			err = w.finishDelivery(ctx, b, job.delivery)
		case job.transaction == int(txDelivery):
			m.mu.Lock()
			result := &deliveryResult{
				Queued: time.Now(), Warehouse: job.terminal.state.homeWID,
				Carrier: job.terminal.state.ri(job.terminal.state.dCarrier, 1, 10),
			}
			m.enqueueDelivery(result, started)
			m.signal()
			m.mu.Unlock()
		default:
			err = b.LogicalOperation(func() error { return w.terminalTransaction(ctx, b, job.terminal.state, job.transaction) })
		}

		completed := time.Now()

		if job.delivery == nil {
			w.recordTerminal(ctx, job, started, completed, err)
			w.pace.response(job.transaction, job.terminal.state.homeWID, started, completed, err)
		}

		m.complete(ctx, job, started, completed, err)

		if err != nil {
			if bench.IsFatalError(err) || ctx.Err() != nil {
				return err
			}

			b.RecordError("tpcc terminal", err)
		}
	}
}

func (m *terminalModel) recordWaits(ctx context.Context, job *terminalJob, cutoff time.Time) {
	if m.pace == nil || job.delivery != nil {
		return
	}

	if !job.thinkStart.IsZero() && cutoff.After(job.thinkStart) {
		end := job.thinkEnd
		if cutoff.Before(end) {
			end = cutoff
		}

		m.pace.appliedWait(ctx, job.previous, 1, job.thinkStart, end, !cutoff.Before(job.thinkEnd))
	}

	if !job.keyingStart.IsZero() && cutoff.After(job.keyingStart) {
		end := job.ready
		if cutoff.Before(end) {
			end = cutoff
		}

		m.pace.appliedWait(ctx, job.transaction, 0, job.keyingStart, end, !cutoff.Before(job.ready))
	}
}

func (m *terminalModel) stopMeasurement(now time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.cutoff.IsZero() || now.Before(m.cutoff) {
		m.cutoff = now
		if m.pace != nil {
			m.pace.window(m.measurement, now)
		}
	}
}

func (m *terminalModel) enqueueDelivery(result *deliveryResult, now time.Time) {
	if m.deliveries == nil {
		m.deliveries = make(map[int64][]*terminalJob)
	}

	job := &terminalJob{delivery: result, ready: now}

	pending, active := m.deliveries[result.Warehouse]
	if active {
		m.deliveries[result.Warehouse] = append(pending, job)
	} else {
		// One consumer per warehouse prevents two deliveries from selecting the same oldest order.
		m.deliveries[result.Warehouse] = nil
		heap.Push(&m.ready, job)
	}
}

func (m *terminalModel) complete(ctx context.Context, job *terminalJob, started, completed time.Time, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.active--
	if job.delivery == nil {
		m.stats.record(job.transaction, started, completed, m.measurement, m.end, err)

		if ctx.Err() == nil && !bench.IsFatalError(err) {
			m.reschedule(job, completed)
		}
	} else {
		warehouse := job.delivery.Warehouse

		pending := m.deliveries[warehouse]
		if len(pending) > 0 {
			next := pending[0]
			m.deliveries[warehouse] = pending[1:]
			heap.Push(&m.ready, next)
		} else {
			delete(m.deliveries, warehouse)
		}
	}

	m.signal()
}

func (w *workload) recordTerminal(
	ctx context.Context, job *terminalJob, started, completed time.Time, err error,
) {
	phase := "measurement"
	if started.Before(w.strict.measurement) {
		phase = "warmup"
	}

	if !completed.Before(w.strict.end) {
		phase = "drain"
	}

	w.m.durations[txNames[job.transaction]].Record(ctx, completed.Sub(started).Seconds()*1000,
		bench.LabelValue("phase", phase), bench.LabelValue("outcome", operationOutcome(ctx, err)))
}

func (w *workload) terminalTransaction(ctx context.Context, b *bench.Bench, state *vuState, idx int) error {
	if w.variant == "procs" {
		return w.iterateProcs(ctx, b, state, idx)
	}

	switch idx {
	case 0:
		return w.newOrder(ctx, b, state)
	case 1:
		return w.payment(ctx, b, state)
	case 2:
		return w.orderStatus(ctx, b, state)
	default:
		return w.stockLevel(ctx, b, state)
	}
}

func (m *terminalModel) close() error {
	if m.results == nil {
		return nil
	}

	var pendingErr error

	cutoff := time.Now()
	if !m.cutoff.IsZero() && m.cutoff.Before(cutoff) {
		cutoff = m.cutoff
	}

	for _, job := range m.idle {
		m.recordWaits(context.Background(), job, cutoff)
	}

	m.idle = nil

	pending := append(terminalQueue(nil), m.ready...)
	for _, jobs := range m.deliveries {
		pending = append(pending, jobs...)
	}

	for _, job := range pending {
		if job.delivery == nil {
			m.recordWaits(context.Background(), job, cutoff)

			continue
		}

		job.delivery.Completed = time.Now()

		job.delivery.Error = "not executed before drain or cancellation"
		if !job.delivery.Queued.Before(m.measurement) && job.delivery.Queued.Before(m.end) {
			m.stats.DeliveryPending++
		}

		m.pace.delivered(context.Background(), job.delivery, nil, true)
		pendingErr = errors.Join(pendingErr, m.results.write(job.delivery))
	}

	closeErr := m.results.close()
	m.results = nil

	return errors.Join(pendingErr, closeErr)
}
