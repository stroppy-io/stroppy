package insertprogress

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/zap"
)

const (
	// DefaultInterval is the default cadence for InsertSpec progress samples.
	DefaultInterval = 10 * time.Second
	// DefaultStallAfter is the default no-progress duration before warning.
	DefaultStallAfter = time.Minute

	percentageMultiplier = 100
	unknownValue         = "unknown"
)

// ErrInvalidMode is returned when progress mode is not one of off/log/metrics/both.
var ErrInvalidMode = errors.New("insert progress: invalid mode")

// Mode controls where InsertSpec progress samples are emitted.
type Mode string

const (
	// ModeOff disables the InsertSpec progress watcher.
	ModeOff Mode = "off"
	// ModeLog emits progress samples to the logger.
	ModeLog Mode = "log"
	// ModeMetrics emits progress samples to the k6 metrics callback.
	ModeMetrics Mode = "metrics"
	// ModeBoth emits progress samples to both logger and metrics callback.
	ModeBoth Mode = "both"
)

// Event describes the lifecycle stage of a progress sample.
type Event string

const (
	// EventProgress is a regular interval sample.
	EventProgress Event = "progress"
	// EventStall is an interval sample emitted after no row progress for StallAfter.
	EventStall Event = "stall"
	// EventCompleted is the final sample for a successful InsertSpec.
	EventCompleted Event = "completed"
	// EventFailed is the final sample for a failed InsertSpec.
	EventFailed Event = "failed"
)

// Config defines one InsertSpec progress tracker.
type Config struct {
	Enabled    bool
	Interval   time.Duration
	StallAfter time.Duration
	Mode       Mode
	Table      string
	Method     string
	Workers    int
	Logger     *zap.Logger
	OnSample   func(Snapshot)
}

// Snapshot is one progress sample emitted by the watcher.
type Snapshot struct {
	Event                Event
	Table                string
	Method               string
	RowKind              string
	Stage                string
	Workers              int
	TotalRows            int64
	GeneratedRows        int64
	ConfirmedRows        int64
	Rows                 int64
	InflightRows         int64
	DeltaRows            int64
	Batches              int64
	Elapsed              time.Duration
	Interval             time.Duration
	StallDuration        time.Duration
	ETA                  time.Duration
	AvgBatchDuration     time.Duration
	CurrentRowsPerSecond float64
	AvgRowsPerSecond     float64
	Percent              float64
	Stalled              bool
}

// Tracker stores mutable InsertSpec progress state and owns the watcher goroutine.
type Tracker struct {
	config Config

	startedAt time.Time
	doneCh    chan struct{}
	doneOnce  sync.Once
	startOnce sync.Once
	waitGroup sync.WaitGroup

	totalRows     atomic.Int64
	generatedRows atomic.Int64
	confirmedRows atomic.Int64
	batches       atomic.Int64
	batchNanos    atomic.Int64
	workers       atomic.Int64
	lastProgress  atomic.Int64

	stateMu        sync.Mutex
	lastSampleRows int64
	lastSampleTime time.Time
	stages         map[int]string
}

// DefaultConfig returns the default InsertSpec progress configuration.
func DefaultConfig() Config {
	return Config{
		Enabled:    true,
		Interval:   DefaultInterval,
		StallAfter: DefaultStallAfter,
		Mode:       ModeBoth,
	}
}

// ParseMode parses a user-facing progress mode.
func ParseMode(raw string) (Mode, error) {
	mode := Mode(strings.ToLower(strings.TrimSpace(raw)))

	switch mode {
	case "":
		return ModeBoth, nil
	case ModeOff, ModeLog, ModeMetrics, ModeBoth:
		return mode, nil
	default:
		return "", fmt.Errorf("%w %q", ErrInvalidMode, raw)
	}
}

// NewTracker creates an InsertSpec progress tracker.
func NewTracker(config *Config) *Tracker {
	normalized := normalizeConfig(config)
	now := time.Now()

	tracker := &Tracker{
		config:         normalized,
		startedAt:      now,
		doneCh:         make(chan struct{}),
		lastSampleTime: now,
		stages:         make(map[int]string),
	}
	tracker.workers.Store(int64(normalized.Workers))
	tracker.lastProgress.Store(now.UnixNano())

	return tracker
}

// Enabled reports whether this tracker should emit progress samples.
func (tracker *Tracker) Enabled() bool {
	if tracker == nil {
		return false
	}

	return tracker.config.Enabled && tracker.config.Mode != ModeOff
}

// Start starts the watcher goroutine.
func (tracker *Tracker) Start(ctx context.Context) {
	if !tracker.Enabled() {
		return
	}

	tracker.startOnce.Do(func() {
		tracker.waitGroup.Add(1)

		go tracker.watch(ctx)
	})
}

