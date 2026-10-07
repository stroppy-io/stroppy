package workload

import (
	"testing"

	"github.com/stroppy-io/stroppy/v6/pkg/bench"
	"github.com/stroppy-io/stroppy/v6/pkg/bench/testkit"
	"github.com/stroppy-io/stroppy/v6/pkg/record"
)

// runOnce executes the test once against a canned answer and returns how many
// iterations failed. A nil reply leaves the statement unanswered.
func runOnce(t *testing.T, reply *record.Response) uint64 {
	t.Helper()

	recorder := &record.Recorder{}
	if reply != nil {
		recorder.Reply(selectOne, *reply)
	}

	run, err := testkit.Record(t.Context(), Test, recorder, bench.RunOptions{
		Params: bench.ParamInputs{CLI: map[string]string{"iterations": "1"}},
	})
	if err != nil {
		t.Fatal(err)
	}

	recorded := recorder.Operations()
	if len(recorded) != 1 {
		t.Fatalf("expected one query, recorded %d", len(recorded))
	}

	if recorded[0].SQL != selectOne {
		t.Fatalf("recorded SQL %q, want %q", recorded[0].SQL, selectOne)
	}

	return run.Errors.FailedIterations
}

func TestSelectOneAcceptsTheAnswer(t *testing.T) {
	if failed := runOnce(t, &record.Response{
		Columns: []string{"?column?"}, Rows: [][]any{{int64(1)}},
	}); failed != 0 {
		t.Fatalf("failed iterations = %d; want 0", failed)
	}
}

func TestSelectOneRejectsAWrongAnswer(t *testing.T) {
	if failed := runOnce(t, &record.Response{
		Columns: []string{"?column?"}, Rows: [][]any{{int64(2)}},
	}); failed != 1 {
		t.Fatalf("failed iterations = %d; want 1 — the answer check is not live", failed)
	}
}

func TestSelectOneRejectsNoAnswer(t *testing.T) {
	if failed := runOnce(t, nil); failed != 1 {
		t.Fatalf("failed iterations = %d; want 1 — a missing row must fail the iteration", failed)
	}
}
