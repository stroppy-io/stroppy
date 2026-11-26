package bench

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/stroppy-io/stroppy/v6/pkg/report"
)

type graphNodeRun struct {
	node                  *graphNode
	started               chan struct{}
	ready                 chan struct{}
	done                  chan struct{}
	actionDone            chan struct{}
	readyOnce             sync.Once
	readyMu               sync.Mutex
	readyWorkers          map[int]bool
	workers               int
	begun                 bool
	ownerDone             <-chan struct{}
	result                Result
	err                   error
	panicValue            any
	startedAt, finishedAt time.Time
	execution             *Execution
}

// Ready publishes a declared readiness milestone once initialization is usable.
func (b *Bench) Ready() {
	if b.execution.ready == nil {
		invalid("graph readiness", inputError("node does not declare a readiness milestone"))
	}

	b.execution.ready(b.Worker())
}

func (g *Graph) run(selected map[*graphNode]bool) error {
	e := g.execution
	started := time.Now()

	ctx, cancel := context.WithCancelCause(e.ctx)
	defer cancel(nil)

	runs := g.newRuns(selected)

	names := make([]string, 0, len(g.nodes))
	for _, node := range g.nodes {
		names = append(names, node.name)
	}

	e.root.stepFilter.declare(names)

	if e.root.capture != nil {
		e.root.capture.writer.Event("graph_start", g.name, "running", 0, 0, 0)
	}

	defer func() {
		if e.root.capture != nil {
			e.root.capture.writer.Event("graph_end", g.name, "joined", 0, time.Since(started), 0)
		}
	}()

	sharedPoolScope := g.hasConcurrentNodes(selected)
	if sharedPoolScope {
		e.startPoolTelemetry(g.name)
		defer e.root.pools.stop()
	}

	var workers sync.WaitGroup
	for _, node := range g.nodes {
		workers.Go(func() { g.runNode(ctx, cancel, runs[node], runs, selected[node], sharedPoolScope) })
	}

	workers.Wait()

	panicValue, failures := g.collectRuns(runs, context.Cause(ctx))
	g.collectMeasurements(runs)

	for _, session := range g.sessions {
		failures = errors.Join(failures, session.close(e.ctx))
	}

	e.graphReports = append(e.graphReports, g.report(runs, started))
	e.err = errors.Join(e.err, failures, e.ctx.Err())

	if panicValue != nil {
		panic(panicValue)
	}

	return e.err
}

func (g *Graph) newRuns(selected map[*graphNode]bool) map[*graphNode]*graphNodeRun {
	runs := make(map[*graphNode]*graphNodeRun, len(g.nodes))
	for _, node := range g.nodes {
		options, _ := graphStepOptions(g.execution, node)

		runs[node] = &graphNodeRun{
			node: node, started: make(chan struct{}), ready: make(chan struct{}), done: make(chan struct{}),
			actionDone: make(chan struct{}), workers: options.policy.workers, readyWorkers: make(map[int]bool),
		}
		if selected[node] && node.session != nil && g.execution.Enabled(node.name) {
			node.session.remaining++
		}
	}

	return runs
}

func (g *Graph) runNode(
	ctx context.Context, cancel context.CancelCauseFunc, run *graphNodeRun,
	runs map[*graphNode]*graphNodeRun, selected, sharedPoolScope bool,
) {
	node := run.node
	defer close(run.done)
	defer joinGraphAuxiliaries(node, runs)

	if selected && node.session != nil && g.execution.Enabled(node.name) {
		defer func() {
			if err := node.session.release(g.execution.ctx); err != nil {
				run.err = errors.Join(run.err, err)
				run.result = Result{Status: Failed, Err: run.err}
				g.recordNodeFailure(run)
			}
		}()
	}

	defer close(run.actionDone)
	defer func() {
		if value := recover(); value != nil {
			run.finishedAt = time.Now()
			run.panicValue, run.err = value, errWorkloadPanic
			run.result = Result{Status: Failed, Err: errWorkloadPanic}
			g.recordNodeFailure(run)
			cancel(errWorkloadPanic)
		}
	}()

	if !selected || ((node.cleanup == 0 || node.optionalCleanup) && !g.execution.Enabled(node.name)) {
		run.result.Status = skippedGraphStatus(node, runs)

		return
	}

	if !waitGraphDependencies(ctx, run, runs, false) {
		run.result.Status = Blocked

		return
	}

	if node.cleanup > 0 && !node.optionalCleanup && !graphOwnerStarted(node, runs) {
		run.result.Status = Skipped

		return
	}

	if node.join {
		run.startedAt, run.finishedAt = time.Now(), time.Now()
		run.result.Status = Completed
		run.publishReady()
		close(run.started)
		g.execution.root.stepFilter.record(report.Step{Name: node.name, Status: string(Completed)})

		return
	}

	if !sharedPoolScope {
		g.execution.startPoolTelemetry(node.name)
		defer g.execution.root.pools.stop()
	}

	g.executeNode(ctx, run, runs)

	if run.err != nil && node.cleanup == 0 && IsFatalError(run.err) {
		cancel(run.err)
	}
}

