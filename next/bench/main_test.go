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

func TestRunStepsFilterPrunes(t *testing.T) {
	tst := twoStepTest(20 * time.Millisecond)
	var out strings.Builder
	// -no-steps=work prunes the workload; setup still runs, work is Skipped.
	code := runMain(tst, []string{"-no-steps", "work"}, envMap(nil), &out, &out)
	if code != 0 {
		t.Fatalf("exit=%d, want 0\n%s", code, out.String())
	}
	s := out.String()
	if !strings.Contains(s, "work") || !strings.Contains(s, "Skipped") {
		t.Fatalf("work should be Skipped under -no-steps:\n%s", s)
	}
}

func TestStepsNoStepsMutuallyExclusive(t *testing.T) {
	tst := twoStepTest(time.Millisecond)
	var out strings.Builder
	code := runMain(tst, []string{"-steps", "a", "-no-steps", "b"}, envMap(nil), &out, &out)
	if code != 2 {
		t.Fatalf("exit=%d, want 2 for -steps + -no-steps together\n%s", code, out.String())
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