// Finish stops the watcher and emits a final sample.
func (tracker *Tracker) Finish(operationErr error) Snapshot {
	if tracker == nil {
		return Snapshot{}
	}

	tracker.doneOnce.Do(func() {
		close(tracker.doneCh)
	})
	tracker.waitGroup.Wait()

	event := EventCompleted
	if operationErr != nil {
		event = EventFailed
	}

	snapshot := tracker.nextSnapshot(time.Now(), event)
	if tracker.Enabled() {
		tracker.emit(&snapshot)
	}

	return snapshot
}

// SetTotal records the actual runtime row count for this InsertSpec.
func (tracker *Tracker) SetTotal(rows int64) {
	if tracker == nil || rows < 0 {
		return
	}

	tracker.totalRows.Store(rows)
}

// SetWorkers records the effective worker count for this InsertSpec.
func (tracker *Tracker) SetWorkers(workers int) {
	if tracker == nil || workers <= 0 {
		return
	}

	tracker.workers.Store(int64(workers))
}

// SetStage records the current stage for one InsertSpec worker.
func (tracker *Tracker) SetStage(workerIndex int, stage string) {
	if tracker == nil || stage == "" {
		return
	}

	tracker.stateMu.Lock()
	tracker.stages[workerIndex] = stage
	tracker.stateMu.Unlock()
}

// AddGenerated records rows produced by datagen/runtime.
func (tracker *Tracker) AddGenerated(_ int, rows int64) {
	if tracker == nil || rows <= 0 {
		return
	}

	tracker.generatedRows.Add(rows)
	tracker.markProgress()
}

// AddConfirmed records rows accepted by a completed driver write unit.
func (tracker *Tracker) AddConfirmed(_ int, rows int64) {
	if tracker == nil || rows <= 0 {
		return
	}

	tracker.confirmedRows.Add(rows)
	tracker.markProgress()
}

// AddBatch records one completed driver write unit.
func (tracker *Tracker) AddBatch(_ int, rows int64, elapsed time.Duration) {
	if tracker == nil || rows <= 0 {
		return
	}

	tracker.batches.Add(1)

	if elapsed > 0 {
		tracker.batchNanos.Add(elapsed.Nanoseconds())
	}
}

func normalizeConfig(config *Config) Config {
	normalized := DefaultConfig()
	if config != nil {
		normalized = *config
	}

	if normalized.Interval <= 0 {
		normalized.Interval = DefaultInterval
	}

	if normalized.StallAfter <= 0 {
		normalized.StallAfter = DefaultStallAfter
	}

	if normalized.Mode == "" {
		normalized.Mode = ModeBoth
	}

	if normalized.Table == "" {
		normalized.Table = unknownValue
	}

	if normalized.Method == "" {
		normalized.Method = unknownValue
	}

	if normalized.Workers <= 0 {
		normalized.Workers = 1
	}

	if normalized.Logger == nil {
		normalized.Logger = zap.NewNop()
	}

	return normalized
}

func (tracker *Tracker) watch(ctx context.Context) {
	defer tracker.waitGroup.Done()

	ticker := time.NewTicker(tracker.config.Interval)
	defer ticker.Stop()

	for {
		select {
		case now := <-ticker.C:
			snapshot := tracker.nextSnapshot(now, EventProgress)
			tracker.emit(&snapshot)
		case <-tracker.doneCh:
			return
		case <-ctx.Done():
			return
		}
	}
}

func (tracker *Tracker) nextSnapshot(now time.Time, event Event) Snapshot {
	generatedRows := tracker.generatedRows.Load()
	confirmedRows := tracker.confirmedRows.Load()
	batchCount := tracker.batches.Load()
	rows, rowKind := displayedRows(generatedRows, confirmedRows)

	tracker.stateMu.Lock()
	previousRows := tracker.lastSampleRows
	previousTime := tracker.lastSampleTime
	tracker.lastSampleRows = rows
	tracker.lastSampleTime = now
	stage := summarizeStages(tracker.stages)
	tracker.stateMu.Unlock()

	deltaRows := rows - previousRows
	if deltaRows < 0 {
		deltaRows = 0
	}

	interval := now.Sub(previousTime)
	elapsed := now.Sub(tracker.startedAt)
	stallDuration := now.Sub(time.Unix(0, tracker.lastProgress.Load()))

	snapshot := Snapshot{
		Event:         event,
		Table:         tracker.config.Table,
		Method:        tracker.config.Method,
		RowKind:       rowKind,
		Stage:         stage,
		Workers:       int(tracker.workers.Load()),
		TotalRows:     tracker.totalRows.Load(),
		GeneratedRows: generatedRows,
		ConfirmedRows: confirmedRows,
		Rows:          rows,
		InflightRows:  max(generatedRows-confirmedRows, 0),
		DeltaRows:     deltaRows,
		Batches:       batchCount,
		Elapsed:       elapsed,
		Interval:      interval,
		StallDuration: stallDuration,
	}
	if batchCount > 0 {
		snapshot.AvgBatchDuration = time.Duration(tracker.batchNanos.Load() / batchCount)
	}

	populateRates(&snapshot)
	tracker.detectStall(&snapshot)

	return snapshot
}

