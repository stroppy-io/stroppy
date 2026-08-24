package bench

import (
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/zap"

	"github.com/stroppy-io/stroppy/pkg/driver"
)

const (
	maxErrorGroups         = 32
	maxErrorOperationBytes = 64
	errorReportInterval    = 30 * time.Second
)

type terminalErrorScope uint8

const (
	terminalErrorIteration terminalErrorScope = iota
	terminalErrorQuery
)

type errorGroup struct {
	operation string
	kind      driver.ErrorKind
}

type errorGroupSummary struct {
	errorGroup
	count uint64
}

type errorSummary struct {
	terminalErrors   uint64
	failedIterations uint64
	failedQueries    uint64
	retryAttempts    uint64
	groups           []errorGroupSummary
}

type errorGroupState struct {
	count atomic.Uint64
}

type errorReporter struct {
	lg       *zap.Logger
	interval time.Duration
	stop     chan struct{}
	done     chan struct{}
	stopOnce sync.Once

	mu     sync.RWMutex
	groups map[errorGroup]*errorGroupState

	failedIterations atomic.Uint64
	failedQueries    atomic.Uint64
	retryAttempts    atomic.Uint64
	lastPeriodic     uint64
}

func newErrorReporter(lg *zap.Logger, interval time.Duration) *errorReporter {
	if lg == nil {
		lg = zap.NewNop()
	}
	if interval <= 0 {
		interval = errorReportInterval
	}

	r := &errorReporter{
		lg:       lg,
		interval: interval,
		stop:     make(chan struct{}),
		done:     make(chan struct{}),
		groups:   make(map[errorGroup]*errorGroupState, maxErrorGroups+1),
	}
	go r.run()

	return r
}

func (r *errorReporter) run() {
	ticker := time.NewTicker(r.interval)
	defer func() {
		ticker.Stop()
		close(r.done)
	}()

	for {
		select {
		case <-ticker.C:
			r.reportPeriodic()
		case <-r.stop:
			return
		}
	}
}

func (r *errorReporter) stopAndWait() {
	if r == nil {
		return
	}

	r.stopOnce.Do(func() { close(r.stop) })
	<-r.done
}

func (r *errorReporter) record(
	vu *VU,
	scope terminalErrorScope,
	operation string,
	err error,
	classify func(error) driver.ErrorFacts,
) {
	if r == nil || err == nil {
		return
	}

	kind := classifyError(err, classify).Kind
	group := errorGroup{operation: boundErrorOperation(operation), kind: normalizeErrorKind(kind)}
	group, state := r.groupState(group)
	count := state.count.Add(1)

	switch scope {
	case terminalErrorIteration:
		r.failedIterations.Add(1)
	case terminalErrorQuery:
		r.failedQueries.Add(1)
	}

	first := count == 1

	if vu != nil && vu.root != nil && vu.root.txMetrics != nil {
		vu.root.txMetrics.recordTerminalError(vu, scope, group)
	}

	if first {
		r.lg.Warn(
			"nonfatal error; continuing",
			zap.String("operation", group.operation),
			zap.String("error_class", string(group.kind)),
			zap.Error(err),
		)
	}
}

