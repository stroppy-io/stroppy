package bench

import (
	"context"
	"errors"
	"fmt"
	"os"
	"reflect"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"go.uber.org/zap"

	"github.com/stroppy-io/stroppy/pkg/common/logger"
	"github.com/stroppy-io/stroppy/pkg/config"
	"github.com/stroppy-io/stroppy/pkg/driver"
)

// Workload is a Go-native benchmark. Define declares and binds typed parameters;
// Setup runs once (schema + load steps); Iterate is the measured body driven across
// VUs by the executor; Teardown runs once.
type Workload interface {
	Name() string
	Define(def *Def) error
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
	cfg  *config.DriverConfig

	// insertMethod is the effective row-insert method resolved for this run.
	// Zero means unset: the workload's own InsertRequest.Method is kept.
	insertMethod driver.InsertMethod

	stepStart time.Time
}

// Driver returns the raw driver (escape hatch).
func (b *Bench) Driver() driver.Driver { return b.drv }

// DriverType returns the resolved driver type enum.
func (b *Bench) DriverType() config.DriverType { return b.cfg.DriverType }

// DriverTypeName returns the driver type as the string enum a workload authors with.
func (b *Bench) DriverTypeName() DriverTypeName {
	return DriverTypeNameOf(b.cfg.DriverType)
}

// VUID returns the 1-based VU id.
func (b *Bench) VUID() uint64 { return b.vu.VUID() }

// Logger returns the session logger.
func (b *Bench) Logger() *zap.Logger { return b.lg }

// --- registry ---

var (
	regMu        sync.RWMutex
	regWorkloads = map[string]func() Workload{}

	errNoWorkloadRegistered      = errors.New("bench: no workload registered")
	errDriverIndexMissing        = errors.New("bench: driver index 0 not configured")
	errUnsupportedExecutor       = errors.New("unsupported executor")
	errVUsOutOfRange             = errors.New("vus must be at least 1")
	errIterationsOutOfRange      = errors.New("iterations must be at least 1")
	errDurationOutOfRange        = errors.New("duration must be positive")
	errDurationNeedsExecutor     = errors.New("duration requires an explicit constant-vus executor")
	errDurationWithWrongExecutor = errors.New("duration is only valid with the constant-vus executor")
	errConstantVUsNeedsDuration  = errors.New("constant-vus requires duration")
	errNegativeQueryTimeout      = errors.New("query-timeout must not be negative")
)

// Register adds a workload factory. Workload packages call it during init.
func Register(factory func() Workload) {
	if factory == nil {
		panic("bench: register nil workload factory")
	}

	wl := factory()
	if nilWorkload(wl) {
		panic("bench: workload factory returned nil")
	}

	name := wl.Name()
	if name == "" {
		panic("bench: register workload with empty name")
	}

	regMu.Lock()
	defer regMu.Unlock()

	if _, exists := regWorkloads[name]; exists {
		panic(fmt.Sprintf("bench: workload %q already registered", name))
	}

	regWorkloads[name] = factory
}

// Lookup returns a fresh instance of a registered Go workload.
func Lookup(name string) (Workload, bool) {
	regMu.RLock()

	factory, ok := regWorkloads[name]

	regMu.RUnlock()

	if !ok {
		return nil, false
	}

	wl := factory()
	if nilWorkload(wl) || wl.Name() != name {
		panic(fmt.Sprintf("bench: workload factory for %q returned an invalid workload", name))
	}

	return wl, true
}

func nilWorkload(workload Workload) bool {
	if workload == nil {
		return true
	}

	value := reflect.ValueOf(workload)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}

// Describe returns a workload's deterministic parameter schema without setup or drivers.
func Describe(name string) (Description, error) {
	wl, ok := Lookup(name)
	if !ok {
		return Description{}, fmt.Errorf("%w as %q", errNoWorkloadRegistered, name)
	}

	_, schema, err := defineWorkload(wl, ParamInputs{}, true)
	if err != nil {
		return Description{}, fmt.Errorf("define workload %q: %w", name, err)
	}

	return Description{Name: name, Params: schema}, nil
}

// DescribeAll returns all registered workload schemas ordered by workload name.
func DescribeAll() ([]Description, error) {
	regMu.RLock()

	names := make([]string, 0, len(regWorkloads))
	for name := range regWorkloads {
		names = append(names, name)
	}

	regMu.RUnlock()

	slices.Sort(names)

	descriptions := make([]Description, 0, len(names))
	for _, name := range names {
		description, err := Describe(name)
		if err != nil {
			return nil, err
		}

		descriptions = append(descriptions, description)
	}

	return descriptions, nil
}

