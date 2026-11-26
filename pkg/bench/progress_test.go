package bench

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	collectorpb "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
	"google.golang.org/protobuf/proto"

	"github.com/stroppy-io/stroppy/v6/pkg/driver/insertprogress"
	"github.com/stroppy-io/stroppy/v6/pkg/report"
)

func progressFixture(t *testing.T, policies ...Policy) (*rootState, *runProgress) {
	t.Helper()
	root, err := newrootState(zap.NewNop(), t.Context(), nil, nil, &MetricsConfig{Quiet: true})
	require.NoError(t, err)
	t.Cleanup(root.shutdownMetrics)

	steps := make([]StepDescription, len(policies))
	p, err := newRunProgress(root, steps)
	require.NoError(t, err)

	root.progress = p

	t.Cleanup(func() {
		if p.stopped.IsZero() {
			p.finish(string(Failed))
		}
	})

	return root, p
}

func progressSnapshot(p *runProgress) float64 {
	p.mu.Lock()
	defer p.mu.Unlock()

	return p.snapshotLocked(time.Now()).Ratio
}

func TestProgressUsesEqualPhasesAndConfirmedRows(t *testing.T) {
	_, p := progressFixture(t, Policy{}, Policy{})
	p.begin("load", Policy{mode: "once", workers: 1})

	total := 2.0
	p.update(0, 0, ProgressUpdate{Activity: "first", Unit: "tables", Total: &total})
	p.insert(0, &insertprogress.Snapshot{Table: "first", TotalRows: 100, GeneratedRows: 100, ConfirmedRows: 10})
	require.InDelta(t, 0.025, progressSnapshot(p), 0.0001)
	p.insert(0, &insertprogress.Snapshot{Table: "first", TotalRows: 100, ConfirmedRows: 100})
	require.InDelta(t, 0.25, progressSnapshot(p), 0.0001)
	p.update(0, 0, ProgressUpdate{Activity: "second", Unit: "tables", Current: 1, Total: &total})
	require.InDelta(t, 0.25, progressSnapshot(p), 0.0001)
	p.end(Completed)
	require.InDelta(t, 0.5, progressSnapshot(p), 0.0001)
	p.begin("opaque", Policy{mode: "once", workers: 1})
	require.InDelta(t, 0.5, progressSnapshot(p), 0.0001)
	p.end(Completed)
	require.Less(t, progressSnapshot(p), 1.0)
	require.InDelta(t, 1.0, p.finish(string(Completed)).Ratio, 0)
}

func TestProgressDrainAndFailureDoNotCompleteRun(t *testing.T) {
	root, p := progressFixture(t, Policy{})
	policy := ConstantWorkers(1, time.Millisecond, DrainTimeout(time.Second))
	p.begin("work", policy)
	require.NoError(t, root.startThroughput("work", 1))
	p.measuring(root.throughput.current.Load())
	p.mu.Lock()
	p.budgetStart = time.Now().Add(-time.Second)
	value := p.snapshotLocked(time.Now())
	p.mu.Unlock()
	require.Equal(t, "draining", value.Status)
	require.Less(t, value.Ratio, 1.0)
	p.end(Failed)
	root.throughput.stop()
	require.Less(t, p.finish(string(Failed)).Ratio, 1.0)
}

func TestProgressMetricsReplaceActivityAndOmitUnknownTotals(t *testing.T) {
	root, p := progressFixture(t, Policy{})
	p.begin("queries", Policy{mode: "once", workers: 2})

	known := 4.0
	p.update(0, 0, ProgressUpdate{Activity: "q1", Unit: "queries", Current: 1, Total: &known})
	p.update(1, 0, ProgressUpdate{Activity: "q2", Unit: "queries", Current: 2})

	collect := func() map[string]metricdata.Metrics {
		var data metricdata.ResourceMetrics
		require.NoError(t, root.manualReader.Collect(t.Context(), &data))

		metrics := make(map[string]metricdata.Metrics)

		for _, scope := range data.ScopeMetrics {
			for _, metric := range scope.Metrics {
				metrics[strings.TrimPrefix(metric.Name, root.metricsPrefix)] = metric
			}
		}

		return metrics
	}
	metrics := collect()
	require.Len(t, metrics["progress_ratio"].Data.(metricdata.Gauge[float64]).DataPoints, 1)
	require.Len(t, metrics["progress_total"].Data.(metricdata.Gauge[float64]).DataPoints, 1)
	converted, ok := reportMetric(metrics["progress_current"])
	require.True(t, ok)
	require.Nil(t, converted.Total)
	p.update(0, 0, ProgressUpdate{Activity: "q3", Unit: "queries", Current: 2})

	metrics = collect()
	_, exists := metrics["progress_total"]
	require.False(t, exists)

	points := metrics["progress_current"].Data.(metricdata.Gauge[float64]).DataPoints
	require.Len(t, points, 2)

	for _, point := range points {
		activity, _ := point.Attributes.Value("activity")
		require.NotEqual(t, "q1", activity.AsString())
		require.False(t, point.Attributes.HasValue("event"))
	}
}

