package bench

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestRunEndToEndNoop(t *testing.T) {
	tst := twoStepTest(50 * time.Millisecond)
	var out strings.Builder
	code := runMain(tst, nil, envMap(nil), &out, &out)
	if code != 0 {
		t.Fatalf("runMain exit=%d, want 0\n%s", code, out.String())
	}
	s := out.String()
	// One shared registry + one summary spanning both steps.
	if !strings.Contains(s, "=== summary") {
		t.Fatalf("no summary emitted:\n%s", s)
	}
	for _, want := range []string{"setup/servicetime", "work/servicetime", "work/iterations"} {
		if !strings.Contains(s, want) {
			t.Fatalf("summary missing %q (registry not shared across steps):\n%s", want, s)
		}
	}
	if !strings.Contains(s, "=== steps (Succeeded) ===") {
		t.Fatalf("run did not succeed:\n%s", s)
	}
}

func TestRunSkipSkippable(t *testing.T) {
	tst := skipStepTest()
	var out strings.Builder
	// -skip=extra skips the Skippable step; setup still runs, extra is Skipped.
	// F3: a downstream step gated on extra via After still runs.
	code := runMain(tst, []string{"-skip", "extra"}, envMap(nil), &out, &out)
	if code != 0 {
		t.Fatalf("exit=%d, want 0\n%s", code, out.String())
	}
	s := out.String()
	if !strings.Contains(s, "extra") || !strings.Contains(s, "Skipped") {
		t.Fatalf("extra should be Skipped under -skip:\n%s", s)
	}
	if !strings.Contains(s, "after") || !strings.Contains(s, "Succeeded") {
		t.Fatalf("after-extra should still run (F3 unblock) and succeed:\n%s", s)
	}
}

func TestRunSkipNonSkippableHardErrors(t *testing.T) {
	tst := skipStepTest()
	var out strings.Builder
	// -skip on a required (non-Skippable) step is a hard error before run.
	code := runMain(tst, []string{"-skip", "setup"}, envMap(nil), &out, &out)
	if code == 0 {
		t.Fatalf("exit=0, want non-zero for -skip on a non-skippable step\n%s", out.String())
	}
	if !strings.Contains(out.String(), "not skippable") {
		t.Fatalf("error should mention the non-skippable guardrail:\n%s", out.String())
	}
}

func TestRunSkipUnknownStepHardErrors(t *testing.T) {
	tst := skipStepTest()
	var out strings.Builder
	// -skip on an undeclared name is a hard error (typo guard).
	code := runMain(tst, []string{"-skip", "ghost"}, envMap(nil), &out, &out)
	if code == 0 {
		t.Fatalf("exit=0, want non-zero for -skip on an unknown step\n%s", out.String())
	}
	if !strings.Contains(out.String(), "not declared") {
		t.Fatalf("error should mention the undeclared step:\n%s", out.String())
	}
}

func TestDefineErrorExits(t *testing.T) {
	// A Define that returns a non-nil error is a configuration failure: runMain
	// surfaces it and exits non-zero before any run starts (replaces the old
	// Opts.Validate hook, D7).
	tst := &Test{
		Name: "bad",
		Define: func(d *Def) error {
			d.Driver("main", "noop")
			d.Step("a", okHandler{})
			return fmt.Errorf("boom")
		},
	}
	var out strings.Builder
	if code := runMain(tst, nil, envMap(nil), &out, &out); code == 0 {
		t.Fatalf("expected non-zero exit on Define error\n%s", out.String())
	}
}

// twoStepTest is a setup(Once) -> work(Closed) test on the noop driver, built
// via Define.
func twoStepTest(dur time.Duration) *Test {
	return &Test{
		Name: "twostep",
		Define: func(d *Def) error {
			d.Driver("main", "noop")
			d.Step("setup", FuncOnce(func(vu *VU) error {
				_, err := vu.Conn() // pin a (noop) connection
				return err
			}))
			d.Step("work", &closedNoopDB{}).Closed(2, dur).After("setup")
			d.Variant("full")
			return nil
		},
	}
}

// skipStepTest is the F4/-skip fixture: setup -> extra(Skippable) -> after,
// all on the noop driver. "extra" is author-marked Skippable (so -skip targets
// it); "setup" is required (so -skip on it is a hard error); "after" gates on
// extra via After, so under -skip=extra the F3 unblock is observable (after
// still runs).
func skipStepTest() *Test {
	return &Test{
		Name: "skiptest",
		Define: func(d *Def) error {
			d.Driver("main", "noop")
			d.Step("setup", FuncOnce(func(vu *VU) error {
				_, err := vu.Conn()
				return err
			}))
			d.Step("extra", &closedNoopDB{}).Closed(1, 5*time.Millisecond).After("setup").Skippable()
			d.Step("after", FuncOnce(func(vu *VU) error {
				_, err := vu.Conn()
				return err
			})).After("extra")
			d.Variant("full")
			return nil
		},
	}
}

// closedNoopDB pins a connection in Init and does a trivial per-Iter driver call.
type closedNoopDB struct{}

func (closedNoopDB) Init(vu *VU) error {
	_, err := vu.Conn()
	return err
}
func (closedNoopDB) Iter(vu *VU) error {
	// Cache hit on the hot path: allowed. A no-op exec against the pinned conn.
	_, err := vu.Conn()
	return err
}
func (closedNoopDB) Close(*VU) error { return nil }