// Run looks up a fresh workload instance and executes it: Define and parameter
// resolution first, Setup once, Iterate across the scenario, then Teardown once.
// steps/noSteps are the explicit --steps / --no-steps filters; drivers and params
// flow explicitly through the typed configuration channels.
func Run(
	ctx context.Context,
	name string,
	drivers map[int]*config.DriverConfig,
	paramInputs ParamInputs,
	steps, noSteps []string,
	metricsConfig *MetricsConfig,
) error {
	lg := logger.Global()

	wl, ok := Lookup(name)
	if !ok {
		return fmt.Errorf("%w as %q", errNoWorkloadRegistered, name)
	}

	scenarioParams, _, err := defineWorkload(wl, paramInputs, false)
	if err != nil {
		return fmt.Errorf("define workload %q: %w", name, err)
	}

	sc, err := scenarioParams.spec(lg)
	if err != nil {
		return fmt.Errorf("scenario: %w", err)
	}

	root, err = newRootState(lg, ctx, steps, noSteps, metricsConfig)
	if err != nil {
		return fmt.Errorf("initialize metrics: %w", err)
	}

	sum := newSummary(root)

	defer root.shutdownMetrics()
	defer sum.print()
	defer func() { _ = root.Teardown() }()

	cfg := drivers[0]
	if cfg == nil {
		return errDriverIndexMissing
	}

	insertMethod, err := resolveInsertMethod(scenarioParams.insertMethod, cfg)
	if err != nil {
		return fmt.Errorf("insert method: %w", err)
	}

	queryTimeout := scenarioParams.queryTimeout.Value()
	if queryTimeout < 0 {
		return fmt.Errorf("%w, got %s", errNegativeQueryTimeout, queryTimeout)
	}

	drv, err := driver.Dispatch(ctx, driver.Options{
		Config:       cfg,
		DialFunc:     root.dialer.DialContext,
		QueryTimeout: queryTimeout,
	})
	if err != nil {
		return fmt.Errorf("driver dispatch: %w", err)
	}

	setupVU := &VU{root: root, vuid: 1, initPhase: true, ctx: ctx}
	setupBench := &Bench{
		root: root, vu: setupVU,
		lg:  lg.Named("workload").With(zap.String("workload", name)),
		drv: drv, cfg: cfg, insertMethod: insertMethod,
	}

	if err := wl.Setup(ctx, setupBench); err != nil {
		return fmt.Errorf("setup: %w", err)
	}

	if err := runScenario(ctx, sc, func(vu *VU) error {
		b := &Bench{
			root: root, vu: vu,
			lg:  lg.Named("workload").With(zap.String("workload", name), zap.Uint64("VUID", vu.VUID())),
			drv: drv, cfg: cfg, insertMethod: insertMethod,
		}

		return wl.Iterate(vu.Context(), b)
	}); err != nil {
		return fmt.Errorf("scenario %q: %w", sc.name, err)
	}

	if err := wl.Teardown(ctx, setupBench); err != nil {
		return fmt.Errorf("teardown: %w", err)
	}

	return nil
}

// --- scenario ---

type scenarioSpec struct {
	name       string
	executor   string
	vus        int
	iterations int64
	duration   time.Duration
}

type scenarioParams struct {
	executor   Param[string]
	vus        Param[int]
	iterations Param[int64]
	duration   Param[time.Duration]

	insertMethod Param[string]
	queryTimeout Param[time.Duration]
}

func defineWorkload(
	wl Workload,
	inputs ParamInputs,
	defaultsOnly bool,
) (scenarioParams, []ParamSchema, error) {
	def := newDef(inputs, defaultsOnly)

	iterationOptions := []ParamOption{LegacyEnvAliases("ITER")}
	if !defaultsOnly && effectiveDurationIsLegacy(inputs) {
		iterationOptions = nil
	}

	params := scenarioParams{
		executor: def.Param.String(
			"executor", "shared-iterations", "Scenario executor: shared-iterations or constant-vus.",
		),
		vus: def.Param.Int("vus", 1, "Number of concurrent virtual users."),
		iterations: def.Param.Int64(
			"iterations", 1, "Total shared iterations.", iterationOptions...,
		),
		duration: def.Param.Duration("duration", 0, "Duration of a constant-vus scenario."),
		insertMethod: def.Param.String(
			"insert-method", "",
			"Row insert method: plain_query, plain_bulk, columnar, or native.",
			DerivedDefault("workload default"),
		),
		queryTimeout: def.Param.Duration(
			"query-timeout", 0,
			"Per-statement query deadline (e.g. 30s, 5s, 500ms); 0 disables it.",
		),
	}

	def.scope = ParamScopeWorkload
	defineErr := wl.Define(def)

	return params, def.schema(), errors.Join(defineErr, def.finish())
}

