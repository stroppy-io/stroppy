package banking

import (
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/stroppy-io/stroppy/v6/pkg/bench"
	"github.com/stroppy-io/stroppy/v6/pkg/gen"
	"github.com/stroppy-io/stroppy/v6/workloads/internal/workloadtest"
)

// The transfer section and its named queries are what the workload requires at
// runtime (w.sql.Require), so the asset contract pins the same names.
const (
	transferSection = "workload_tx_transfer"
	verifySection   = "verify"
)

func TestEmbeddedAssetContract(t *testing.T) {
	workloadtest.Files(t, files, "README.md", "pg.sql", "mysql.sql", "pico.sql", "ydb.sql")

	// Sections every dialect carries, and the named queries the workload requires
	// of whichever one runs.
	sections := []string{"drop_schema", "create_schema", transferSection, verifySection}
	queries := []workloadtest.Query{
		{Section: transferSection, Name: "read_account"},
		{Section: transferSection, Name: "read_token"},
		{Section: transferSection, Name: "rotate_token"},
		{Section: transferSection, Name: "move_balance"},
		{Section: verifySection, Name: "total_balance"},
		{Section: verifySection, Name: "token_stats"},
	}

	for _, dialect := range []string{"pg.sql", "mysql.sql", "pico.sql", "ydb.sql"} {
		t.Run(dialect, func(t *testing.T) {
			workloadtest.SQL(t, files, dialect, sections, queries)
		})
	}

	// The fault injector is postgres-only, so only its asset carries those.
	workloadtest.SQL(t, files, "pg.sql",
		[]string{"setup_injection", "injection"},
		[]workloadtest.Query{
			{Section: "setup_injection", Name: "create_macavity"},
			{Section: "injection", Name: "arm"},
		},
	)
}

// TestInjectVariantDescribesItsOwnKnobs holds the injecting variant to the two
// things it promises: the fault parameters, and a setup step that fails early
// when the injector is missing.
func TestInjectVariantDescribesItsOwnKnobs(t *testing.T) {
	description, err := bench.DescribeTest(Inject)
	if err != nil {
		t.Fatalf("DescribeTest(banking/inject): %v", err)
	}

	params := make(map[string]bool, len(description.Params))
	for _, param := range description.Params {
		params[param.Name] = true
	}

	for _, want := range []string{"inject-point", "inject-action", "inject-occurrence", "inject-every"} {
		if !params[want] {
			t.Errorf("banking/inject is missing --%s", want)
		}
	}

	steps := make(map[string]bool, len(description.Steps))
	for _, step := range description.Steps {
		steps[step.Name] = true
	}

	if !steps["setup_injection"] {
		t.Error("banking/inject has no setup_injection step, so a missing injector would only surface mid-run")
	}

	if !steps["verify"] {
		t.Error("banking/inject does not verify the invariants")
	}

	// The plain variant must not carry any of that: it is the portable one.
	plain, err := bench.DescribeTest(Test)
	if err != nil {
		t.Fatalf("DescribeTest(banking): %v", err)
	}

	for _, param := range plain.Params {
		if strings.HasPrefix(param.Name, "inject-") {
			t.Errorf("the portable variant declares --%s", param.Name)
		}
	}

	for _, step := range plain.Steps {
		if step.Name == "setup_injection" {
			t.Error("the portable variant has a setup_injection step")
		}
	}
}

func TestDriverDefaults(t *testing.T) {
	for _, tt := range []struct {
		driver bench.DriverTypeName
		want   string
	}{
		{bench.DriverPostgres, "pg.sql"},
		{bench.DriverMySQL, "mysql.sql"},
		{bench.DriverPicodata, "pico.sql"},
		{bench.DriverYDB, "ydb.sql"},
	} {
		if got := sqlFileFor(tt.driver); got != tt.want {
			t.Errorf("sqlFileFor(%s) = %s, want %s", tt.driver, got, tt.want)
		}
	}

	for _, tt := range []struct {
		driver bench.DriverTypeName
		want   bench.TxIsolationName
	}{
		{bench.DriverPostgres, bench.IsoReadCommitted},
		{bench.DriverMySQL, bench.IsoReadCommitted},
		{bench.DriverPicodata, bench.IsoNone},
		{bench.DriverYDB, bench.IsoSerializable},
	} {
		if got := resolveIsolation(tt.driver, ""); got != tt.want {
			t.Errorf("resolveIsolation(%s) = %s, want %s", tt.driver, got, tt.want)
		}
	}

	if got := resolveIsolation(bench.DriverPostgres, bench.IsoSerializable); got != bench.IsoSerializable {
		t.Errorf("explicit isolation override = %s, want %s", got, bench.IsoSerializable)
	}
}

// TestAccountRowsStartAsTheInitialPermutation pins the state the invariants are
// stated against: every account begins with the initial balance and a token, and
// the tokens are 1..accounts — a permutation the transfer step may rearrange but
// never lose or duplicate.
func TestAccountRowsStartAsTheInitialPermutation(t *testing.T) {
	const accounts = 5

	source := accountRows(accounts)
	columns := source.Schema().ColumnNames()

	index := make(map[string]int, len(columns))
	for position, name := range columns {
		index[name] = position
	}

	for _, required := range []string{"id", "balance", "token"} {
		if _, ok := index[required]; !ok {
			t.Fatalf("generated rows have no %q column (columns: %v)", required, columns)
		}
	}

	rows := drain(t, source)
	if len(rows) != accounts {
		t.Fatalf("generated %d rows, want %d", len(rows), accounts)
	}

	for want := int64(1); want <= accounts; want++ {
		row := rows[want-1]

		if got := mustInt64(t, row[index["id"]], "id"); got != want {
			t.Errorf("row %d id = %d, want %d", want-1, got, want)
		}

		if got := mustInt64(t, row[index["balance"]], "balance"); got != initialBalance {
			t.Errorf("row %d balance = %d, want %d", want-1, got, initialBalance)
		}

		if got := mustInt64(t, row[index["token"]], "token"); got != want {
			t.Errorf("row %d token = %d, want %d", want-1, got, want)
		}
	}
}

// drain materializes a batch source into positional rows, in entity order.
func drain(t *testing.T, source gen.BatchSource) [][]any {
	t.Helper()

	columns := source.Schema().ColumnNames()

	cursor, err := source.Prepare(0, -1, 64)
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}

	var rows [][]any

	scratch := make([]any, len(columns))

	for {
		batch, err := cursor.Next()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}

			t.Fatalf("Next: %v", err)
		}

		for i := range batch.Len() {
			batch.MaterializeRow(i, scratch)

			row := make([]any, len(columns))
			copy(row, scratch)
			rows = append(rows, row)
		}
	}

	return rows
}

func mustInt64(t *testing.T, value any, name string) int64 {
	t.Helper()

	number, ok := value.(int64)
	if !ok {
		t.Fatalf("%s = %v, want int64", name, value)
	}

	return number
}
