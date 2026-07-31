package bench

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/zap"

	"github.com/stroppy-io/stroppy/pkg/common/proto/stroppy"
	"github.com/stroppy-io/stroppy/pkg/driver"
)

// Workload is a Go-native benchmark. Setup runs once (schema + load steps);
// Iterate is the measured body driven across VUs by the executor; Teardown runs once.
type Workload interface {
	Name() string
	Setup(ctx context.Context, b *Bench) error
	Iterate(ctx context.Context, b *Bench) error
	Teardown(ctx context.Context, b *Bench) error
}

// Bench is the per-VU session handed to a workload: the shared driver (pool-backed,
// safe for concurrent VUs), the VU's identity/step-tag for metrics, and the SDK API.
type Bench struct {
	root *RootState
	vu   *VU
	lg   *zap.Logger
	drv  driver.Driver
	cfg  *stroppy.DriverConfig

	stepStart time.Time
}

// Driver returns the raw driver (escape hatch).
func (b *Bench) Driver() driver.Driver { return b.drv }

// DriverType returns the resolved driver type proto enum.
func (b *Bench) DriverType() stroppy.DriverConfig_DriverType { return b.cfg.GetDriverType() }

// DriverTypeName returns the driver type as the string enum a workload authors with.
func (b *Bench) DriverTypeName() DriverTypeName {
	return DriverTypeNameFromProto(b.cfg.GetDriverType())
}

// VUID returns the 1-based VU id.
func (b *Bench) VUID() uint64 { return b.vu.VUID() }

// Logger returns the session logger.
func (b *Bench) Logger() *zap.Logger { return b.lg }

// --- registry ---

var (
	regMu        sync.Mutex
	regWorkloads = map[string]Workload{}

	errNoWorkloadRegistered = errors.New("bench: no workload registered")
	errDriverIndexMissing   = errors.New("bench: driver index 0 not configured")
	errUnsupportedExecutor  = errors.New("unsupported executor")
)

// Register a Go workload (called from workload init()).
func Register(w Workload) {
	regMu.Lock()
	defer regMu.Unlock()

	regWorkloads[w.Name()] = w
}

// Lookup a registered Go workload by name.
func Lookup(name string) (Workload, bool) {
	regMu.Lock()
	defer regMu.Unlock()

	w, ok := regWorkloads[name]

	return w, ok
}

// Run looks up the named Go workload and executes it: Setup once, Iterate across the
// scenario, Teardown once. drivers carries the resolved per-index driver configs (same
// *stroppy.DriverConfig the TS path consumes); env is the script env (-e overrides +
// config), consulted by Env after the real process environment. Scenario comes from
// VUS/DURATION/ITER env.
func Run(
	ctx context.Context,
	name string,
	drivers map[int]*stroppy.DriverConfig,
	env map[string]string,
	lg *zap.Logger,
) error {
	wl, ok := Lookup(name)
	if !ok {
		return fmt.Errorf("%w as %q", errNoWorkloadRegistered, name)
	}

	root = newRootState(lg, ctx, env)
	sum := newSummary(root)

	sum.start(ctx)
	defer sum.print()
	defer root.Teardown()

	cfg := drivers[0]
	if cfg == nil {
		return errDriverIndexMissing
	}

	drv, err := driver.Dispatch(ctx, driver.Options{Config: cfg, Logger: lg, DialFunc: root.dialer.DialContext})
	if err != nil {
		return fmt.Errorf("driver dispatch: %w", err)
	}

	setupVU := &VU{root: root, vuid: 1, initPhase: true, ctx: ctx}
	setupBench := &Bench{
		root: root, vu: setupVU,
		lg:  lg.Named("workload").With(zap.String("workload", name)),
		drv: drv, cfg: cfg,
	}

	if err := wl.Setup(ctx, setupBench); err != nil {
		return fmt.Errorf("setup: %w", err)
	}

	sc := readScenario()
	if err := runScenario(ctx, sc, func(vu *VU) error {
		b := &Bench{
			root: root, vu: vu,
			lg:  lg.Named("workload").With(zap.String("workload", name), zap.Uint64("VUID", vu.VUID())),
			drv: drv, cfg: cfg,
		}

		return wl.Iterate(ctx, b)
	}); err != nil {
		return fmt.Errorf("scenario %q: %w", sc.name, err)
	}

	if err := wl.Teardown(ctx, setupBench); err != nil {
		return fmt.Errorf("teardown: %w", err)
	}

	return nil
}

// --- scenario (read from env, mirrors declareScenario) ---

type scenarioSpec struct {
	name       string
	executor   string
	vus        int
	iterations int64
	duration   time.Duration
}

func readScenario() scenarioSpec {
	spec := scenarioSpec{name: "workload"}

	spec.vus = EnvInt("VUS", 1)
	if spec.vus < 1 {
		spec.vus = 1
	}

	if d := Env("DURATION", ""); d != "" {
		dur, err := time.ParseDuration(d)
		if err == nil {
			spec.executor = "constant-vus"
			spec.duration = dur

			return spec
		}
	}

	spec.executor = "shared-iterations"

	spec.iterations = int64(EnvInt("ITER", 1))
	if spec.iterations < 1 {
		spec.iterations = 1
	}

	return spec
}

// --- executor (shared-iterations + constant-vus) ---

