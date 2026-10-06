package bench

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/zap"

	"github.com/stroppy-io/stroppy/v6/pkg/report"
)

// Action is ordinary work independent of its execution policy.
type Action func(context.Context, *Bench) error

// StepStatus describes an immediate step outcome.
type StepStatus string

const (
	Observed            StepStatus = "observed"
	Completed           StepStatus = "completed"
	CompletedWithErrors StepStatus = "completed_with_errors"
	Skipped             StepStatus = "skipped"
	Blocked             StepStatus = "blocked"
	Failed              StepStatus = "failed"
	Canceled            StepStatus = "canceled"
)

// Result is a copied outcome; modifying it cannot change execution.
type Result struct {
	Status StepStatus
	Err    error
}

// StepDescription describes one encountered step, not every possible Go branch.
type StepDescription struct {
	Name     string `json:"name"`
	Executor string `json:"executor"`
	Workers  int    `json:"workers"`
	Measured bool   `json:"measured"`
}

// Execution observes or executes steps immediately in ordinary Go order.
type Execution struct {
	def          *Def
	root         *rootState
	ctx          context.Context //nolint:containedctx // execution replay owns action context lifetime.
	err          error
	observed     []StepDescription
	databases    map[string]*databaseSlot
	databaseMu   sync.Mutex
	measurements map[string]float64
	phase        string
	reporting    bool
	policies     map[string]report.Execution
	failurePhase string
	filter       *stepFilterState
}

func (e *Execution) Err() error { return e.err }

// Enabled checks selection without recording or executing a step.
func (e *Execution) Enabled(name string) bool {
	if e.root != nil {
		return e.root.stepFilter.enabled(name)
	}

	if e.filter != nil {
		return e.filter.enabled(name)
	}

	return true
}

//nolint:gocognit,gocyclo,cyclop,funlen // immediate step outcomes, filtering and measurement order stay explicit.
func (e *Execution) Step(name string, action Action, options ...StepOption) Result {
	if name == "" || action == nil {
		invalid("step", inputError("name and action are required"))
	}

	var opts stepOptions

	for _, option := range options {
		if option == nil {
			invalid("step", inputError("nil option"))
		}

		option.applyStep(&opts)
	}

	if opts.policy.mode == "" {
		opts.policy = Policy{mode: "once", workers: 1}
	}

	if opts.driver.owner != nil && opts.driver.owner != e.def {
		invalid("step driver", inputError("reference belongs to another definition"))
	}

	if opts.policy.workers < 1 {
		invalid("step", inputError("invalid policy"))
	}

	if opts.always && opts.policy.mode != "once" {
		invalid("step", inputError("cleanup must run once"))
	}

	e.observed = append(e.observed, StepDescription{
		name,
		opts.policy.mode,
		opts.policy.workers,
		opts.measured || opts.policy.mode != "once",
	})
	if e.root == nil {
		return Result{Status: Observed}
	}

	record := func(status StepStatus, elapsed time.Duration) {
		e.root.stepFilter.record(report.Step{
			Name:            name,
			Status:          string(status),
			DurationSeconds: elapsed.Seconds(),
		})
	}
	if !e.root.stepFilter.enabled(name) {
		record(Skipped, 0)

		return Result{Status: Skipped}
	}

	if !opts.always && (e.err != nil || e.ctx.Err() != nil) {
		if e.ctx.Err() != nil {
			e.err = errors.Join(e.err, e.ctx.Err())
		}

		record(Blocked, 0)

		return Result{Status: Blocked}
	}

	ctx := e.ctx

	cancel := func() {}
	if opts.always {
		ctx, cancel = context.WithTimeout(context.WithoutCancel(ctx), opts.cleanup)
	}
	defer cancel()

	started := time.Now()
	e.phase = name
	before := e.root.errorReporter.snapshot().terminalErrors

	measured := opts.measured || opts.policy.mode != "once"
	if measured {
		if err := e.root.startThroughput(name, opts.policy.workers); err != nil {
			e.err = errors.Join(e.err, err)

			return Result{Status: Failed, Err: err}
		}
		defer e.root.throughput.stop()
	}

	err := e.execute(ctx, name, action, &opts)
	if measured {
		e.root.throughput.stop()
	}

	elapsed := time.Since(started)

	if measured {
		window := e.root.throughput.windows[len(e.root.throughput.windows)-1]
		seconds := window.seconds()
		e.measurements[name] = seconds

		policyReport := report.Execution{
			Executor:           opts.policy.mode,
			Workers:            opts.policy.workers,
			Iterations:         opts.policy.iterations,
			DurationSeconds:    opts.policy.duration.Seconds(),
			DrainUnlimited:     opts.policy.drain.unlimited,
			MeasurementSeconds: seconds,
		}
		if opts.policy.mode == "constant-vus" {
			if !opts.policy.drain.unlimited {
				timeout := opts.policy.drain.duration.Seconds()
				policyReport.DrainTimeoutSeconds = &timeout
			}

			policyReport.DrainSeconds = max(0, seconds-opts.policy.duration.Seconds())
		}

		e.policies[name] = policyReport
	}

	status := Completed
	if err != nil {
		status = Failed
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			status = Canceled
		}

		if e.failurePhase == "" {
			e.failurePhase = name
		}

		e.err = errors.Join(e.err, fmt.Errorf("step %q: %w", name, err))
	} else if e.root.errorReporter.snapshot().terminalErrors > before {
		status = CompletedWithErrors
	}

	record(status, elapsed)

	return Result{status, err}
}

