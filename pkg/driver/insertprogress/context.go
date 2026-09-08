// Package insertprogress provides driver-agnostic insert operation progress tracking.
package insertprogress

import "context"

type (
	trackerContextKey struct{}
	workerContextKey  struct{}
)

// ContextWithTracker attaches an insert operation progress tracker to ctx.
func ContextWithTracker(ctx context.Context, tracker *Tracker) context.Context {
	if tracker == nil {
		return ctx
	}

	return context.WithValue(ctx, trackerContextKey{}, tracker)
}

// FromContext returns the insert operation progress tracker attached to ctx, if any.
func FromContext(ctx context.Context) *Tracker {
	if ctx == nil {
		return nil
	}

	tracker, _ := ctx.Value(trackerContextKey{}).(*Tracker)

	return tracker
}

// ContextWithWorker attaches the current insert operation worker index to ctx.
func ContextWithWorker(ctx context.Context, workerIndex int) context.Context {
	return context.WithValue(ctx, workerContextKey{}, workerIndex)
}

// WorkerFromContext returns the current insert operation worker index.
func WorkerFromContext(ctx context.Context) int {
	if ctx == nil {
		return 0
	}

	workerIndex, _ := ctx.Value(workerContextKey{}).(int)

	return workerIndex
}

// Canceled reports whether ctx has been canceled. Insert workers call it
// inside their row-drain loops so cancellation propagates through
// errgroup.WithContext and unblocks the worker instead of letting it drain the
// whole table. The non-blocking select keeps the per-row cost negligible.
func Canceled(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		return nil
	}
}
