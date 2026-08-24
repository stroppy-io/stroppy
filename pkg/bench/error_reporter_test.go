package bench

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"github.com/stroppy-io/stroppy/pkg/driver"
)

func TestErrorReporterSuppressesDuplicatesAndRecordsMetrics(t *testing.T) {
	core, logs := observer.New(zapcore.WarnLevel)
	rootState, err := newRootState(zap.New(core), context.Background(), nil, nil, &MetricsConfig{})
	require.NoError(t, err)
	t.Cleanup(func() {
		rootState.errorReporter.stopAndWait()
		rootState.shutdownMetrics()
	})

	vu := &VU{root: rootState, vuid: 1, ctx: context.Background()}
	classifyDeadlock := func(error) driver.ErrorFacts {
		return driver.ErrorFacts{Kind: driver.ErrorKindDeadlock}
	}
	classifyCustom := func(error) driver.ErrorFacts {
		return driver.ErrorFacts{Kind: driver.ErrorKind("backend-private")}
	}

	for range 3 {
		rootState.errorReporter.record(
			vu, terminalErrorQuery, "q1", errors.New("same backend failure"), classifyDeadlock,
		)
	}

	rootState.errorReporter.record(
		vu, terminalErrorIteration, "iteration", errors.New("different failure"), classifyCustom,
	)
	rootState.errorReporter.recordRetry(vu)

	require.Len(t, logs.FilterMessage("nonfatal error; continuing").All(), 2)
	require.NotContains(t, fmt.Sprint(logs.All()[0].Context), "backend-private")

	snapshot := rootState.errorReporter.snapshot()
	require.Equal(t, uint64(4), snapshot.terminalErrors)
	require.Equal(t, uint64(1), snapshot.failedIterations)
	require.Equal(t, uint64(3), snapshot.failedQueries)
	require.Equal(t, uint64(1), snapshot.retryAttempts)
	require.Len(t, snapshot.groups, 2)
	require.Equal(t, driver.ErrorKindUnknown, snapshot.groups[0].kind)

	var data metricdata.ResourceMetrics
	require.NoError(t, rootState.manualReader.Collect(context.Background(), &data))
	require.InDelta(t, 4, findSum(t, data, rootState.metricsPrefix+"terminal_errors_total"), 0)
	require.InDelta(t, 1, findSum(t, data, rootState.metricsPrefix+"failed_iterations_total"), 0)
	require.InDelta(t, 3, findSum(t, data, rootState.metricsPrefix+"failed_queries_total"), 0)
	require.InDelta(t, 1, findSum(t, data, rootState.metricsPrefix+"retry_attempts_total"), 0)

	for _, scope := range data.ScopeMetrics {
		for _, m := range scope.Metrics {
			if m.Name != rootState.metricsPrefix+"terminal_errors_total" {
				continue
			}

			sum, ok := m.Data.(metricdata.Sum[float64])
			require.True(t, ok)

			for _, point := range sum.DataPoints {
				attrs := point.Attributes.ToSlice()

				for _, attr := range attrs {
					require.NotContains(t, attr.Value.AsString(), "failure")
				}
			}
		}
	}
}

func TestErrorReporterBoundsGroupsAndPeriodicNotices(t *testing.T) {
	core, logs := observer.New(zapcore.WarnLevel)
	reporter := newErrorReporter(zap.New(core), time.Hour)
	t.Cleanup(reporter.stopAndWait)

	for i := range maxErrorGroups + 10 {
		reporter.record(
			nil,
			terminalErrorQuery,
			fmt.Sprintf("query-%d-%s", i, strings.Repeat("x", maxErrorOperationBytes)),
			errors.New("query failed"),
			nil,
		)
	}

	snapshot := reporter.snapshot()
	require.Equal(t, uint64(maxErrorGroups+10), snapshot.terminalErrors)
	require.Len(t, snapshot.groups, maxErrorGroups+1)
	require.Len(t, logs.FilterMessage("nonfatal error; continuing").All(), maxErrorGroups+1)

	for _, group := range snapshot.groups {
		require.LessOrEqual(t, len(group.operation), maxErrorOperationBytes)
	}

	reporter.reportPeriodic()
	require.Len(t, logs.FilterMessage("nonfatal errors continue").All(), 1)

	reporter.reportPeriodic()
	require.Len(t, logs.FilterMessage("nonfatal errors continue").All(), 1)
}

func TestBoundErrorOperationSanitizesAndPreservesRuneBoundaries(t *testing.T) {
	t.Parallel()

	bidi := string(rune(0x202e))
	format := bidi + string(rune(0x200d))
	require.Equal(t, "safe???[31m??end", boundErrorOperation("safe\r\n\x1b[31m\x00"+bidi+"end"))
	require.Equal(t, "left??right", boundErrorOperation("left"+format+"right"))
	require.Equal(t, "a?b", boundErrorOperation(string([]byte{'a', 0xff, 'b'})))

	prefix := strings.Repeat("a", maxErrorOperationBytes-1)
	got := boundErrorOperation(prefix + "é")
	require.Equal(t, prefix, got)
	require.True(t, utf8.ValidString(got))
	require.LessOrEqual(t, len(got), maxErrorOperationBytes)
}

func TestRecordQueryErrorExcludesRunCancellation(t *testing.T) {
	reporter := newErrorReporter(zap.NewNop(), time.Hour)
	t.Cleanup(reporter.stopAndWait)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	rootState := &RootState{errorReporter: reporter, txMetrics: &txMetrics{}}
	b := &Bench{root: rootState, vu: &VU{root: rootState, ctx: ctx}}
	b.RecordQueryError("query", errors.Join(errors.New("backend detail"), context.Canceled))

	snapshot := reporter.snapshot()
	require.Zero(t, snapshot.terminalErrors)
	require.Empty(t, snapshot.groups)
}

func TestErrorSummaryIsProminentAndDeterministic(t *testing.T) {
	rootState, err := newRootState(zap.NewNop(), context.Background(), nil, nil, &MetricsConfig{})
	require.NoError(t, err)
	t.Cleanup(func() {
		rootState.errorReporter.stopAndWait()
		rootState.shutdownMetrics()
	})

	reporter := rootState.errorReporter

	reporter.record(
		nil,
		terminalErrorQuery,
		"q2",
		errors.New("second"),
		func(error) driver.ErrorFacts { return driver.ErrorFacts{Kind: driver.ErrorKindTimeout} },
	)
	reporter.record(
		nil,
		terminalErrorIteration,
		"iteration",
		errors.New("first"),
		func(error) driver.ErrorFacts { return driver.ErrorFacts{Kind: driver.ErrorKindDeadlock} },
	)
	reporter.recordRetry(nil)

	var output bytes.Buffer
	newSummary(rootState).printTo(&output)

	text := output.String()
	require.Contains(t, text, "=== bench completed with errors ===")
	require.Contains(t, text, "terminal_errors_total")
	require.Contains(t, text, "failed_iterations_total")
	require.Contains(t, text, "failed_queries_total")
	require.Contains(t, text, "retry_attempts_total")
	require.Less(t, strings.Index(text, "operation=iteration"), strings.Index(text, "operation=q2"))
	require.NotContains(t, text, "first")
	require.NotContains(t, text, "second")
}
