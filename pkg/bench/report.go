package bench

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"runtime"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	"github.com/stroppy-io/stroppy/pkg/config"
	"github.com/stroppy-io/stroppy/pkg/report"
)

// ReportOptions provides report identity owned by the calling application.
type ReportOptions struct {
	StroppyVersion string
	RunID          string
	Metadata       map[string]string
}

// ReportContext contains final run data available to workload contributors.
type ReportContext struct {
	Metrics map[string]MetricSnapshot
}

// ReportContribution is one workload-owned report result.
type ReportContribution struct {
	Status report.WorkloadReportStatus
	Reason string
	Data   any
}

// ReportContributor builds one independently versioned workload payload after
// teardown and final metric collection.
type ReportContributor func(ReportContext) (ReportContribution, error)

type reportDefinition struct {
	kind        string
	schema      int
	contributor ReportContributor
}

// Report declares one workload-owned payload. Declaration is optional: every
// workload receives the common report envelope without adding custom code.
func (d *Def) Report(kind string, schema int, contributor ReportContributor) {
	kind = strings.TrimSpace(kind)
	if kind == "" {
		d.addError(errors.New("report kind must not be empty"))

		return
	}

	if schema < 1 {
		d.addError(fmt.Errorf("report %q schema must be at least 1", kind))

		return
	}

	for _, declared := range d.reports {
		if declared.kind == kind {
			d.addError(fmt.Errorf("duplicate report kind %q", kind))

			return
		}
	}

	d.reports = append(d.reports, reportDefinition{kind: kind, schema: schema, contributor: contributor})
}

func newRunReport(
	name string,
	drivers map[int]*config.DriverConfig,
	params []resolvedParam,
	scenario scenarioSpec,
	steps, noSteps []string,
	options ReportOptions,
) *report.Run {
	started := time.Now().UTC()
	reportID := options.RunID
	if reportID == "" {
		reportID = uuid.NewString()
	}

	run := &report.Run{
		Schema:         report.SchemaVersion,
		Kind:           report.Kind,
		ID:             reportID,
		RunID:          options.RunID,
		StroppyVersion: options.StroppyVersion,
		StartedAt:      started,
		Status:         report.StatusFailed,
		Workload:       name,
		Drivers:        reportDrivers(drivers),
		Metadata:       maps.Clone(options.Metadata),
		Host: report.Host{
			OS: runtime.GOOS, Arch: runtime.GOARCH, GoVersion: runtime.Version(),
			CPUs: runtime.NumCPU(), MaxProcs: runtime.GOMAXPROCS(0),
		},
		Scenario:        reportScenario(scenario),
		Parameters:      reportParameters(params),
		StepSelection:   report.StepSelection{Only: slices.Clone(steps), Exclude: slices.Clone(noSteps)},
		Metrics:         map[string]report.Metric{},
		Errors:          report.ErrorSummary{Groups: []report.ErrorGroup{}},
		Steps:           []report.Step{},
		WorkloadReports: []report.WorkloadReport{},
	}

	if len(run.Drivers) > 0 {
		run.Driver = run.Drivers[0].Type
	}

	return run
}

func reportDrivers(drivers map[int]*config.DriverConfig) []report.Driver {
	indexes := make([]int, 0, len(drivers))
	for index := range drivers {
		indexes = append(indexes, index)
	}

	slices.Sort(indexes)

	out := make([]report.Driver, 0, len(indexes))
	for _, index := range indexes {
		driverConfig := drivers[index]
		if driverConfig == nil {
			continue
		}

		out = append(out, report.Driver{Index: index, Type: driverConfig.DriverType.String()})
	}

	return out
}

func reportScenario(scenario scenarioSpec) report.Scenario {
	out := report.Scenario{Executor: scenario.executor, VUs: scenario.vus}
	if scenario.executor == "constant-vus" {
		seconds := scenario.duration.Seconds()
		out.DurationSeconds = &seconds
	} else {
		iterations := scenario.iterations
		out.IterationsRequested = &iterations
	}

	return out
}