func TestFinishedProgressActivityClockFreezes(t *testing.T) {
	_, p := progressFixture(t, Policy{})
	p.begin("streams", OncePerWorker(2))

	total := 1.0
	p.update(0, 0, ProgressUpdate{Activity: "q1", Unit: "queries", Current: 1, Total: &total})
	p.update(1, 0, ProgressUpdate{Activity: "q2", Unit: "queries", Total: &total})
	p.iterationDone(0)
	p.mu.Lock()
	first := p.snapshotLocked(time.Now())
	later := p.snapshotLocked(time.Now().Add(time.Minute))
	p.mu.Unlock()
	require.False(t, first.Sources[0].Active)
	require.True(t, first.Sources[1].Active)
	require.InDelta(t, first.Sources[0].ElapsedSeconds, later.Sources[0].ElapsedSeconds, 0)
	require.Greater(t, later.Sources[1].ElapsedSeconds, first.Sources[1].ElapsedSeconds)
}

func TestProgressSDKExcludesNonadditiveAggregates(t *testing.T) {
	test := Test{Name: "progress-snapshots", Define: func(d *Def) error {
		d.Report.Contribute("progress.snapshot", 1, func(snapshot ReportContext) (ReportContribution, error) {
			for _, name := range []string{"progress_current", "progress_total", "progress_source_ratio"} {
				require.NotContains(t, snapshot.Metrics, name)
				require.NotEmpty(t, snapshot.Series[name].Series)
			}

			return ReportContribution{Status: report.WorkloadReportOK}, nil
		})
		d.Execution.Step("queries", func(ctx context.Context, b *Bench) error {
			total := 1.0
			b.Progress(ctx, ProgressUpdate{Activity: "q1", Unit: "queries", Current: 1, Total: &total})
			metrics, err := b.CollectedMetrics()
			require.NoError(t, err)

			for _, name := range []string{"progress_current", "progress_total", "progress_source_ratio"} {
				require.NotContains(t, metrics, name)
			}

			return nil
		}, OncePerWorker(2))

		return d.Execution.Err()
	}}
	result, err := RunTest(t.Context(), test, RunOptions{Report: &ReportOptions{}, Metrics: &MetricsConfig{Quiet: true}})
	require.NoError(t, err)
	require.Len(t, result.Metrics["progress_source_ratio"].Series, 3)
}

func TestProgressRetainsFinishedStepSources(t *testing.T) {
	_, p := progressFixture(t, Policy{}, Policy{})
	p.begin("queries", Policy{mode: "once", workers: 1})

	total := 2.0
	p.update(0, 0, ProgressUpdate{Activity: "q2", Unit: "queries", Current: 2, Total: &total})
	p.end(Completed)
	p.begin("cleanup", Policy{mode: "once", workers: 1})
	p.end(Completed)
	value := p.finish(string(Completed))
	require.Len(t, value.Sources, 1)
	require.Equal(t, "queries", value.Sources[0].Step)
	require.Equal(t, "q2", value.Sources[0].Activity)
}

func TestProgressRuntimeFlowIsBoundedAndMonotonic(t *testing.T) {
	_, p := progressFixture(t, Policy{})
	previous := 0.0

	for range 4 {
		p.begin("discovered", Policy{mode: "once", workers: 1})
		current := progressSnapshot(p)
		require.GreaterOrEqual(t, current, previous)
		p.end(Completed)
		previous = progressSnapshot(p)
		require.Less(t, previous, 1.0)
	}

	value := p.finish(string(Canceled))
	require.GreaterOrEqual(t, value.Ratio, previous)
	require.Less(t, value.Ratio, 1.0)
}