// resolveInsertMethod merges the explicit typed --insert-method override with
// the user-set driver-level defaultInsertMethod (-D/JSON/config drivers).
// Precedence: an explicit typed value (CLI/env/config) wins; otherwise the
// user-set driver method is used; when neither is set the result is zero, so
// workloads keep their own InsertRequest.Method. Presets carry no method, so
// there is no hidden preset tier.
func resolveInsertMethod(paramValue Param[string], cfg *config.DriverConfig) (driver.InsertMethod, error) {
	if paramValue.Explicit() {
		return driver.ResolveInsertMethod(cfg.DriverType, paramValue.Value())
	}

	if cfg.GetInsertMethod() != "" {
		return driver.ResolveInsertMethod(cfg.DriverType, cfg.GetInsertMethod())
	}

	return 0, nil
}

func effectiveDurationIsLegacy(inputs ParamInputs) bool {
	if _, ok := inputs.CLI["duration"]; ok {
		return false
	}

	_, processDuration := os.LookupEnv("DURATION")
	_, legacyEnvDuration := inputs.LegacyEnv["DURATION"]
	_, runConfigDuration := inputs.RunConfig["duration"]
	_, configEnvDuration := inputs.LegacyConfigEnv["DURATION"]

	switch {
	case processDuration, legacyEnvDuration:
		return true
	case runConfigDuration:
		return false
	default:
		return configEnvDuration
	}
}

func (params *scenarioParams) spec(lg *zap.Logger) (scenarioSpec, error) {
	executor := params.executor.Value()
	legacyDuration := params.duration.Explicit() && slices.Contains([]ParamSource{
		ParamSourceProcessEnv,
		ParamSourceLegacyEnv,
		ParamSourceLegacyConfigEnv,
	}, params.duration.Source())

	if legacyDuration && !params.executor.Explicit() {
		executor = "constant-vus"

		if lg != nil {
			lg.Warn(
				"legacy DURATION inferred the constant-vus executor; set executor explicitly",
				zap.String("source", string(params.duration.Source())),
			)
		}
	}

	if params.vus.Value() < 1 {
		return scenarioSpec{}, fmt.Errorf("%w, got %d", errVUsOutOfRange, params.vus.Value())
	}

	if params.iterations.Value() < 1 {
		return scenarioSpec{}, fmt.Errorf("%w, got %d", errIterationsOutOfRange, params.iterations.Value())
	}

	if params.duration.Explicit() && params.duration.Value() <= 0 {
		return scenarioSpec{}, fmt.Errorf("%w, got %s", errDurationOutOfRange, params.duration.Value())
	}

	if params.duration.Explicit() && !legacyDuration &&
		(!params.executor.Explicit() || executor != "constant-vus") {
		return scenarioSpec{}, errDurationNeedsExecutor
	}

	spec := scenarioSpec{
		name:       "workload",
		executor:   executor,
		vus:        params.vus.Value(),
		iterations: params.iterations.Value(),
		duration:   params.duration.Value(),
	}

	switch executor {
	case "shared-iterations":
		if params.duration.Explicit() {
			return scenarioSpec{}, errDurationWithWrongExecutor
		}
	case "constant-vus":
		if !params.duration.Explicit() {
			return scenarioSpec{}, errConstantVUsNeedsDuration
		}
	default:
		return scenarioSpec{}, fmt.Errorf("%w %q", errUnsupportedExecutor, executor)
	}

	return spec, nil
}

// --- executor (shared-iterations + constant-vus) ---

func runScenario(ctx context.Context, sc scenarioSpec, iterate func(*VU) error) error {
	scenarioCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	fatalErrors := make(chan error, 1)

	var wg sync.WaitGroup

	startWorker := func(vuid int, keep func() bool) {
		wg.Go(func() {
			if err := runWorker(scenarioCtx, vuid, iterate, keep); IsFatalError(err) {
				select {
				case fatalErrors <- err:
					cancel()
				default:
				}
			}
		})
	}

	switch sc.executor {
	case "shared-iterations":
		remaining := sc.iterations
		for i := range sc.vus {
			startWorker(i+1, func() bool {
				return atomic.AddInt64(&remaining, -1) >= 0
			})
		}
	case "constant-vus":
		deadline := time.Now().Add(sc.duration)
		for i := range sc.vus {
			startWorker(i+1, func() bool {
				return time.Now().Before(deadline)
			})
		}
	default:
		return fmt.Errorf("%w %q", errUnsupportedExecutor, sc.executor)
	}

	wg.Wait()

	select {
	case err := <-fatalErrors:
		return err
	default:
	}

	return ctx.Err()
}

