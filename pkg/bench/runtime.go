package bench

import (
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"time"

	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"go.uber.org/zap"

	"github.com/stroppy-io/stroppy/v6/internal/testdriver"
	"github.com/stroppy-io/stroppy/v6/pkg/config"
	"github.com/stroppy-io/stroppy/v6/pkg/driver"
	"github.com/stroppy-io/stroppy/v6/pkg/report"
)

const defaultCleanupTimeout = 30 * time.Second

// Bench is a stable step-worker-scoped set of author capabilities. Workers own
// separate Bench values; workload state remains ordinary Go state.
type Bench struct {
	root         *rootState
	vu           *VU
	lg           *zap.Logger
	drv          driver.Driver
	cfg          *config.DriverConfig
	execution    *Execution
	Log          Logger
	databaseName string
}

func (b *Bench) Worker() int                    { return b.vu.worker }
func (b *Bench) Iteration() uint64              { return b.vu.iterScenario }
func (b *Bench) DriverTypeName() DriverTypeName { return DriverTypeNameOf(b.cfg.DriverType) }

// RunOptions is explicit input for one fresh observation/execution operation.
type RunOptions struct {
	Drivers map[string]DriverConfig
	Params  ParamInputs
	Steps   []string
	NoSteps []string
	Logger  Logger
	Metrics *MetricsConfig
	Report  *ReportOptions
}

// RunTest observes a definition, validates inputs, then executes a fresh replay.
// Reports are returned as data; this function never writes report history.
//
//nolint:gocognit,nestif,cyclop,funlen,gocritic // observation, execution and finalization form one operation.
func RunTest(ctx context.Context, test Test, options RunOptions) (result *report.Run, err error) {
	defer recoverValidation(&err)

	observed, err := observeSelected(
		test,
		options.Params,
		copyDriverConfigs(options.Drivers),
		false,
		options.Steps,
		options.NoSteps,
		testdriver.FromContext(ctx),
	)
	if err != nil {
		return nil, fmt.Errorf("define %q: %w", test.Name, err)
	}

	log := options.Logger.backend
	if log == nil {
		log = zap.NewNop()
	}

	root, err := newrootState(log, ctx, options.Steps, options.NoSteps, options.Metrics)
	if err != nil {
		return nil, err
	}
	defer root.shutdownMetrics()

	d := newDef(observed.inputs, false)
	d.environment = observed.environment
	d.Drivers.configs = copyDriverConfigs(observed.Drivers.configs)
	d.Drivers.testBackend = observed.Drivers.testBackend
	e := &d.Execution
	e.ctx = ctx
	e.root = root
	e.databases = map[string]*databaseSlot{}
	e.measurements = map[string]float64{}
	e.policies = map[string]report.Execution{}

	e.reporting = options.Report != nil
	if options.Report != nil {
		result = newRunReport(
			test.Name,
			observed.resolved,
			options.Steps,
			options.NoSteps,
			*options.Report,
		)
	}

	defer func() {
		var panicValue any

		if value := recover(); value != nil {
			if failure, ok := value.(*ValidationError); ok {
				err = errors.Join(err, failure)
			} else {
				panicValue = value
			}
		}

		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), defaultCleanupTimeout)
		for name, slot := range e.databases {
			if closeErr := slot.drv.Teardown(cleanupCtx); closeErr != nil {
				err = errors.Join(err, fmt.Errorf("driver %s cleanup: %w", name, closeErr))
			}
		}

		cancel()

		err = driver.JoinErrors(err, e.Err(), ctx.Err())
		if result != nil {
			result.Measurements = e.measurements
			result.Executions = e.policies
			result.Parameters = reportParameters(d.resolved)
			result.Drivers = nil
			result.Driver = ""
			configurations := map[string]DriverConfig{}

			for name, value := range d.Drivers.configs {
				if name == "default" {
					name = ""
				}

				configurations[name] = value
			}

			for name, value := range d.Drivers.declared {
				if name == "default" {
					name = ""
				}

				configurations[name] = value
			}

			if _, ok := configurations[""]; !ok {
				configurations[""] = e.configuration("")
			}

			names := make([]string, 0, len(configurations))
			for name := range configurations {
				names = append(names, name)
			}

			slices.Sort(names)

			for _, name := range names {
				value := configurations[name]

				publicName := name
				if name == "" {
					publicName = "default"
					result.Driver = string(value.Kind)
				}

				result.Drivers = append(result.Drivers, report.Driver{Name: publicName, Type: string(value.Kind)})
			}

			for _, step := range e.observed {
				execution, exists := e.policies[step.Name]
				if !exists {
					continue
				}

				result.Scenario = report.Scenario{Executor: execution.Executor, VUs: execution.Workers}
				if execution.Executor == "shared-iterations" {
					count := execution.Iterations
					result.Scenario.IterationsRequested = &count
				} else {
					seconds := execution.DurationSeconds
					result.Scenario.DurationSeconds = &seconds
				}

				break
			}
		}

		phase := e.failurePhase
		if phase == "" {
			phase = e.phase
		}

		result, err = finishRun(result, root, d.reports, err, phase)

		if panicValue != nil {
			panic(panicValue)
		}
	}()

	err = test.Define(d)

	return result, err
}

