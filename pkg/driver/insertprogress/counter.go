package insertprogress

import "context"

const defaultRowReportEvery int64 = 1000

// RowCounter batches per-row progress updates to avoid an atomic operation per generated row.
type RowCounter struct {
	tracker     *Tracker
	workerIndex int
	flushEvery  int64
	pending     int64
	confirm     bool
}

// NewGeneratedRowCounter returns a row counter that reports generated rows.
func NewGeneratedRowCounter(ctx context.Context) RowCounter {
	return RowCounter{
		tracker:     FromContext(ctx),
		workerIndex: WorkerFromContext(ctx),
		flushEvery:  defaultRowReportEvery,
	}
}

// NewConfirmedRowCounter returns a row counter that reports confirmed rows.
func NewConfirmedRowCounter(ctx context.Context) RowCounter {
	return RowCounter{
		tracker:     FromContext(ctx),
		workerIndex: WorkerFromContext(ctx),
		flushEvery:  defaultRowReportEvery,
		confirm:     true,
	}
}

// Add records rows and flushes to the tracker when the local buffer is large enough.
func (counter *RowCounter) Add(rows int64) {
	if counter == nil || counter.tracker == nil || rows <= 0 {
		return
	}

	counter.pending += rows
	if counter.pending >= counter.flushEvery {
		counter.Flush()
	}
}

// Flush sends any pending rows to the tracker.
func (counter *RowCounter) Flush() {
	if counter == nil || counter.tracker == nil || counter.pending <= 0 {
		return
	}

	if counter.confirm {
		counter.tracker.AddConfirmed(counter.workerIndex, counter.pending)
	} else {
		counter.tracker.AddGenerated(counter.workerIndex, counter.pending)
	}

	counter.pending = 0
}

// AddGenerated records generated rows on the tracker attached to ctx.
func AddGenerated(ctx context.Context, rows int64) {
	ctxTracker := FromContext(ctx)
	if ctxTracker == nil {
		return
	}

	ctxTracker.AddGenerated(WorkerFromContext(ctx), rows)
}

// AddConfirmed records confirmed rows on the tracker attached to ctx.
func AddConfirmed(ctx context.Context, rows int64) {
	ctxTracker := FromContext(ctx)
	if ctxTracker == nil {
		return
	}

	ctxTracker.AddConfirmed(WorkerFromContext(ctx), rows)
}