func joinGraphAuxiliaries(node *graphNode, runs map[*graphNode]*graphNodeRun) {
	for _, auxiliary := range runs {
		if auxiliary.node.auxiliary == node {
			<-auxiliary.done
		}
	}
}

func skippedGraphStatus(node *graphNode, runs map[*graphNode]*graphNodeRun) StepStatus {
	status := Skipped

	for _, dependency := range node.dependencies {
		if dependency.event != "started" && dependency.event != "ready" {
			prior := runs[dependency.node]
			<-prior.done

			if !graphDependencySatisfied(dependency.event, prior.result.Status) {
				status = Blocked
			}
		}
	}

	return status
}

func (g *Graph) recordNodeFailure(run *graphNodeRun) {
	root := g.execution.root
	root.stepFilter.outcome(run.node.name, Failed)

	if run.execution != nil && run.execution.progress != nil {
		run.execution.progress.end(Failed)
	}

	if root.capture != nil {
		root.capture.writer.Event("node_end", run.node.name, string(Failed), run.workers, 0, 0)
	}
}

func (g *Graph) executeNode(ctx context.Context, run *graphNodeRun, runs map[*graphNode]*graphNodeRun) {
	node := run.node

	cleanupCancel := func() {}
	if node.cleanup > 0 {
		ctx, cleanupCancel = context.WithTimeout(context.WithoutCancel(g.execution.ctx), node.cleanup)
	}
	defer cleanupCancel()

	nodeContext, stop := context.WithCancel(ctx)
	defer stop()

	joined := watchAuxiliary(nodeContext, stop, run, runs)
	defer func() {
		stop()

		if joined != nil {
			<-joined
		}
	}()

	child := g.nodeExecution(nodeContext, run, runs)
	if g.execution.root.progress != nil {
		child.progress = g.execution.root.progress.graphChild()
	}

	run.execution = child

	options := append([]StepOption(nil), node.options...)
	if node.session != nil {
		options = append(options, Use(node.session.driver), RetainSession())
	}

	run.result = child.Step(node.name, node.action, options...)
	if node.after != nil {
		node.after(run.result)
	}

	run.err = run.result.Err

	run.finishedAt = time.Now()

	if child.progress != nil {
		child.progress.mu.Lock()

		child.progress.stopped = run.finishedAt
		if run.err == nil {
			child.progress.ratio = 1
		}
		child.progress.mu.Unlock()
	}
}

func watchAuxiliary(
	ctx context.Context, stop context.CancelFunc, run *graphNodeRun, runs map[*graphNode]*graphNodeRun,
) <-chan struct{} {
	if run.node.auxiliary == nil {
		return nil
	}

	run.ownerDone = runs[run.node.auxiliary].actionDone

	joined := make(chan struct{})
	go func() {
		defer close(joined)

		select {
		case <-run.ownerDone:
			stop()
		case <-ctx.Done():
		}
	}()

	return joined
}

func (g *Graph) collectRuns(runs map[*graphNode]*graphNodeRun, cause error) (panicValue any, failures error) {
	e := g.execution

	for _, node := range g.nodes {
		run := runs[node]
		if child := run.execution; child != nil {
			for name, seconds := range child.measurements {
				e.measurements[name] = seconds
			}

			for name, policy := range child.policies {
				e.policies[name] = policy
			}
		}

		if run.startedAt.IsZero() {
			e.root.stepFilter.record(report.Step{Name: node.name, Status: string(run.result.Status)})
		}

		failure := graphRunFailure(e.ctx, cause, run.err)
		if failure != nil {
			failures = errors.Join(failures, fmt.Errorf("node %s: %w", node.name, failure))
			if e.failurePhase == "" {
				e.failurePhase = node.name
			}
		}

		if panicValue == nil {
			panicValue = run.panicValue
		}
	}

	return panicValue, failures
}

func (g *Graph) collectMeasurements(runs map[*graphNode]*graphNodeRun) {
	for _, window := range g.measurements {
		first, last, complete := graphMeasurementBounds(window, runs)
		if complete {
			g.execution.measurements[window.name] = last.Sub(first).Seconds()
		}
	}
}

