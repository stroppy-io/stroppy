package dag

import (
	"context"
	"time"
)

// Status is a node's terminal outcome. The zero value is never observed
// on a terminal [NodeResult]: nodes that never ran are absent from a
// [RunResult], not present with a zero Status.
type Status int

const (
	// Succeeded means Run returned nil on some attempt.
	Succeeded Status = iota
	// Failed means every attempt of Run returned a non-retryable (or
	// exhausted) error, including a recovered panic.
	Failed
	// Skipped means the node never ran: its edge conditions were not
	// met, or its If predicate returned false.
	Skipped
	// Canceled means the run's context was done before or during the
	// node's execution.
	Canceled
)

// String renders the Status name.
func (s Status) String() string {
	switch s {
	case Succeeded:
		return "Succeeded"
	case Failed:
		return "Failed"
	case Skipped:
		return "Skipped"
	case Canceled:
		return "Canceled"
	default:
		return "Unknown"
	}
}

// FailurePolicy controls how a node's Failed status is treated by the
// walker: how it propagates to the rest of the run and to this node's
// After-edge dependents.
type FailurePolicy int

const (
	// AbortRun cancels the run's context so that every node not yet
	// terminal ends up Canceled. OnFailure-gated cleanup nodes are
	// exempt: they run against the caller's context, so the abort does
	// not cancel them (external cancellation still does). Default.
	AbortRun FailurePolicy = iota
	// SkipDependents leaves the rest of the graph running; only this
	// node's After-edge dependent closure resolves to Skipped, via the
	// normal edge-gate evaluation (a Failed dependency never satisfies
	// After).
	SkipDependents
	// Continue records the failure but satisfies this node's direct
	// After-edge dependents as if it had succeeded, so they still run.
	Continue
)

// String renders the FailurePolicy name.
func (p FailurePolicy) String() string {
	switch p {
	case AbortRun:
		return "AbortRun"
	case SkipDependents:
		return "SkipDependents"
	case Continue:
		return "Continue"
	default:
		return "Unknown"
	}
}

// Node is one unit of work in the graph. The executor policy (once,
// worker pool, rate-paced loop, ...) lives inside Run; the walker only
// calls it and tracks its terminal status.
//
// Dependency edges are declared as three independent ID lists:
//
//   - After: this node runs only once every listed dependency has
//     Succeeded (a Failed dependency under FailurePolicy Continue also
//     satisfies After).
//   - AfterAny: this node runs once every listed dependency has reached
//     a terminal state and at least one Succeeded.
//   - OnFailure: this node runs once every listed dependency has reached
//     a terminal state and at least one Failed — a cleanup path. Such
//     nodes survive an AbortRun abort: they execute against the caller's
//     context rather than the canceled run context, so cleanup still
//     runs under the default failure policy. If no listed dependency
//     Failed, the node resolves Skipped.
//
// A node may combine all three; it runs only if every present kind is
// satisfied (e.g. OnFailure + After gates a cleanup on both a failure
// and an unrelated successful dependency). A node with no edges is a
// root and runs immediately.
type Node struct {
	// ID uniquely identifies the node within a Graph.
	ID string
	// Run is the opaque unit of work. Recovered panics are captured as
	// a Failed status with the panic value in the error.
	Run func(ctx context.Context) error
	// If is evaluated once the node's edge conditions are satisfied; a
	// false result skips the node (and, transitively, its After-edge
	// dependents).
	If func() bool

	After     []string
	AfterAny  []string
	OnFailure []string

	Failure FailurePolicy
}

// NodeResult is a node's terminal outcome and execution accounting.
type NodeResult struct {
	ID       string
	Status   Status
	Start    time.Time
	End      time.Time
	Attempts int
	Err      error
}

// RunResult is the outcome of one [Run] call.
type RunResult struct {
	// Nodes maps node ID to its terminal result. Every node reachable
	// in the graph has an entry.
	Nodes map[string]*NodeResult
	// Status summarizes the run: Failed if any node Failed, else
	// Canceled if any node Canceled, else Succeeded.
	Status Status
}

// Node looks up a node's result by ID, returning nil if absent.
func (r *RunResult) Node(id string) *NodeResult {
	return r.Nodes[id]
}
