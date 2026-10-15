package banking

import (
	"context"
	"errors"
	"fmt"

	"github.com/stroppy-io/stroppy/v6/pkg/bench"
)

const neverArmed = ^uint64(0)

const defaultInjectEvery = 25

// The injecting variant fails transactions on purpose, so the invariants are checked
// against interruptions rather than only concurrency. It needs a fault injector in the
// server: Macavity (https://github.com/stroppy-io/Macavity) injects at before_commit —
// the moment that matters here, because the client cannot tell from the error whether
// the commit happened.
//
// PostgreSQL's own injection points cannot: they are compiled out of stock builds, and
// there is no point in the commit path at all.
const variantInject = "inject"

// errInjectDriverUnsupported rejects the variant where Macavity cannot exist. Recording
// and noop are allowed so it stays testable without a server.
var (
	errUnknownInjectAction     = errors.New("banking/inject: unknown --inject-action")
	errNegativeInjectEvery     = errors.New("banking/inject: --inject-every must not be negative")
	errInjectDriverUnsupported = errors.New(
		"banking/inject: needs the postgres driver with the Macavity extension available")
)

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

// configureVariant is the seam between the variants: the plain one does nothing here.
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

	// Arming inside the transfer's own transaction is deliberate: in autocommit the arming
	// statement's own commit is skipped, but inside an explicit transaction it is the first
	// hit — so occurrence 1 fails exactly the transfer that armed it.
	d.Execution.Step("setup_injection", sqlSection{sql: w.sql, name: "setup_injection"}.Run)

	return nil
}

// maybeInject arms one fault inside the transfer's transaction, once per iteration and
// never once per attempt: the event is one-shot, so the next attempt runs without one
// and commits. That is how a retry absorbs an injected failure at all.
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

// shouldArm decides whether this attempt arms the injector, at most once per iteration
// per worker — the property the retry path depends on.
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
