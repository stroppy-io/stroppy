package bench

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"runtime"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	"github.com/stroppy-io/stroppy/v6/pkg/report"
)

// ReportOptions provides report identity owned by the calling application.
type ReportOptions struct {
	StroppyVersion string
	BuildDigest    string
	RunID          string
	Metadata       map[string]string
}

// ReportContext contains final run data available to workload contributors.
type ReportContext struct {
	Metrics      map[string]MetricSnapshot
	Series       map[string]report.Metric
	Steps        []report.Step
	Measurements map[string]float64
	Status       report.Status
}

// ReportContribution is one workload-owned report result.
type (
	FinalSnapshot = ReportContext
	Contribution  = ReportContribution
)

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
	renderer    ReportRenderer
}

var (
	errEmptyReportKind     = inputError("report kind must not be empty")
	errInvalidReportSchema = inputError("report schema must be at least 1")
	errDuplicateReportKind = inputError("duplicate report kind")
)

// ReportDeclarations owns optional independently versioned workload payloads.
type ReportDeclarations struct{ def *Def }

// ReportRenderer writes human output from the same finalized contribution payload.
type ReportRenderer func(io.Writer, json.RawMessage) error

func (r *ReportDeclarations) Render(kind string, renderer ReportRenderer) {
	for index := range r.def.reports {
		if r.def.reports[index].kind == kind {
			r.def.reports[index].renderer = renderer

			return
		}
	}

	invalid("report renderer", inputError("undeclared report %q", kind))
}

func (r *ReportDeclarations) Contribute(kind string, schema int, contributor ReportContributor) {
	kind = strings.TrimSpace(kind)
	if kind == "" {
		invalid("report", errEmptyReportKind)
	}

	if schema < 1 {
		invalid("report", errInvalidReportSchema)
	}

	for _, existing := range r.def.reports {
		if existing.kind == kind {
			invalid("report", fmt.Errorf("%w %q", errDuplicateReportKind, kind))
		}
	}

	r.def.reports = append(r.def.reports, reportDefinition{kind: kind, schema: schema, contributor: contributor})
}

func (r *ReportDeclarations) Metadata(key, value string) {
	if r.def.Execution.root == nil {
		return
	}

	root := r.def.Execution.root
	root.reportMu.Lock()
	defer root.reportMu.Unlock()

	if root.reportData == nil {
		root.reportData = map[string]string{}
	}

	root.reportData[key] = value
}

// Put copies a directly computed payload at publication time.
func (r *ReportDeclarations) Put(kind string, schema int, value any) {
	var data json.RawMessage

	if r.def.Execution.root != nil && r.def.Execution.reporting {
		encoded, err := json.Marshal(value)
		if err != nil {
			invalid("report payload", err)
		}

		data = encoded
	}

	r.Contribute(
		kind,
		schema,
		func(ReportContext) (ReportContribution, error) { return ReportContribution{Data: data}, nil },
	)
}

func newRunReport(
	name string,
	params []resolvedParam,
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
		BuildDigest:    options.BuildDigest,
		StartedAt:      started,
		Status:         report.StatusFailed,
		Workload:       name,
		Drivers:        []report.Driver{},
		Metadata:       maps.Clone(options.Metadata),
		Host: report.Host{
			OS: runtime.GOOS, Arch: runtime.GOARCH, GoVersion: runtime.Version(),
			CPUs: runtime.NumCPU(), MaxProcs: runtime.GOMAXPROCS(0),
		},

		Parameters: reportParameters(params),
		StepSelection: report.StepSelection{
			Only:    slices.Clone(steps),
			Exclude: slices.Clone(noSteps),
		},
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

// AddReportData adds or replaces one JSON value in the common report metadata.
// Values are copied into the final envelope and should contain only non-secret run labels.
func (b *Bench) AddReportData(key, value string) {
	b.root.reportMu.Lock()
	if b.root.reportData == nil {
		b.root.reportData = make(map[string]string)
	}

	b.root.reportData[key] = value
	b.root.reportMu.Unlock()
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
	root *rootState,
	definitions []reportDefinition,
	data metricdata.ResourceMetrics,
	runErr error,
	phase string,
) {
	run.FinishedAt = time.Now().UTC()

	root.reportMu.Lock()
	run.Custom = maps.Clone(root.reportData)
	root.reportMu.Unlock()
	run.Steps = root.stepFilter.snapshot()
	run.Metrics = reportMetrics(data, root.metricsPrefix)
	run.Errors = reportErrors(root.errorReporter.snapshot())

	measurementMetric := run.Metrics["measurement_seconds"]
	if seconds := metricTotal(&measurementMetric); seconds > 0 {
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
	run.WorkloadReports = buildWorkloadReports(definitions, ReportContext{
		Metrics:      snapshots,
		Series:       run.Metrics,
		Steps:        slices.Clone(run.Steps),
		Measurements: maps.Clone(run.Measurements),
		Status:       run.Status,
	})
}

func buildWorkloadReports(definitions []reportDefinition, reportContext ReportContext) []report.WorkloadReport {
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

		contribution, err := definition.contributor(reportContext)
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

	message = message[:maxBytes]
	for !utf8.ValidString(message) {
		message = message[:len(message)-1]
	}

	return message
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
				"p50": histogramQuantile(
					snapshot.Bounds,
					snapshot.Buckets,
					snapshot.Count,
					medianP,
				),
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

func reportAttributes(set attribute.Set) map[string]any {
	values := set.ToSlice()
	if len(values) == 0 {
		return nil
	}

	out := make(map[string]any, len(values))
	for _, value := range values {
		out[string(value.Key)] = value.Value.AsInterface()
	}

	return out
}

func metricTotal(metric *report.Metric) float64 {
	if metric == nil || metric.Total == nil {
		return 0
	}

	return *metric.Total
}
