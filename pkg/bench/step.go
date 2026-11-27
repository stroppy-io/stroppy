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
	for selector := range s.except {
		if name == selector || strings.HasPrefix(name, selector+"/") {
			return false
		}
	}

	if len(s.only) > 0 {
		for selector := range s.only {
			if name == selector || strings.HasPrefix(name, selector+"/") {
				return true
			}
		}

		return false
	}

	return true
}

func (s *stepFilterState) declare(names []string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, name := range names {
		if s.records[name] == nil {
			s.records[name] = &report.Step{Name: name, Status: string(Skipped)}
			s.order = append(s.order, name)
		}
	}
}

func (s *stepFilterState) outcome(name string, status StepStatus) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if step := s.records[name]; step != nil {
		step.Status = string(status)
	}
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
	} else if recorded.Executions == 0 {
		recorded.Status = step.Status
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
