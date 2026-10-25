package banking

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"strings"
	"testing"
	"time"

	"github.com/stroppy-io/stroppy/v6/pkg/bench"
	"github.com/stroppy-io/stroppy/v6/pkg/driver"
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

	// The three dialects that carry the full witness. Picodata is deliberately not
	// in this list: TestPicodataAssetCarriesNoWitness holds it to its reduced set.
	for _, dialect := range []string{"pg.sql", "mysql.sql", "ydb.sql"} {
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

// TestPicodataAssetCarriesNoWitness pins the deliberate difference: Picodata cannot
// express the token's uniqueness (unique indexes are limited to sharding-key
// columns) and cannot make a cross-shard transfer atomic, so its asset must carry
// the reduced transfer and the total-balance check instead — and must not carry the
// queries that would imply a guarantee the engine cannot give.
func TestPicodataAssetCarriesNoWitness(t *testing.T) {
	workloadtest.SQL(t, files, "pico.sql",
		[]string{"drop_schema", "create_schema", transferSection, verifySection},
		[]workloadtest.Query{
			{Section: transferSection, Name: "move_balance"},
			{Section: verifySection, Name: "total_balance"},
		},
	)

	parsed, err := fs.ReadFile(files, "pico.sql")
	if err != nil {
		t.Fatal(err)
	}

	// The reduced dialect must not mention the witness queries at all: a reader
	// should see the limit in the asset, not a query that silently never runs.
	for _, forbidden := range []string{"rotate_token", "token_stats", "read_token"} {
		if strings.Contains(string(parsed), forbidden+"\n") {
			t.Errorf("pico.sql declares %s, which the driver cannot honor", forbidden)
		}
	}
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
		{bench.DriverTiDB, "mysql.sql"},
		{bench.DriverCRDB, "pg.sql"},
		{bench.DriverYugabyte, "pg.sql"},
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

// verifyPolicy mirrors the policy verify builds, without a bench whose driver it
// would take the classifier from; a nil classifier leaves errors unknown, as a driver
// that could not interpret them would.
func verifyPolicy(attempts int, classify func(error) driver.ErrorFacts) bench.RetryPolicy {
	return bench.RetryPolicy{MaxAttempts: attempts, Actions: waitingActions(), Classify: classify}
}

// TestVerifyWaitsOutARecoveringServer pins the behavior the crash variant needs:
// the check must not give up on the first refusal, because that variant leaves the
// cluster reinitializing, and must still report a real violation.
func TestVerifyWaitsOutARecoveringServer(t *testing.T) {
	attempts := 0

	err := retryCheck(t.Context(), verifyPolicy(5, nil), 0, func() error {
		attempts++

		if attempts < 3 {
			return errors.New("the database system is in recovery mode")
		}

		return nil
	})
	if err != nil {
		t.Fatalf("retryCheck = %v, want nil once the server answered", err)
	}

	if attempts != 3 {
		t.Errorf("checked %d time(s), want 3", attempts)
	}

	// A violation is reported at once, not after the attempts are spent: a read that
	// succeeded and showed broken data is evidence, and retrying it would let a later
	// read erase the failure.
	calls := 0

	err = retryCheck(t.Context(), verifyPolicy(4, nil), 0, func() error {
		calls++

		return fmt.Errorf("%w is 49999, want 50000", errBalanceMismatch)
	})
	if !errors.Is(err, errBalanceMismatch) {
		t.Errorf("retryCheck = %v, want the violation reported", err)
	}

	if calls != 1 {
		t.Errorf("checked %d time(s), want 1 — a violation is not retried", calls)
	}
}

// TestVerifyKeepsAViolationItHasSeen is the case the retry used to erase: a check
// that observes a violation and then, on a later attempt, sees healthy data. The
// run must fail: the first read proved the invariant was broken at that moment, and
// a subsequent read cannot unprove it.
func TestVerifyKeepsAViolationItHasSeen(t *testing.T) {
	calls := 0

	err := retryCheck(t.Context(), verifyPolicy(5, nil), 0, func() error {
		calls++

		if calls == 1 {
			return fmt.Errorf("%w: tokens are not the permutation", errTokenPermutation)
		}

		return nil
	})
	if !errors.Is(err, errTokenPermutation) {
		t.Fatalf("retryCheck = %v, want the violation it saw on the first attempt", err)
	}

	if calls != 1 {
		t.Errorf("checked %d time(s), want 1", calls)
	}
}

// TestVerifyReportsANamedFailureAtOnce is the shape the hand-written violation list
// could not reach: a failure that is not a violation, and that no further attempt can
// answer differently, because the driver said what it was.
func TestVerifyReportsANamedFailureAtOnce(t *testing.T) {
	calls := 0

	refused := errors.New("query is not supported by this driver")

	err := retryCheck(t.Context(), verifyPolicy(5, func(error) driver.ErrorFacts {
		return driver.ErrorFacts{Kind: driver.ErrorKindUnsupported}
	}), 0, func() error {
		calls++

		return refused
	})
	if !errors.Is(err, refused) {
		t.Fatalf("retryCheck = %v, want the failure the driver named", err)
	}

	if calls != 1 {
		t.Errorf("checked %d time(s), want 1 — a named failure is not waited out", calls)
	}
}

// TestVerifyReportsACanceledRun covers the way out of the wait: attempts are the
// bound, but a canceled context ends it at once instead of being read as a check that
// never got an answer.
func TestVerifyReportsACanceledRun(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	calls := 0

	err := retryCheck(ctx, verifyPolicy(5, nil), time.Hour, func() error {
		calls++

		return errors.New("no answer")
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("retryCheck = %v, want %v", err, context.Canceled)
	}

	if calls != 1 {
		t.Errorf("checked %d time(s), want 1", calls)
	}
}