func (r *errorReporter) groupState(group errorGroup) (errorGroup, *errorGroupState) {
	r.mu.RLock()
	state := r.groups[group]
	r.mu.RUnlock()
	if state != nil {
		return group, state
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if state = r.groups[group]; state != nil {
		return group, state
	}
	if len(r.groups) >= maxErrorGroups {
		group = errorGroup{operation: "other", kind: driver.ErrorKindUnknown}
		if state = r.groups[group]; state != nil {
			return group, state
		}
	} else {
		group.operation = strings.Clone(group.operation)
	}

	state = &errorGroupState{}
	r.groups[group] = state

	return group, state
}

func (r *errorReporter) recordRetry(vu *VU) {
	if r == nil {
		return
	}

	r.retryAttempts.Add(1)

	if vu != nil && vu.root != nil && vu.root.txMetrics != nil {
		vu.root.txMetrics.recordRetry(vu)
	}
}

func (r *errorReporter) reportPeriodic() {
	failedIterations := r.failedIterations.Load()
	failedQueries := r.failedQueries.Load()
	total := failedIterations + failedQueries
	if total == r.lastPeriodic {
		return
	}

	newErrors := total - r.lastPeriodic
	r.lastPeriodic = total
	retryAttempts := r.retryAttempts.Load()

	r.mu.RLock()
	groupCount := uint64(len(r.groups))
	r.mu.RUnlock()

	var suppressed uint64
	if total > groupCount {
		suppressed = total - groupCount
	}

	r.lg.Warn(
		"nonfatal errors continue",
		zap.Uint64("new_errors", newErrors),
		zap.Uint64("terminal_errors", total),
		zap.Uint64("suppressed_events", suppressed),
		zap.Uint64("failed_iterations", failedIterations),
		zap.Uint64("failed_queries", failedQueries),
		zap.Uint64("retry_attempts", retryAttempts),
	)
}

func (r *errorReporter) snapshot() errorSummary {
	if r == nil {
		return errorSummary{}
	}

	failedIterations := r.failedIterations.Load()
	failedQueries := r.failedQueries.Load()
	summary := errorSummary{
		terminalErrors:   failedIterations + failedQueries,
		failedIterations: failedIterations,
		failedQueries:    failedQueries,
		retryAttempts:    r.retryAttempts.Load(),
	}

	r.mu.RLock()
	summary.groups = make([]errorGroupSummary, 0, len(r.groups))
	for group, state := range r.groups {
		summary.groups = append(summary.groups, errorGroupSummary{errorGroup: group, count: state.count.Load()})
	}
	r.mu.RUnlock()

	slices.SortFunc(summary.groups, func(a, b errorGroupSummary) int {
		if byOperation := strings.Compare(a.operation, b.operation); byOperation != 0 {
			return byOperation
		}

		return strings.Compare(string(a.kind), string(b.kind))
	})

	return summary
}

func (r *errorReporter) writeSummary(w io.Writer) {
	summary := r.snapshot()
	if summary.terminalErrors == 0 {
		return
	}

	fmt.Fprintln(w, "\n=== bench completed with errors ===")
	fmt.Fprintf(w, "  %-40s %d\n", "terminal_errors_total", summary.terminalErrors)
	fmt.Fprintf(w, "  %-40s %d\n", "failed_iterations_total", summary.failedIterations)
	fmt.Fprintf(w, "  %-40s %d\n", "failed_queries_total", summary.failedQueries)
	fmt.Fprintf(w, "  %-40s %d\n", "retry_attempts_total", summary.retryAttempts)
	fmt.Fprintln(w, "  representative error groups:")

	for _, group := range summary.groups {
		fmt.Fprintf(
			w,
			"    operation=%-20s class=%-14s count=%d\n",
			group.operation,
			group.kind,
			group.count,
		)
	}
}

func boundErrorOperation(operation string) string {
	if len(operation) > maxErrorOperationBytes {
		operation = operation[:maxErrorOperationBytes]
	}

	operation = strings.TrimSpace(strings.ToValidUTF8(operation, "?"))
	if operation == "" {
		return "unknown"
	}

	return operation
}

func normalizeErrorKind(kind driver.ErrorKind) driver.ErrorKind {
	switch kind {
	case driver.ErrorKindUnknown,
		driver.ErrorKindSerialization,
		driver.ErrorKindDeadlock,
		driver.ErrorKindLockTimeout,
		driver.ErrorKindTransient,
		driver.ErrorKindUnsupported,
		driver.ErrorKindCanceled,
		driver.ErrorKindTimeout:
		return kind
	default:
		return driver.ErrorKindUnknown
	}
}

func canceledError(ctx context.Context, err error) bool {
	return ctx != nil && ctx.Err() != nil && errors.Is(err, ctx.Err())
}

// RecordQueryError records a query-set error after the workload has decided not
// to retry it. The query set may continue and the run remains successful.
func (b *Bench) RecordQueryError(operation string, err error) {
	if b == nil || b.root == nil || b.root.errorReporter == nil || b.vu == nil || err == nil {
		return
	}
	if canceledError(b.vu.Context(), err) {
		return
	}

	var classify func(error) driver.ErrorFacts
	if b.drv != nil {
		classify = b.drv.ClassifyError
	}

	b.root.errorReporter.record(b.vu, terminalErrorQuery, operation, err, classify)
}
