package bench

import (
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/stroppy-io/stroppy/pkg/report"
)

// Step filtering turns the explicit --steps allowlist and --no-steps blocklist
// passed to Run into the per-step allow/deny decision used by Bench.Step.

type stepFilterState struct {
	only   map[string]struct{}
	except map[string]struct{}

	mu      sync.Mutex
	records []report.Step
}

func newStepFilter(steps, noSteps []string) *stepFilterState {
	s := &stepFilterState{only: map[string]struct{}{}, except: map[string]struct{}{}}

	for _, n := range steps {
		if n = strings.TrimSpace(n); n != "" {
			s.only[n] = struct{}{}
		}
	}

	for _, n := range noSteps {
		if n = strings.TrimSpace(n); n != "" {
			s.except[n] = struct{}{}
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

func (s *stepFilterState) record(step report.Step) {
	s.mu.Lock()
	s.records = append(s.records, step)
	s.mu.Unlock()
}

func (s *stepFilterState) snapshot() []report.Step {
	s.mu.Lock()
	defer s.mu.Unlock()

	return slices.Clone(s.records)
}

// Step runs fn as a named phase: skips if filtered out (logging the skip), otherwise
// tags metrics, notifies, logs start/end timing, clears the tag, and returns fn's
// error. Use it for one-shot setup/load/schema steps, which should each emit one
// start/end record.
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

func (b *Bench) step(name string, fn func() error, silent bool) (err error) {
	if !b.root.stepFilter.enabled(name) {
		b.root.stepFilter.record(report.Step{Name: name, Status: "skipped"})
		if !silent {
			b.lg.Sugar().Infof("Skipping step '%s'", name)
		}

		return nil
	}

	started := time.Now()
	stepBegin(b, name, silent)
	defer func() {
		stepEnd(b, name, silent)
		status := "completed"
		if err != nil {
			status = "failed"
		}
		b.root.stepFilter.record(report.Step{
			Name: name, Status: status, DurationSeconds: time.Since(started).Seconds(),
		})
	}()

	if fn != nil {
		return fn()
	}

	return nil
}

func stepBegin(b *Bench, name string, silent bool) {
	b.vu.stepTag = name
	if b.root != nil {
		b.root.NotifyStep(name, statusRunning)
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

	if b.root != nil {
		b.root.NotifyStep(name, statusCompleted)
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
