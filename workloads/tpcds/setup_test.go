package tpcds

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stroppy-io/stroppy/v6/pkg/bench"
	"github.com/stroppy-io/stroppy/v6/pkg/bench/testkit"
	"github.com/stroppy-io/stroppy/v6/pkg/record"
)

func TestSetupErrorIdentifiesStatement(t *testing.T) {
	backend := errors.New("Query compilation timed out")
	recorder := &record.Recorder{}
	recorder.Reply("SELECT 2;", record.Response{Err: backend})

	test := bench.Test{Name: "setup-error", Define: func(d *bench.Def) error {
		w := &workload{schemaSQL: bench.ParseSQL("--+ drop_schema\n--= first\nSELECT 1;\n--= failing_drop\nSELECT 2;")}

		d.Execution.Step("drop_schema", func(ctx context.Context, b *bench.Bench) error {
			return w.runSection(ctx, b, w.schemaSQL, "drop_schema")
		})

		return d.Execution.Err()
	}}

	_, err := testkit.Record(t.Context(), test, recorder, bench.RunOptions{})
	if !errors.Is(err, backend) || !strings.Contains(err.Error(), `statement 2 ("failing_drop")`) ||
		!strings.Contains(err.Error(), "driver recording, elapsed") {
		t.Fatalf("setup error lost identity or cause: %v", err)
	}
}
