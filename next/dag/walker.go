package dag

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// Run executes b to completion and returns the outcome. It launches each
// node in its own goroutine as it becomes ready and returns only after
// every launched goroutine has exited (no goroutine leaks). Canceling ctx
// mid-run — externally, or via a node's AbortRun failure — resolves every
// not-yet-terminal node to Canceled, with one carve-out: OnFailure-gated
// cleanup nodes survive an AbortRun abort (they run against the caller's
// ctx, not the aborted run context) and are canceled only by ctx itself.
func Run(ctx context.Context, b *Built) *RunResult {
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	w := &walker{
		ext:     ctx,
		ctx:     runCtx,
		cancel:  cancel,
		built:   b,
		results: make(map[string]*NodeResult, len(b.order)),
		pending: make(map[string]int, len(b.order)),
	}

	for id, c := range b.watched {
		w.pending[id] = c
	}

	for _, id := range b.order {
		if b.watched[id] == 0 {
			w.launch(id)
		}
	}

	w.wg.Wait()

	return w.summarize()
}

// walker holds the mutable state of one Run call. mu guards results and
// pending; every other field is set once before any goroutine starts.
type walker struct {
	// ext is the caller's context; ctx is derived from it and is
	// additionally canceled by an AbortRun failure. OnFailure-gated
	// nodes run against ext so cleanup survives an abort.
	ext    context.Context
	ctx    context.Context
	cancel context.CancelFunc
	built  *Built

	wg sync.WaitGroup
	mu sync.Mutex

	results map[string]*NodeResult
	pending map[string]int
}

func (w *walker) launch(id string) {
	w.wg.Add(1)

	go func() {
		defer w.wg.Done()
		w.runNode(id)
	}()
}

func (w *walker) runNode(id string) {
	n := w.built.nodes[id]
	res := &NodeResult{ID: id}

	// OnFailure-gated nodes are exempt from AbortRun's internal
	// cancellation — cleanup must still run after an abort — so they
	// execute against the caller's context. External cancellation
	// reaches them through it.
	ctx := w.ctx
	if len(n.OnFailure) > 0 {
		ctx = w.ext
	}

	switch {
	case ctx.Err() != nil:
		res.Status = Canceled
	case !w.evalGate(n):
		res.Status = Skipped
	case n.If != nil && !n.If():
		res.Status = Skipped
	default:
		res.Start = time.Now()
		res.Status, res.Attempts, res.Err = w.execute(ctx, n)
		res.End = time.Now()
	}

	w.finish(id, res)
}

// evalGate reports whether every edge kind n declares is satisfied by
// its dependencies' current (terminal, by construction) statuses.
func (w *walker) evalGate(n *Node) bool {
	if len(n.After) > 0 {
		for _, d := range n.After {
			s := w.status(d)
			if s == Succeeded {
				continue
			}

			if s == Failed && w.built.nodes[d].Failure == Continue {
				continue
			}

			return false
		}
	}

	if len(n.AfterAny) > 0 && !w.anySucceeded(n.AfterAny) {
		return false
	}

	if len(n.OnFailure) > 0 && !w.anyFailed(n.OnFailure) {
		return false
	}

	return true
}

func (w *walker) anySucceeded(ids []string) bool {
	for _, d := range ids {
		if w.status(d) == Succeeded {
			return true
		}
	}

	return false
}

func (w *walker) anyFailed(ids []string) bool {
	for _, d := range ids {
		if w.status(d) == Failed {
			return true
		}
	}

	return false
}

func (w *walker) status(id string) Status {
	w.mu.Lock()
	defer w.mu.Unlock()

	return w.results[id].Status
}

// execute runs n.Run against ctx, retrying per n.Retry while attempts
// remain, the error is classified retryable, and ctx is not done. A
// panic in Run is recovered and treated as a Failed attempt.
func (w *walker) execute(ctx context.Context, n *Node) (Status, int, error) {
	maxAttempts := n.Retry.MaxAttempts
	if maxAttempts < 1 {
		maxAttempts = 1
	}

	var lastErr error

	attempt := 0
	for attempt < maxAttempts {
		attempt++
		lastErr = call(ctx, n)

		if lastErr == nil {
			return Succeeded, attempt, nil
		}

		if ctx.Err() != nil {
			return Canceled, attempt, lastErr
		}

		if attempt >= maxAttempts || n.Retry.Retryable == nil || !n.Retry.Retryable(lastErr) {
			break
		}

		if !sleep(ctx, n.Retry.Backoff, attempt) {
			return Canceled, attempt, lastErr
		}
	}

	if n.Failure == AbortRun {
		w.cancel()
	}

	return Failed, attempt, lastErr
}

// sleep waits out the backoff for the given attempt, or returns false if
// ctx is done first.
func sleep(ctx context.Context, backoff func(int) time.Duration, attempt int) bool {
	if backoff == nil {
		return true
	}

	timer := time.NewTimer(backoff(attempt))
	defer timer.Stop()

	select {
	case <-timer.C:
		return true
	case <-ctx.Done():
		return false
	}
}

// call invokes n.Run, converting a recovered panic into an error.
func call(ctx context.Context, n *Node) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("dag: panic in node %q: %v", n.ID, r)
		}
	}()

	return n.Run(ctx)
}

// finish records id's result and launches every dependent whose last
// unresolved dependency was id.
func (w *walker) finish(id string, res *NodeResult) {
	w.mu.Lock()
	w.results[id] = res

	var ready []string
	for _, dep := range w.built.dependents[id] {
		w.pending[dep]--
		if w.pending[dep] == 0 {
			ready = append(ready, dep)
		}
	}
	w.mu.Unlock()

	for _, id := range ready {
		w.launch(id)
	}
}

func (w *walker) summarize() *RunResult {
	status := Succeeded

	for _, r := range w.results {
		switch r.Status {
		case Failed:
			status = Failed
		case Canceled:
			if status != Failed {
				status = Canceled
			}
		}
	}

	return &RunResult{Nodes: w.results, Status: status}
}