func graphMeasurementBounds(
	window graphMeasurement, runs map[*graphNode]*graphNodeRun,
) (first, last time.Time, complete bool) {
	for _, node := range window.nodes {
		run := runs[node]
		if run.startedAt.IsZero() || run.finishedAt.IsZero() || run.result.Status != Completed {
			return first, last, false
		}

		if first.IsZero() || run.startedAt.Before(first) {
			first = run.startedAt
		}

		if run.finishedAt.After(last) {
			last = run.finishedAt
		}
	}

	return first, last, true
}

func graphOwnerStarted(node *graphNode, runs map[*graphNode]*graphNodeRun) bool {
	for _, dependency := range node.dependencies {
		if runs[dependency.node].begun {
			return true
		}
	}

	return false
}

func waitGraphDependencies(
	ctx context.Context, run *graphNodeRun, runs map[*graphNode]*graphNodeRun, beforeAction bool,
) bool {
	for _, dependency := range run.node.dependencies {
		if dependency.beforeAction != beforeAction {
			continue
		}

		prior := runs[dependency.node]
		if run.node.cleanup > 0 {
			<-prior.done

			continue
		}

		if !waitGraphEvent(ctx, dependency.event, prior) {
			return false
		}

		if dependency.event != "started" && dependency.event != "ready" &&
			!graphDependencySatisfied(dependency.event, prior.result.Status) {
			return false
		}
	}

	return run.node.cleanup > 0 || ctx.Err() == nil
}

func waitGraphEvent(ctx context.Context, kind string, prior *graphNodeRun) bool {
	event := prior.done

	switch kind {
	case "started":
		event = prior.started
	case "ready":
		event = prior.ready
	}

	select {
	case <-event:
		return true
	case <-prior.done:
		select {
		case <-event:
			return true
		default:
			return false
		}
	case <-ctx.Done():
		return false
	}
}

func graphDependencySatisfied(event string, status StepStatus) bool {
	switch event {
	case "succeeded":
		return status == Completed
	case "ordered":
		return status == Completed || status == CompletedWithErrors || status == Skipped
	default:
		return true
	}
}

func (g *Graph) nodeExecution(
	ctx context.Context, run *graphNodeRun, runs map[*graphNode]*graphNodeRun,
) *Execution {
	parent := g.execution
	child := &Execution{
		def: parent.def, root: parent.root, ctx: ctx, databases: parent.databases,
		measurements: make(map[string]float64), policies: make(map[string]report.Execution), reporting: parent.reporting,
		databaseOwner: parent, graphRun: true, graphCleanup: run.node.cleanup > 0, session: run.node.session,
	}
	child.onStart = func() {
		if run.node.before != nil {
			run.node.before()
		}

		run.startedAt = time.Now()
		close(run.started)
	}

	child.onEnter = func() { run.begun = true }
	if !run.node.ready {
		child.onInitialized = run.publishReady
	} else {
		child.ready = run.signalReady
	}

	child.beforeAction = func(ctx context.Context) error {
		if waitGraphDependencies(ctx, run, runs, true) {
			return nil
		}

		if ctx.Err() != nil {
			return ctx.Err()
		}

		return inputError("node %q action prerequisite did not complete", run.node.name)
	}
	child.onResult = func(result Result) Result { return run.normalizeResult(parent.ctx, result) }

	return child
}

func (run *graphNodeRun) publishReady() {
	run.readyOnce.Do(func() {
		close(run.ready)

		if run.execution != nil && run.execution.root.capture != nil {
			run.execution.root.capture.writer.Event("node_ready", run.node.name, "ready", run.workers, 0, 0)
		}
	})
}

func (run *graphNodeRun) signalReady(worker int) {
	run.readyMu.Lock()
	defer run.readyMu.Unlock()

	run.readyWorkers[worker] = true
	if len(run.readyWorkers) == run.workers {
		run.publishReady()
	}
}

func (run *graphNodeRun) normalizeResult(parent context.Context, result Result) Result {
	if run.node.auxiliary != nil && errors.Is(result.Err, context.Canceled) && parent.Err() == nil {
		select {
		case <-run.ownerDone:
			if remaining := withoutGraphCancellation(result.Err); remaining == nil {
				result = Result{Status: Completed}
			} else {
				result = Result{Status: Failed, Err: remaining}
			}
		default:
		}
	}

	if run.node.ready && result.Err == nil {
		select {
		case <-run.ready:
		default:
			return Result{Status: Failed, Err: inputError("node %q completed without readiness", run.node.name)}
		}
	}

	return result
}