func displayedRows(generatedRows, confirmedRows int64) (rows int64, rowKind string) {
	if confirmedRows >= generatedRows {
		return confirmedRows, "confirmed"
	}

	return generatedRows, "generated"
}

func populateRates(snapshot *Snapshot) {
	if snapshot.Interval > 0 {
		snapshot.CurrentRowsPerSecond = float64(snapshot.DeltaRows) / snapshot.Interval.Seconds()
	}

	if snapshot.Elapsed > 0 {
		snapshot.AvgRowsPerSecond = float64(snapshot.Rows) / snapshot.Elapsed.Seconds()
	}

	if snapshot.TotalRows > 0 {
		snapshot.Percent = float64(snapshot.Rows) * percentageMultiplier / float64(snapshot.TotalRows)
	}

	if snapshot.AvgRowsPerSecond > 0 && snapshot.TotalRows > snapshot.Rows {
		remainingRows := snapshot.TotalRows - snapshot.Rows
		snapshot.ETA = time.Duration(float64(time.Second) * float64(remainingRows) / snapshot.AvgRowsPerSecond)
	}
}

func (tracker *Tracker) detectStall(snapshot *Snapshot) {
	if snapshot.Event != EventProgress || tracker.config.StallAfter <= 0 {
		return
	}

	if snapshot.StallDuration < tracker.config.StallAfter {
		return
	}

	if snapshot.TotalRows > 0 && snapshot.Rows >= snapshot.TotalRows {
		return
	}

	snapshot.Stalled = true
	snapshot.Event = EventStall
}

func (tracker *Tracker) emit(snapshot *Snapshot) {
	if tracker.config.Mode.metrics() && tracker.config.OnSample != nil {
		tracker.config.OnSample(*snapshot)
	}

	if !tracker.config.Mode.logs() {
		return
	}

	fields := snapshotFields(snapshot)
	if snapshot.Event == EventStall || snapshot.Event == EventFailed {
		tracker.config.Logger.Warn("insert progress", fields...)

		return
	}

	tracker.config.Logger.Info("insert progress", fields...)
}

func snapshotFields(snapshot *Snapshot) []zap.Field {
	fields := []zap.Field{
		zap.String("event", string(snapshot.Event)),
		zap.String("table", snapshot.Table),
		zap.String("method", snapshot.Method),
		zap.String("row_kind", snapshot.RowKind),
		zap.String("stage", snapshot.Stage),
		zap.Int("workers", snapshot.Workers),
		zap.Int64("rows", snapshot.Rows),
		zap.Int64("total_rows", snapshot.TotalRows),
		zap.Int64("generated_rows", snapshot.GeneratedRows),
		zap.Int64("confirmed_rows", snapshot.ConfirmedRows),
		zap.Int64("inflight_rows", snapshot.InflightRows),
		zap.Int64("delta_rows", snapshot.DeltaRows),
		zap.Int64("batches", snapshot.Batches),
		zap.Duration("elapsed", snapshot.Elapsed),
		zap.Duration("sample_interval", snapshot.Interval),
		zap.Duration("avg_batch_duration", snapshot.AvgBatchDuration),
		zap.Float64("current_rows_per_second", snapshot.CurrentRowsPerSecond),
		zap.Float64("avg_rows_per_second", snapshot.AvgRowsPerSecond),
		zap.Float64("percent", snapshot.Percent),
	}

	if snapshot.ETA > 0 {
		fields = append(fields, zap.Duration("eta", snapshot.ETA))
	}

	if snapshot.Stalled {
		fields = append(fields, zap.Duration("stall_duration", snapshot.StallDuration))
	}

	return fields
}

func summarizeStages(stages map[int]string) string {
	if len(stages) == 0 {
		return unknownValue
	}

	counts := make(map[string]int, len(stages))
	for _, stage := range stages {
		counts[stage]++
	}

	names := make([]string, 0, len(counts))
	for stage := range counts {
		names = append(names, stage)
	}

	sort.Strings(names)

	parts := make([]string, 0, len(names))
	for _, stage := range names {
		count := counts[stage]
		if count == 1 {
			parts = append(parts, stage)
		} else {
			parts = append(parts, fmt.Sprintf("%s:%d", stage, count))
		}
	}

	return strings.Join(parts, ",")
}

func (tracker *Tracker) markProgress() {
	tracker.lastProgress.Store(time.Now().UnixNano())
}

func (mode Mode) logs() bool {
	return mode == ModeLog || mode == ModeBoth
}

func (mode Mode) metrics() bool {
	return mode == ModeMetrics || mode == ModeBoth
}