func (e *Execution) bench(vu *VU, name string) *Bench {
	cfg, err := e.configuration(name).runtime()
	if err != nil {
		invalid("driver "+name, err)
	}

	log := e.root.lg.Named("workload").With(zap.String("step", vu.stepTag), zap.Uint64("worker", vu.vuid-1))

	return &Bench{
		root:         e.root,
		vu:           vu,
		lg:           log,
		cfg:          cfg,
		execution:    e,
		databaseName: name,
		Log:          Logger{log},
	}
}

//nolint:gocognit,cyclop // worker joining, cancellation causes and budgets share one loop.
func (e *Execution) execute(parent context.Context, name string, action Action, opts *stepOptions) error {
	parent = context.WithValue(parent, metricStepKey{}, name)

	ctx, cancel := context.WithCancelCause(parent)
	defer cancel(nil)

	var remaining atomic.Int64
	remaining.Store(opts.policy.iterations)

	deadline := time.Time{}

	var drainTimer *time.Timer

	if opts.policy.mode == "constant-vus" {
		deadline = time.Now().Add(opts.policy.duration)
		if !opts.policy.drain.unlimited {
			drainTimer = time.AfterFunc(opts.policy.duration+opts.policy.drain.duration, func() { cancel(errDrainExpired) })
			defer drainTimer.Stop()
		}
	}

	var wg sync.WaitGroup

	fatal := make(chan error, opts.policy.workers)

	panics := make(chan any, opts.policy.workers)
	for worker := range opts.policy.workers {
		wg.Go(func() {
			defer func() {
				if value := recover(); value != nil {
					if validation, ok := value.(*ValidationError); ok {
						fatal <- validation

						cancel(validation)
					} else {
						panics <- value

						cancel(inputError("action panic"))
					}
				}
			}()
			//nolint:gosec // worker is nonnegative and bounded by validated policy.
			vu := &VU{
				root:      e.root,
				ctx:       metricWorkerContext(ctx, worker, opts.policy.workers),
				vuid:      uint64(worker) + 1,
				worker:    worker,
				stepTag:   name,
				initPhase: opts.policy.mode == "once",
			}
			vu.metricWriter, _ = vu.ctx.Value(metricWriterContextKey{}).(*metricWriter)
			vu.metricStepAttrs = e.root.txMetrics.stepAttributes(name)
			b := e.bench(vu, opts.driver.name)

			for {
				if ctx.Err() != nil {
					return
				}

				if opts.policy.mode == "shared-iterations" && remaining.Add(-1) < 0 {
					return
				}

				if opts.policy.mode == "constant-vus" && !time.Now().Before(deadline) {
					return
				}

				started := time.Now()

				err := action(vu.ctx, b)
				if err == nil && errors.Is(context.Cause(ctx), errDrainExpired) {
					err = errDrainExpired
				}

				vu.iterTest++

				vu.iterScenario++
				if opts.policy.mode != "once" {
					e.root.txMetrics.recordIteration(vu, time.Since(started))

					if window := e.root.throughput.current.Load(); window != nil {
						window.writer(worker).iterations.Add(1)
					}
				}

				if err != nil {
					if opts.policy.mode == "once" || IsFatalError(err) {
						fatal <- err

						cancel(err)

						return
					}

					if parent.Err() != nil {
						return
					}

					var classify func(error) ErrorFacts
					if b.drv != nil {
						classify = b.drv.ClassifyError
					}

					e.root.errorReporter.record(
						vu,
						terminalErrorIteration,
						name,
						err,
						classify,
					)

					if errors.Is(context.Cause(ctx), errDrainExpired) {
						return
					}
				}

				if opts.policy.mode == "once" {
					return
				}
			}
		})
	}

	wg.Wait()

	select {
	case value := <-panics:
		panic(value)
	default:
	}

	select {
	case err := <-fatal:
		return err
	default:
	}

	return parent.Err()
}

var errDrainExpired = errors.New("duration drain expired")