func TestProgressPublicAPIReportsNativeWorkAndOutcome(t *testing.T) {
	for name, terminal := range map[string]error{
		"completed": nil, "failed": errors.New("failed"), "canceled": context.Canceled,
	} {
		t.Run(name, func(t *testing.T) {
			core, logs := observer.New(zap.InfoLevel)
			test := Test{Name: "progress", Define: func(d *Def) error {
				d.Execution.Step("queries", func(ctx context.Context, b *Bench) error {
					total := 3.0
					b.Progress(ctx, ProgressUpdate{Activity: "q1", Unit: "queries", Current: 1, Total: &total})

					return terminal
				})

				return d.Execution.Err()
			}}
			result, err := RunTest(t.Context(), test, RunOptions{
				Logger: Logger{backend: zap.New(core)}, Report: &ReportOptions{},
				Metrics: &MetricsConfig{SummaryWriter: io.Discard},
			})
			require.ErrorIs(t, err, terminal)
			require.NotNil(t, result.Progress)
			require.InDelta(t, result.Progress.Ratio, *result.Metrics["progress_ratio"].Total, 0)

			if terminal == nil {
				require.InDelta(t, 1.0, result.Progress.Ratio, 0)
			} else {
				require.Less(t, result.Progress.Ratio, 1.0)
			}

			require.NotEmpty(t, logs.FilterMessage("run progress").All())
			require.Equal(t, "q1", result.Progress.Sources[0].Activity)
		})
	}
}

func TestProgressOTLPReportAndCaptureAgree(t *testing.T) {
	received := make(chan *collectorpb.ExportMetricsServiceRequest, 8)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		data, err := io.ReadAll(request.Body)

		var message collectorpb.ExportMetricsServiceRequest
		if err != nil || proto.Unmarshal(data, &message) != nil {
			w.WriteHeader(http.StatusBadRequest)

			return
		}

		received <- &message

		w.Header().Set("Content-Type", "application/x-protobuf")
	}))
	defer server.Close()

	test := Test{Name: "progress-export", Define: func(d *Def) error {
		d.Execution.Step("queries", func(ctx context.Context, b *Bench) error {
			total := 2.0
			b.Progress(ctx, ProgressUpdate{Activity: "q2", Unit: "queries", Current: 2, Total: &total})

			return nil
		})

		return d.Execution.Err()
	}}
	result, err := RunTest(t.Context(), test, RunOptions{
		Report: &ReportOptions{}, Metrics: &MetricsConfig{
			Quiet:        true,
			HTTPEndpoint: strings.TrimPrefix(server.URL, "http://"), Insecure: true,
		},
		Capture: &CaptureOptions{Directory: t.TempDir(), Metrics: true, Interval: time.Hour},
	})
	require.NoError(t, err)
	require.InDelta(t, 1.0, *result.Metrics["progress_ratio"].Total, 0)

	select {
	case request := <-received:
		found := false

		for _, resource := range request.ResourceMetrics {
			for _, scope := range resource.ScopeMetrics {
				for _, metric := range scope.Metrics {
					if metric.Name == "stroppy_progress_ratio" {
						points := metric.GetGauge().DataPoints
						require.Len(t, points, 1)
						require.InDelta(t, result.Progress.Ratio, points[0].GetAsDouble(), 0)

						found = true
					}
				}
			}
		}

		require.True(t, found)
	case <-time.After(time.Second):
		t.Fatal("progress not exported")
	}

	file, err := report.OpenCaptureFile(".", result.Capture, "metrics.jsonl")
	require.NoError(t, err)

	defer file.Close()

	decoder := json.NewDecoder(file)

	var last report.MetricSample

	for {
		var sample report.MetricSample
		if err := decoder.Decode(&sample); err != nil {
			require.ErrorIs(t, err, io.EOF)

			break
		}

		last = sample
	}

	require.InDelta(t, result.Progress.Ratio, *last.Metrics["progress_ratio"].Total, 0)
}

func TestProgressInvalidValues(t *testing.T) {
	test := Test{Name: "invalid-progress", Define: func(d *Def) error {
		d.Execution.Step("work", func(ctx context.Context, b *Bench) error {
			b.Progress(ctx, ProgressUpdate{Activity: "SELECT secret FROM t", Unit: "queries"})

			return nil
		})

		return d.Execution.Err()
	}}
	_, err := RunTest(t.Context(), test, RunOptions{Metrics: &MetricsConfig{Quiet: true}})
	require.ErrorContains(t, err, "progress")
}
