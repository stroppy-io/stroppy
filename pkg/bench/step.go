package bench

import (
	"strings"
	"sync"

	"github.com/stroppy-io/stroppy/v6/pkg/report"
)

// Step filtering turns the explicit --steps allowlist and --no-steps blocklist
// passed to Run into the per-step allow/deny decision used by Bench.Step.

type stepFilterState struct {
	only   map[string]struct{}
	except map[string]struct{}

	mu      sync.Mutex
	records map[string]*report.Step
	order   []string
}

func newStepFilter(steps, noSteps []string) *stepFilterState {
	s := &stepFilterState{
		only: map[string]struct{}{}, except: map[string]struct{}{}, records: map[string]*report.Step{},
	}

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
	defer s.mu.Unlock()

	recorded := s.records[step.Name]
	if recorded == nil {
		recorded = &report.Step{Name: step.Name, Status: step.Status}
		s.records[step.Name] = recorded
		s.order = append(s.order, step.Name)
	}

	if step.Status != "skipped" && step.Status != "blocked" {
		recorded.Status = step.Status
		recorded.Executions++
		recorded.DurationSeconds += step.DurationSeconds
	}
}

func (s *stepFilterState) snapshot() []report.Step {
	s.mu.Lock()
	defer s.mu.Unlock()

	steps := make([]report.Step, 0, len(s.order))
	for _, name := range s.order {
		steps = append(steps, *s.records[name])
	}

	return steps
}