func reportParameters(params []resolvedParam) report.Parameters {
	out := report.Parameters{
		Run:      make(map[string]report.Parameter),
		Workload: make(map[string]report.Parameter),
	}

	for _, param := range params {
		target := out.Workload
		if param.scope == ParamScopeRun {
			target = out.Run
		}

		target[param.name] = report.Parameter{Value: param.value, Source: string(param.source)}
	}

	return out
}

func finalizeRunReport(
	run *report.Run,
	root *RootState,
	definitions []reportDefinition,
	data metricdata.ResourceMetrics,
	runErr error,
	phase string,
) {
	run.FinishedAt = time.Now().UTC()
	run.Steps = root.stepFilter.snapshot()
	run.Metrics = reportMetrics(data, root.metricsPrefix)
	run.Errors = reportErrors(root.errorReporter.snapshot())
	if seconds := metricTotal(run.Metrics["measurement_seconds"]); seconds > 0 {
		run.MeasurementSeconds = seconds
	}

	switch {
	case runErr != nil && errors.Is(runErr, context.Canceled):
		run.Status = report.StatusCanceled
	case runErr != nil:
		run.Status = report.StatusFailed
	case run.Errors.TerminalErrors > 0:
		run.Status = report.StatusCompletedWithErrors
	default:
		run.Status = report.StatusCompleted
	}

	if runErr != nil {
		run.Failure = &report.Failure{Phase: phase, Reason: boundReportError(runErr)}
	}

	snapshots := aggregateMetricSnapshots(data, root.metricsPrefix)
	run.WorkloadReports = buildWorkloadReports(definitions, ReportContext{Metrics: snapshots})
}

func buildWorkloadReports(definitions []reportDefinition, context ReportContext) []report.WorkloadReport {
	out := make([]report.WorkloadReport, 0, len(definitions))
	for _, definition := range definitions {
		item := report.WorkloadReport{
			Kind: definition.kind, Schema: definition.schema, Status: report.WorkloadReportMissing,
		}
		if definition.contributor == nil {
			item.Reason = "report contributor is not configured"
			out = append(out, item)

			continue
		}

		contribution, err := definition.contributor(context)
		if err != nil {
			item.Status = report.WorkloadReportError
			item.Reason = boundReportError(err)
			out = append(out, item)

			continue
		}

		item.Status = contribution.Status
		if item.Status == "" {
			item.Status = report.WorkloadReportOK
		}
		item.Reason = contribution.Reason
		if contribution.Data != nil {
			encoded, marshalErr := json.Marshal(contribution.Data)
			if marshalErr != nil {
				item.Status = report.WorkloadReportError
				item.Reason = boundReportError(fmt.Errorf("marshal report data: %w", marshalErr))
			} else {
				item.Data = encoded
			}
		}

		out = append(out, item)
	}

	return out
}

func reportErrors(summary errorSummary) report.ErrorSummary {
	out := report.ErrorSummary{
		TerminalErrors: summary.terminalErrors, FailedIterations: summary.failedIterations,
		FailedQueries: summary.failedQueries, RetryAttempts: summary.retryAttempts,
		Groups: make([]report.ErrorGroup, 0, len(summary.groups)),
	}
	for _, group := range summary.groups {
		out.Groups = append(out.Groups, report.ErrorGroup{
			Operation: group.operation, Class: string(group.kind), Count: group.count,
		})
	}

	return out
}

func boundReportError(err error) string {
	const maxBytes = 2048

	message := strings.ToValidUTF8(err.Error(), "?")
	if len(message) <= maxBytes {
		return message
	}

	return message[:maxBytes]
}

