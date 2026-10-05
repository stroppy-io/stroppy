package bench

import (
	"time"
)

// Drain defines the grace period after a duration budget. Use named presets or
// DrainTimeout; an unspecified zero value is invalid.
type Drain struct {
	duration  time.Duration
	unlimited bool
	set       bool
}

var (
	DrainNoTimeout         = Drain{unlimited: true, set: true}
	DrainHalfMinuteTimeout = Drain{duration: defaultCleanupTimeout, set: true}
)

func DrainTimeout(duration time.Duration) Drain {
	if duration < 0 {
		invalid("drain", inputError("timeout must not be negative"))
	}

	return Drain{duration: duration, set: true}
}

// Policy is an immutable execution budget. A zero Policy is invalid.
type Policy struct {
	mode       string
	workers    int
	iterations int64
	duration   time.Duration
	drain      Drain
}

func (p Policy) Workers() int { return p.workers }
func (p Policy) Mode() string { return p.mode }
func (p Policy) applyStep(s *stepOptions) {
	if p.mode == "" {
		invalid("step", inputError("zero execution policy"))
	}

	if s.policy.mode != "" {
		invalid("step", inputError("multiple execution policies"))
	}

	s.policy = p
}

// SharedIterations runs count invocations distributed across workers.
func SharedIterations(workers int, count int64) Policy {
	p, err := TrySharedIterations(workers, count)
	if err != nil {
		invalid("shared-iterations", err)
	}

	return p
}

func TrySharedIterations(workers int, count int64) (Policy, error) {
	if workers < 1 || count < 1 {
		return Policy{}, inputError("workers and iterations must be positive")
	}

	return Policy{mode: "shared-iterations", workers: workers, iterations: count}, nil
}

// ConstantWorkers stops new invocations at duration and drains existing ones.
func ConstantWorkers(workers int, duration time.Duration, drain Drain) Policy {
	p, err := TryConstantWorkers(workers, duration, drain)
	if err != nil {
		invalid("constant-vus", err)
	}

	return p
}

func TryConstantWorkers(workers int, duration time.Duration, drain Drain) (Policy, error) {
	if workers < 1 || duration <= 0 || !drain.set {
		return Policy{}, inputError("positive workers/duration and explicit drain are required")
	}

	if !drain.unlimited && drain.duration > time.Duration(1<<63-1)-duration {
		return Policy{}, inputError("duration and drain exceed timer capacity")
	}

	return Policy{
		mode:     "constant-vus",
		workers:  workers,
		duration: duration,
		drain:    drain,
	}, nil
}

// SelectExecutor chooses an already-validated policy; invalid selection panics.
func SelectExecutor(mode string, candidates ...Policy) Policy {
	seen := map[string]bool{}

	var selected Policy

	for _, p := range candidates {
		if p.mode == "" || seen[p.mode] {
			invalid("executor selection", inputError("invalid or duplicate policy"))
		}

		seen[p.mode] = true
		if p.mode == mode {
			selected = p
		}
	}

	if selected.mode == "" {
		invalid("executor selection", inputError("unavailable executor %q", mode))
	}

	return selected
}

// RunDefaults supplies ordinary defaults for the optional standard parameter bundle.
type RunDefaults struct {
	Workers    int
	Iterations int64
	Duration   time.Duration
	Drain      Drain
}

// RunSettings contains resolved values, with no hidden engine override.
type RunSettings struct {
	Executor     string
	Workers      int
	Iterations   int64
	Duration     time.Duration
	Drain        Drain
	QueryTimeout time.Duration
}

// RunParameters is optional sugar over individual parameter declarations.
func RunParameters(p *ParamDeclarations, defaults RunDefaults) RunSettings {
	old := p.def.scope

	p.def.scope = ParamScopeRun
	defer func() { p.def.scope = old }()

	if defaults.Workers == 0 {
		defaults.Workers = 1
	}

	if defaults.Iterations == 0 {
		defaults.Iterations = 1
	}

	if defaults.Duration == 0 {
		defaults.Duration = time.Minute
	}

	if !defaults.Drain.set {
		defaults.Drain = DrainHalfMinuteTimeout
	}

	var s RunSettings

	s.Executor, _ = p.String(
		"executor",
		"shared-iterations",
		"Scenario executor.",
		OneOf("shared-iterations", "constant-vus"),
	)
	s.Workers, _ = p.Int("vus", defaults.Workers, "Concurrent virtual users.", Min(1))
	s.Iterations, _ = p.Int64(
		"iterations",
		defaults.Iterations,
		"Total shared iterations.",
		Min(int64(1)),
	)
	s.Duration, _ = p.Duration(
		"duration",
		defaults.Duration,
		"Duration of a constant-vus run.",
		Min(time.Nanosecond),
	)

	drainDefault := defaults.Drain.duration.String()
	if defaults.Drain.unlimited {
		drainDefault = "none"
	}

	raw, _ := p.String(
		"drain-timeout",
		drainDefault,
		"Grace after duration; none waits without timeout.",
	)
	if raw == "none" {
		s.Drain = DrainNoTimeout
	} else {
		value, err := time.ParseDuration(raw)
		if err != nil {
			invalid("drain-timeout", err)
		}

		s.Drain = DrainTimeout(value)
	}

	s.QueryTimeout, _ = p.Duration(
		"query-timeout",
		0,
		"Per-statement deadline; zero disables it.",
		Min(time.Duration(0)),
	)
	p.def.Drivers.QueryTimeout(s.QueryTimeout)

	return s
}

func (s RunSettings) Policy() Policy {
	return SelectExecutor(
		s.Executor,
		SharedIterations(s.Workers, s.Iterations),
		ConstantWorkers(s.Workers, s.Duration, s.Drain),
	)
}

type (
	StepOption  interface{ applyStep(options *stepOptions) }
	stepOptions struct {
		policy   Policy
		always   bool
		cleanup  time.Duration
		measured bool
		driver   DriverRef
	}
)
type stepOption func(*stepOptions)

func (o stepOption) applyStep(s *stepOptions) { o(s) }

// Always allows a reached, selected action to run after failure with a detached
// cleanup context. It does not bypass step filters or register a finalizer.
func Always(timeout time.Duration) StepOption {
	if timeout <= 0 {
		invalid("cleanup", inputError("timeout must be positive"))
	}

	return stepOption(func(s *stepOptions) { s.always = true; s.cleanup = timeout })
}

// Measure includes a once-only step in throughput measurement.
func Measure() StepOption { return stepOption(func(s *stepOptions) { s.measured = true }) }

// Use selects the default database for this step.
func Use(ref DriverRef) StepOption { return stepOption(func(s *stepOptions) { s.driver = ref }) }