// RunCatalog executes one descriptor selected from an explicit catalog.
//
//nolint:gocritic // run inputs are copied operation values.
func RunCatalog(ctx context.Context, catalog *Catalog, name string, options RunOptions) (*report.Run, error) {
	test, ok := catalog.Test(name)
	if !ok {
		return nil, fmt.Errorf("%w %q", errNoWorkloadRegistered, name)
	}

	return RunTest(ctx, test, options)
}

func finishRun(
	runReport *report.Run,
	root *rootState,
	definitions []reportDefinition,
	runErr error,
	phase string,
) (*report.Run, error) {
	root.errorReporter.stopAndWait()

	var data metricdata.ResourceMetrics
	if err := root.manualReader.Collect(context.Background(), &data); err != nil {
		runErr = errors.Join(runErr, fmt.Errorf("collect metrics: %w", err))
		phase = "report"
	}

	mergeMetricWriters(&data)

	if root.onSummary != nil {
		root.onSummary(reportMetrics(data, root.metricsPrefix))
	}

	if runReport != nil {
		finalizeRunReport(runReport, root, definitions, data, runErr, phase)

		for index, definition := range definitions {
			if definition.renderer != nil && index < len(runReport.WorkloadReports) &&
				runReport.WorkloadReports[index].Data != nil {
				if renderErr := definition.renderer(root.summaryWriter, runReport.WorkloadReports[index].Data); renderErr != nil {
					item := &runReport.WorkloadReports[index]
					item.Status = report.WorkloadReportError
					item.Reason = boundReportError(renderErr)
				}
			}
		}
	}

	newSummary(root).printDataTo(root.summaryWriter, data)

	return runReport, runErr
}

// --- summary ---

type summary struct {
	root *rootState
}

func newSummary(root *rootState) *summary { return &summary{root: root} }

func (s *summary) printTo(out io.Writer) {
	var data metricdata.ResourceMetrics
	if err := s.root.manualReader.Collect(context.Background(), &data); err != nil {
		if !s.root.quietSummary {
			fmt.Fprintf(out, "bench: collect metrics: %v\n", err)
		}

		return
	}

	mergeMetricWriters(&data)

	if s.root.onSummary != nil {
		s.root.onSummary(reportMetrics(data, s.root.metricsPrefix))
	}

	s.printDataTo(out, data)
}

func (s *summary) printDataTo(out io.Writer, data metricdata.ResourceMetrics) {
	if s.root.errorReporter != nil && !s.root.quietSummary {
		defer s.root.errorReporter.writeSummary(out)
	}

	if s.root.quietSummary {
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
		fmt.Fprintln(out, "bench: no metrics recorded")

		return
	}

	fmt.Fprintln(out, "\n=== bench summary ===")

	for _, line := range lines {
		fmt.Fprintln(out, line)
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