func runWorker(ctx context.Context, vuid int, iterate func(*VU) error, keep func() bool) error {
	vu := &VU{root: root, vuid: uint64(vuid), ctx: ctx} //nolint:gosec // G115: scale-bound, no overflow
	for keep() {
		if err := ctx.Err(); err != nil {
			return err
		}

		vu.iterTest++
		vu.iterScenario++

		start := time.Now()
		err := iterate(vu)
		root.txMetrics.recordIteration(vu, time.Since(start))

		if err != nil {
			if IsFatalError(err) {
				return err
			}

			if ctx.Err() != nil && errors.Is(err, ctx.Err()) {
				return ctx.Err()
			}

			root.lg.Error("iteration failed", zap.Int("vu", vuid), zap.Error(err))

			return nil
		}
	}

	return nil
}

// --- metric handles (Counter/Trend/Rate) ---

type Metric struct {
	root *RootState
	m    *metric
}

func (b *Bench) Counter(name string) *Metric { return b.newMetric(name, Counter) }
func (b *Bench) Trend(name string) *Metric   { return b.newMetric(name, Trend) }
func (b *Bench) Rate(name string) *Metric    { return b.newMetric(name, Rate) }

func (b *Bench) newMetric(name string, typ metricType) *Metric {
	m, err := b.root.registry.NewMetric(name, typ)
	if err != nil {
		b.lg.Fatal("can't register metric", zap.String("name", name), zap.Error(err))
	}

	return &Metric{root: b.root, m: m}
}

// Add records a value with optional tag key/value pairs.
func (m *Metric) Add(value float64, tags ...string) {
	m.m.add(context.Background(), value, m.m.taggedAttributes(tags))
}

// --- summary ---

type summary struct {
	root *RootState
}

func newSummary(root *RootState) *summary { return &summary{root: root} }

func (s *summary) print() {
	var data metricdata.ResourceMetrics
	if err := s.root.manualReader.Collect(context.Background(), &data); err != nil {
		fmt.Fprintf(os.Stderr, "bench: collect metrics: %v\n", err)

		return
	}

	var lines []string

	for _, scope := range data.ScopeMetrics {
		for _, metric := range scope.Metrics {
			name := strings.TrimPrefix(metric.Name, s.root.metricsPrefix)
			switch aggregation := metric.Data.(type) {
			case metricdata.Sum[float64]:
				var total float64
				for _, point := range aggregation.DataPoints {
					total += point.Value
				}

				lines = append(lines, fmt.Sprintf("  %-40s %.3f", name, total))
			case metricdata.Gauge[float64]:
				lines = append(lines, fmt.Sprintf("  %-40s %.3f", name, sumGauge(aggregation.DataPoints)))
			case metricdata.Histogram[float64]:
				lines = append(lines, formatHistogramSummary(name, aggregation.DataPoints))
			}
		}
	}

	if len(lines) == 0 {
		fmt.Fprintln(os.Stderr, "bench: no metrics recorded")

		return
	}

	fmt.Fprintln(os.Stderr, "\n=== bench summary ===")

	for _, line := range lines {
		fmt.Fprintln(os.Stderr, line)
	}
}

func sumGauge(points []metricdata.DataPoint[float64]) float64 {
	var total float64
	for _, point := range points {
		total += point.Value
	}

	return total
}

func formatHistogramSummary(name string, points []metricdata.HistogramDataPoint[float64]) string {
	var (
		count   uint64
		sum     float64
		bounds  []float64
		buckets []uint64
	)

	for _, point := range points {
		count += point.Count

		sum += point.Sum
		if len(bounds) == 0 {
			bounds = point.Bounds
			buckets = make([]uint64, len(point.BucketCounts))
		}

		for i, bucketCount := range point.BucketCounts {
			buckets[i] += bucketCount
		}
	}

	average := 0.0
	if count > 0 {
		average = sum / float64(count)
	}

	return fmt.Sprintf(
		"  %-40s count=%d avg=%.3f p(50)~=%.3f p(90)~=%.3f p(95)~=%.3f p(99)~=%.3f",
		name, count, average,
		histogramQuantile(bounds, buckets, count, medianP),
		histogramQuantile(bounds, buckets, count, p90),
		histogramQuantile(bounds, buckets, count, p95),
		histogramQuantile(bounds, buckets, count, p99),
	)
}

func histogramQuantile(bounds []float64, buckets []uint64, count uint64, quantile float64) float64 {
	if count == 0 || len(buckets) == 0 {
		return 0
	}

	target := uint64(float64(count-1)*quantile) + 1

	var cumulative uint64
	for i, bucketCount := range buckets {
		cumulative += bucketCount
		if cumulative >= target {
			if i < len(bounds) {
				return bounds[i]
			}

			if len(bounds) > 0 {
				return bounds[len(bounds)-1]
			}
		}
	}

	return 0
}
