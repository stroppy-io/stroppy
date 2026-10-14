package banking

import (
	"context"
	"errors"
	"fmt"

	"github.com/stroppy-io/stroppy/v6/pkg/bench"
)

// neverArmed marks a worker that has not armed the injector yet.
const neverArmed = ^uint64(0)

// defaultInjectEvery arms the injector on every Nth transfer unless told otherwise.
const defaultInjectEvery = 25

// The injecting variant fails transactions on purpose, so the invariants are
// checked against interruptions rather than only against concurrency. It needs a
// fault injector in the server: Macavity (https://github.com/CrystallineCore/Macavity)
// hooks the transaction callbacks through supported APIs and injects at
// before_commit — the moment a banking test cares about, because the client cannot
// tell from the error alone whether the commit happened.
//
// PostgreSQL's own injection points cannot do this: they are compiled out of
// stock builds, and (measured on 18.6) there is no injection point in the commit
// path at all — the only dependable core point is mid-UPDATE.
const variantInject = "inject"

// errInjectDriverUnsupported rejects the inject variant on a driver where the
// Macavity extension cannot exist. Recording and noop are allowed so the variant
// stays testable and describable without a server.
var (
	errUnknownInjectAction     = errors.New("banking/inject: unknown --inject-action")
	errNegativeInjectEvery     = errors.New("banking/inject: --inject-every must not be negative")
	errInjectDriverUnsupported = errors.New(
		"banking/inject: needs the postgres driver with the Macavity extension available")
)

// Inject is the fault-injecting variant: postgres only, and it requires the
// Macavity extension to be installable on the server.
var Inject = bench.Test{
	Name:   "banking/inject",
	Define: defineInject,
	Source: publication("Inject"),
}

func init() { bench.Register(Inject) }

func defineInject(d *bench.Def) error { return defineVariant(d, variantInject) }

type injectConfig struct {
	arm        bench.QueryHandle
	point      string
	action     string
	occurrence int64
	every      int64
}

// configureVariant is the seam between the two variants: the plain one does
// nothing here, the injecting one adds its parameters, its setup step and the
// handle it fires.
func (w *workload) configureVariant(d *bench.Def, variant string) error {
	if variant != variantInject {
		return nil
	}

	point, _ := d.Param.String("inject-point", "before_commit",
		"Fault point to inject at (Macavity point name).")
	action, _ := d.Param.String("inject-action", "error",
		"Injected action: error fails the commit, crash kills the backend.")
	occurrence, _ := d.Param.Int64("inject-occurrence", 1,
		"Fire the armed event on this matching hit.")
	every, _ := d.Param.Int64("inject-every", defaultInjectEvery,
		"Arm the injector every Nth transfer; 0 disables injection.")

	switch action {
	case "error", "crash":
	default:
		return fmt.Errorf("%w %q, want error or crash", errUnknownInjectAction, action)
	}

	switch w.driverType {
	case bench.DriverPostgres, bench.DriverNoop, bench.DriverRecording:
	default:
		return fmt.Errorf("%w: driver is %s", errInjectDriverUnsupported, w.driverType)
	}

	if every < 0 {
		return fmt.Errorf("%w, got %d", errNegativeInjectEvery, every)
	}

	w.inject = &injectConfig{
		arm:        w.sql.Require("injection", "arm"),
		point:      point,
		action:     action,
		occurrence: occurrence,
		every:      every,
	}

	// Arming inside the transfer's own transaction is deliberate: with Macavity,
	// arming in autocommit skips the arming statement's own commit, but arming
	// inside an explicit transaction does not — that transaction's COMMIT is the
	// first hit. So occurrence 1 fails exactly the transfer that armed it, which is
	// the commit this workload wants to interrupt.
	//
	// The extension is created in setup rather than opportunistically, and it is
	// not PUBLIC: connecting as a role that may not execute macavity_arm fails
	// there with a permission error rather than mid-run.
	d.Execution.Step("setup_injection", sqlSection{sql: w.sql, name: "setup_injection"}.Run)

	return nil
}

// maybeInject arms one injected fault inside the transfer's transaction shortly
// before it commits — once per iteration, never once per attempt.
//
// The transfer itself is untouched: what the run then observes is the client's
// reaction to a failed commit, and whether the invariants survive it. Because the
// armed event is one-shot, the next attempt of the *same* iteration runs without a
// fresh one and commits, which is how a retry absorbs the injected failure at all:
// measured against a real server, re-arming per attempt failed every attempt and no
// retry could ever recover.
func (w *workload) maybeInject(ctx context.Context, tx *bench.Tx, b *bench.Bench) error {
	if !w.shouldArm(b.Worker(), b.Iteration()) {
		return nil
	}

	err := tx.Exec(ctx, w.inject.arm.Text, map[string]any{
		"point":      w.inject.point,
		"action":     w.inject.action,
		"occurrence": w.inject.occurrence,
	})
	if err != nil {
		return fmt.Errorf("arm %s/%s: %w", w.inject.point, w.inject.action, err)
	}

	return nil
}

// shouldArm decides whether this attempt arms the injector, and remembers when it
// does. It arms at most once per iteration per worker, which is the property the
// retry path depends on: the armed event is one-shot, so the second attempt of an
// iteration that already armed one must run *without* a fresh fault and commit.
//
// Measured against a real server with arms recorded per attempt: re-arming inside
// the retry loop gave 2.76 arms per armed iteration and failed every attempt, so no
// retry could ever absorb the fault.
func (w *workload) shouldArm(worker int, iteration uint64) bool {
	if w.inject == nil || w.inject.every == 0 {
		return false
	}

	index := worker % len(w.injectArmedFor)

	if w.injectArmedFor[index] == iteration {
		return false
	}

	if iteration%uint64(w.inject.every) != 0 { //nolint:gosec // modulo against a positive int64
		return false
	}

	w.injectArmedFor[index] = iteration

	return true
}
