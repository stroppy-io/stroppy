// Package insertprogress provides driver-agnostic InsertSpec progress tracking.
package insertprogress

import "context"

type (
	trackerContextKey struct{}
	workerContextKey  struct{}
)

// ContextWithTracker attaches an InsertSpec progress tracker to ctx.
func ContextWithTracker(ctx context.Context, tracker *Tracker) context.Context {
	if tracker == nil {
		return ctx
	}

	return context.WithValue(ctx, trackerContextKey{}, tracker)
}

// FromContext returns the InsertSpec progress tracker attached to ctx, if any.
func FromContext(ctx context.Context) *Tracker {
	if ctx == nil {
		return nil
	}

	tracker, _ := ctx.Value(trackerContextKey{}).(*Tracker)

	return tracker
}

// ContextWithWorker attaches the current InsertSpec worker index to ctx.
func ContextWithWorker(ctx context.Context, workerIndex int) context.Context {
	return context.WithValue(ctx, workerContextKey{}, workerIndex)
}

// WorkerFromContext returns the current InsertSpec worker index.
func WorkerFromContext(ctx context.Context) int {
	if ctx == nil {
		return 0
	}

	workerIndex, _ := ctx.Value(workerContextKey{}).(int)

	return workerIndex
}