func aggregateMetricSnapshots(data metricdata.ResourceMetrics, prefix string) map[string]MetricSnapshot {
	out := make(map[string]MetricSnapshot)
	for _, scope := range data.ScopeMetrics {
		for _, metric := range scope.Metrics {
			snapshot, ok := snapshotMetric(metric)
			if ok {
				out[strings.TrimPrefix(metric.Name, prefix)] = snapshot
			}
		}
	}

	return out
}

func reportMetrics(data metricdata.ResourceMetrics, prefix string) map[string]report.Metric {
	out := make(map[string]report.Metric)
	for _, scope := range data.ScopeMetrics {
		for _, metric := range scope.Metrics {
			name := strings.TrimPrefix(metric.Name, prefix)
			converted, ok := reportMetric(metric)
			if ok {
				out[name] = converted
			}
		}
	}

	return out
}

func reportMetric(metric metricdata.Metrics) (report.Metric, bool) {
	switch aggregation := metric.Data.(type) {
	case metricdata.Sum[float64]:
		result := report.Metric{Type: "counter", Unit: metric.Unit}
		result.Series = make([]report.MetricSeries, 0, len(aggregation.DataPoints))
		var total float64
		for _, point := range aggregation.DataPoints {
			value := point.Value
			total += value
			result.Series = append(result.Series, report.MetricSeries{
				Attributes: reportAttributes(point.Attributes), Total: &value,
			})
		}
		result.Total = &total
		return result, true
	case metricdata.Gauge[float64]:
		result := report.Metric{Type: "gauge", Unit: metric.Unit}
		result.Series = make([]report.MetricSeries, 0, len(aggregation.DataPoints))
		var total float64
		for _, point := range aggregation.DataPoints {
			value := point.Value
			total += value
			result.Series = append(result.Series, report.MetricSeries{
				Attributes: reportAttributes(point.Attributes), Total: &value,
			})
		}
		result.Total = &total
		return result, true
	case metricdata.Histogram[float64]:
		snapshot := histogramSnapshot(aggregation.DataPoints)
		average := 0.0
		if snapshot.Count > 0 {
			average = snapshot.Sum / float64(snapshot.Count)
		}
		count := snapshot.Count
		sum := snapshot.Sum
		result := report.Metric{
			Type: "histogram", Unit: metric.Unit, Count: &count, Sum: &sum, Average: &average,
			Bounds: slices.Clone(snapshot.Bounds), BucketCounts: slices.Clone(snapshot.Buckets),
			Percentiles: map[string]float64{
				"p50": histogramQuantile(snapshot.Bounds, snapshot.Buckets, snapshot.Count, medianP),
				"p90": histogramQuantile(snapshot.Bounds, snapshot.Buckets, snapshot.Count, p90),
				"p95": histogramQuantile(snapshot.Bounds, snapshot.Buckets, snapshot.Count, p95),
				"p99": histogramQuantile(snapshot.Bounds, snapshot.Buckets, snapshot.Count, p99),
			},
			Series: make([]report.MetricSeries, 0, len(aggregation.DataPoints)),
		}
		for _, point := range aggregation.DataPoints {
			pointCount := point.Count
			pointSum := point.Sum
			pointAverage := 0.0
			if pointCount > 0 {
				pointAverage = pointSum / float64(pointCount)
			}
			result.Series = append(result.Series, report.MetricSeries{
				Attributes: reportAttributes(point.Attributes), Count: &pointCount, Sum: &pointSum,
				Average: &pointAverage, Bounds: slices.Clone(point.Bounds),
				BucketCounts: slices.Clone(point.BucketCounts),
			})
		}
		return result, true
	default:
		return report.Metric{}, false
	}
}

func reportAttributes(set attribute.Set) map[string]string {
	values := set.ToSlice()
	if len(values) == 0 {
		return nil
	}

	out := make(map[string]string, len(values))
	for _, value := range values {
		out[string(value.Key)] = value.Value.String()
	}

	return out
}

func metricTotal(metric report.Metric) float64 {
	if metric.Total == nil {
		return 0
	}

	return *metric.Total
}
