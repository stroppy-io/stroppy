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
//
// F3: a Skipped dependency satisfies After/AfterAny just like Succeeded —
// skip unblocks dependents (skip an extra step and the rest still runs).
// Requiredness is the author's guarantee that a Skipped step is safe to run
// its dependents after; the operator may only skip author-marked-Skippable
// steps (enforced at the bench layer), so a skip can't break required
// structure. Canceled never satisfies: an aborted run must not release new
// work. A Failed dependency satisfies After only under FailurePolicy Continue.
func (w *walker) evalGate(n *Node) bool {
	if len(n.After) > 0 {
		for _, d := range n.After {
			if satisfies(w.status(d), w.built.nodes[d]) {
				continue
			}
			return false
		}
	}

	if len(n.AfterAny) > 0 && !w.anySatisfied(n.AfterAny) {
		return false
	}

	if len(n.OnFailure) > 0 && !w.anyFailed(n.OnFailure) {
		return false
	}

	return true
}

// satisfies reports whether dependency status s unblocks an After-edge dependent
// of the node dep: Succeeded or Skipped (F3) always; Failed only under Continue.
func satisfies(s Status, dep *Node) bool {
	if s == Succeeded || s == Skipped {
		return true
	}
	return s == Failed && dep.Failure == Continue
}

func (w *walker) anySatisfied(ids []string) bool {
	for _, d := range ids {
		s := w.status(d)
		if s == Succeeded || s == Skipped {
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

// execute runs n.Run against ctx once. A panic in Run is recovered and treated
// as a Failed attempt — defensive only; SDK/driver functions return errors
// rather than panicking (D10), so this catches author bugs, not API faults.
func (w *walker) execute(ctx context.Context, n *Node) (Status, int, error) {
	err := call(ctx, n)
	if err == nil {
		return Succeeded, 1, nil
	}
	if ctx.Err() != nil {
		return Canceled, 1, err
	}
	if n.Failure == AbortRun {
		w.cancel()
	}
	return Failed, 1, err
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
