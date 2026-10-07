package workload

import (
	"testing"

	"github.com/stroppy-io/stroppy/v6/pkg/bench"
	"github.com/stroppy-io/stroppy/v6/pkg/bench/testkit"
	"github.com/stroppy-io/stroppy/v6/pkg/record"
)

// runOnce executes the workload once against a canned answer for the loaded
// statement and returns how many iterations failed.
func runOnce(t *testing.T, params map[string]string, reply *record.Response) uint64 {
	t.Helper()

	recorder := &record.Recorder{}
	if reply != nil {
		recorder.Reply("SELECT 1", *reply)
	}

	run, err := testkit.Record(t.Context(), Test, recorder, bench.RunOptions{
		Params: bench.ParamInputs{CLI: params},
	})
	if err != nil {
		t.Fatal(err)
	}

	recorded := recorder.Operations()
	if len(recorded) != 1 {
		t.Fatalf("expected one query, recorded %d", len(recorded))
	}

	if recorded[0].SQL != "SELECT 1" {
		t.Fatalf("recorded SQL %q, want the query loaded from %s", recorded[0].SQL, queryFile)
	}

	return run.Errors.FailedIterations
}

func oneRow(value int64) *record.Response {
	return &record.Response{Columns: []string{"?column?"}, Rows: [][]any{{value}}}
}

func TestEmbeddedQuerySatisfiesTheDefaultExpectation(t *testing.T) {
	if failed := runOnce(t, nil, oneRow(1)); failed != 0 {
		t.Fatalf("failed iterations = %d; want 0", failed)
	}
}

func TestExpectedParameterIsChecked(t *testing.T) {
	if failed := runOnce(t, map[string]string{"expected": "1"}, oneRow(1)); failed != 0 {
		t.Fatalf("failed iterations = %d; want 0", failed)
	}

	if failed := runOnce(t, map[string]string{"expected": "2"}, oneRow(1)); failed != 1 {
		t.Fatalf("failed iterations = %d; want 1 — --expected is not checked", failed)
	}
}

func TestLabelReachesTheReportData(t *testing.T) {
	recorder := &record.Recorder{}
	recorder.Reply("SELECT 1", *oneRow(1))

	run, err := testkit.Record(t.Context(), Test, recorder, bench.RunOptions{
		Params: bench.ParamInputs{CLI: map[string]string{"label": "nightly"}},
	})
	if err != nil {
		t.Fatal(err)
	}

	if got := run.Custom["label"]; got != "nightly" {
		t.Fatalf("report label = %q, want %q", got, "nightly")
	}
}