func runScenario(ctx context.Context, sc scenarioSpec, iterate func(*VU) error) error {
	switch sc.executor {
	case "shared-iterations":
		var (
			remaining = sc.iterations
			wg        sync.WaitGroup
		)
		for i := range sc.vus {
			wg.Add(1)
			go func(vuid int) {
				defer wg.Done()

				runWorker(ctx, vuid, iterate, func() bool {
					return atomic.AddInt64(&remaining, -1) >= 0
				})
			}(i + 1)
		}

		wg.Wait()
	case "constant-vus":
		deadline := time.Now().Add(sc.duration)

		var wg sync.WaitGroup
		for i := range sc.vus {
			wg.Add(1)
			go func(vuid int) {
				defer wg.Done()

				runWorker(ctx, vuid, iterate, func() bool {
					return time.Now().Before(deadline)
				})
			}(i + 1)
		}

		wg.Wait()
	default:
		return fmt.Errorf("%w %q", errUnsupportedExecutor, sc.executor)
	}

	return nil
}

func runWorker(ctx context.Context, vuid int, iterate func(*VU) error, keep func() bool) {
	vu := &VU{root: root, vuid: uint64(vuid), ctx: ctx} //nolint:gosec // G115: scale-bound, no overflow
	for keep() {
		vu.iterTest++
		vu.iterScenario++

		start := time.Now()
		err := iterate(vu)
		root.txMetrics.recordIteration(vu, time.Since(start))

		if err != nil {
			if errors.Is(err, errAbort) {
				return
			}

			root.lg.Error("iteration failed", zap.Int("vu", vuid), zap.Error(err))

			return
		}
	}
}

var errAbort = errors.New("aborted")

// --- metric handles (Counter/Trend/Rate) ---

type Metric struct {
	root *RootState
	m    *metric
}

func (b *Bench) Counter(name string) *Metric { return b.newMetric(name, Counter) }
func (b *Bench) Trend(name string) *Metric   { return b.newMetric(name, Trend) }
func (b *Bench) Rate(name string) *Metric    { return b.newMetric(name, Rate) }

func (b *Bench) newMetric(name string, t metricType) *Metric {
	m, err := b.root.registry.NewMetric(name, t)
	if err != nil {
		b.lg.Fatal("can't register metric", zap.String("name", name), zap.Error(err))
	}

	return &Metric{root: b.root, m: m}
}

// Add records a value with optional tag key/values.
func (m *Metric) Add(v float64, tags ...string) {
	tagSet := m.root.registry.RootTagSet()
	for i := 0; i+1 < len(tags); i += 2 {
		tagSet = tagSet.With(tags[i], tags[i+1])
	}

	PushIfNotDone(context.Background(), m.root.samples, Sample{
		Metric: m.m, Tags: tagSet,
		Time: time.Now(), Value: v,
	})
}

// --- summary (drain samples, print) ---

type metricSink struct {
	typ   metricType
	count uint64
	sum   float64
	vals  []float64
}

type summary struct {
	root  *RootState
	mu    sync.Mutex
	sinks map[string]*metricSink
}

func newSummary(r *RootState) *summary { return &summary{root: r, sinks: map[string]*metricSink{}} }

func (s *summary) start(ctx context.Context) {
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case sc, ok := <-s.root.samples:
				if !ok {
					return
				}

				s.ingest(sc)
			}
		}
	}()
}

func (s *summary) ingest(sc SampleContainer) {
	for _, sample := range sc.GetSamples() {
		name := sample.Metric.Name

		s.mu.Lock()

		sk, ok := s.sinks[name]
		if !ok {
			sk = &metricSink{typ: sample.Metric.Type}
			s.sinks[name] = sk
		}

		sk.count++

		sk.sum += sample.Value
		if sk.typ == Trend {
			sk.vals = append(sk.vals, sample.Value)
		}
		s.mu.Unlock()
	}
}

func (s *summary) print() {
	close(s.root.samples)

	for sc := range s.root.samples {
		s.ingest(sc)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if len(s.sinks) == 0 {
		fmt.Fprintln(os.Stderr, "bench: no metrics recorded")

		return
	}

	names := make([]string, 0, len(s.sinks))
	for n := range s.sinks {
		names = append(names, n)
	}

	sort.Strings(names)
	fmt.Fprintln(os.Stderr, "\n=== bench summary ===")

	for _, n := range names {
		sk := s.sinks[n]
		switch sk.typ {
		case Counter:
			fmt.Fprintf(os.Stderr, "  %-40s %d\n", n, uint64(sk.sum))
		case Trend:
			fmt.Fprintf(os.Stderr, "  %-40s count=%d avg=%.3f %s\n",
				n, sk.count, sk.sum/float64(max1(sk.count)), percentiles(sk.vals))
		case Rate:
			pct := 0.0
			if sk.count > 0 {
				pct = 100 * sk.sum / float64(sk.count)
			}

			fmt.Fprintf(os.Stderr, "  %-40s %.2f%%  %d out of %d\n", n, pct, uint64(sk.sum), sk.count)
		default:
			fmt.Fprintf(os.Stderr, "  %-40s count=%d sum=%.3f\n", n, sk.count, sk.sum)
		}
	}
}

func max1(n uint64) uint64 {
	if n == 0 {
		return 1
	}

	return n
}

func percentiles(vals []float64) string {
	if len(vals) == 0 {
		return ""
	}

	s := make([]float64, len(vals))
	copy(s, vals)
	sort.Float64s(s)

	pct := func(p float64) float64 { return s[int(float64(len(s)-1)*p)] }

	return fmt.Sprintf("p(50)=%.3f p(90)=%.3f p(95)=%.3f p(99)=%.3f", pct(0.5), pct(0.9), pct(0.95), pct(0.99))
}
