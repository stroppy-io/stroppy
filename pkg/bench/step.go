package bench

import (
	"fmt"
	"os"
	"strings"
	"time"
)

// Step filtering (STROPPY_STEPS allowlist / STROPPY_NO_STEPS blocklist), parsed at
// Run start (see newRootState) once STROPPY_STEPS env is in place.

type stepFilterState struct {
	only   map[string]struct{}
	except map[string]struct{}
}

func newStepFilter() *stepFilterState {
	s := &stepFilterState{only: map[string]struct{}{}, except: map[string]struct{}{}}

	if v := os.Getenv("STROPPY_STEPS"); v != "" {
		for _, n := range strings.Split(v, ",") {
			if n = strings.TrimSpace(n); n != "" {
				s.only[n] = struct{}{}
			}
		}
	}

	if v := os.Getenv("STROPPY_NO_STEPS"); v != "" {
		for _, n := range strings.Split(v, ",") {
			if n = strings.TrimSpace(n); n != "" {
				s.except[n] = struct{}{}
			}
		}
	}

	return s
}

func (s *stepFilterState) enabled(name string) bool {
	if _, ok := s.except[name]; ok {
		return false
	}

	if len(s.only) > 0 {
		if _, ok := s.only[name]; !ok {
			return false
		}
	}

	return true
}

// Step runs fn as a named phase: skips if filtered out (logging the skip), otherwise
// tags metrics, notifies, logs start/end timing, clears the tag, and returns fn's
// error. Use it for one-shot setup/load/schema steps, which should each emit one
// start/end record. Mirrors helpers.ts Step.
func (b *Bench) Step(name string, fn func() error) error {
	return b.step(name, fn, false)
}

// StepSilent runs fn under the named step tag with the same filtering semantics as
// Step, but emits no console records. Use it for a step that wraps every iteration
// (the "workload" step): per-VU query/transaction metrics keep the step tag while
// the iterations stay quiet.
func (b *Bench) StepSilent(name string, fn func() error) error {
	return b.step(name, fn, true)
}

func (b *Bench) step(name string, fn func() error, silent bool) error {
	if !b.root.stepFilter.enabled(name) {
		if !silent {
			b.lg.Sugar().Infof("Skipping step '%s'", name)
		}

		return nil
	}

	stepBegin(b, name, silent)
	defer stepEnd(b, name, silent)

	if fn != nil {
		return fn()
	}

	return nil
}

func stepBegin(b *Bench, name string, silent bool) {
	b.vu.stepTag = name
	if root != nil {
		root.NotifyStep(name, statusRunning)
	}

	if silent {
		return
	}

	b.lg.Sugar().Infof("Start of '%s' step", name)
	b.stepStart = time.Now()
}

func stepEnd(b *Bench, name string, silent bool) {
	if !silent && !b.stepStart.IsZero() {
		b.lg.Sugar().Infof("End of '%s' step (took %s)", name, fmtStepDuration(time.Since(b.stepStart)))
	}

	b.vu.stepTag = ""

	if root != nil {
		root.NotifyStep(name, statusCompleted)
	}
}

const (
	statusRunning   int32 = 1
	statusCompleted int32 = 2

	secondsPerMinute = 60
)

func fmtStepDuration(d time.Duration) string {
	switch {
	case d < time.Millisecond:
		return "0ms"
	case d < time.Minute:
		// Truncate to millisecond precision for display; Truncate avoids the
		// mul-after-div precision loss that durationcheck flags.
		return d.Truncate(time.Millisecond).String()
	default:
		m := int(d.Minutes())
		s := int(d.Seconds()) - m*secondsPerMinute

		return fmt.Sprintf("%dm%02ds", m, s)
	}
}
