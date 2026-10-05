package authoring

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/stroppy-io/stroppy/v6/pkg/bench"
	"github.com/stroppy-io/stroppy/v6/pkg/gen"
	"github.com/stroppy-io/stroppy/v6/pkg/report"
)

// Accounts creates its own table, loads rows, then transfers balances atomically.
var (
	Accounts           = bench.Test{Name: "example/accounts", Define: defineAccounts}
	errAccountContents = errors.New("account contents changed")
)

const (
	minimumAccounts       = 2
	defaultAccounts       = 100
	initialBalance        = 1000
	transferAttempts      = 3
	accountCleanupTimeout = 5 * time.Second
)

type account struct {
	ID      int64 `db:"id"`
	Balance int64 `db:"balance"`
}

type accountWork struct {
	rows        int64
	loadWorkers int
	transfers   *bench.CounterHandle
}

func defineAccounts(d *bench.Def) error {
	run := bench.RunParameters(&d.Param, bench.RunDefaults{Iterations: exampleIterations})
	rows, _ := d.Param.Int64("rows", defaultAccounts, "Accounts to load.", bench.Min(int64(minimumAccounts)))
	workers, _ := d.Param.Int("load-workers", 1, "Parallel load workers.", bench.Min(1))
	work := accountWork{rows, workers, d.Metrics.Counter("account_transfers")}
	d.Report.Contribute("example.accounts", 1, work.report)
	d.Execution.Step("create", createAccounts)
	d.Execution.Step("load", work.load)
	d.Execution.Step("transfer", work.transfer, run.Policy())
	d.Execution.Step("verify", work.verify)
	d.Execution.Step("drop", dropAccounts, bench.Always(accountCleanupTimeout))

	return d.Execution.Err()
}

func createAccounts(ctx context.Context, b *bench.Bench) error {
	return b.Exec(ctx, "CREATE TABLE stroppy_example_accounts (id BIGINT PRIMARY KEY, balance BIGINT NOT NULL)", nil)
}

func dropAccounts(ctx context.Context, b *bench.Bench) error {
	return b.Exec(ctx, "DROP TABLE IF EXISTS stroppy_example_accounts", nil)
}

func accountRow(index uint64) (account, error) {
	//nolint:gosec // index is bounded by the positive int64 source row count.
	return account{ID: int64(index) + 1, Balance: initialBalance}, nil
}

func (w *accountWork) load(ctx context.Context, b *bench.Bench) error {
	_, err := b.Insert(ctx, "stroppy_example_accounts", gen.FromRows(w.rows, accountRow),
		bench.InsertMethod(bench.InsertPlainBulk), bench.LoadWorkers(w.loadWorkers))

	return err
}

func (w *accountWork) transfer(ctx context.Context, b *bench.Bench) error {
	//nolint:gosec // rows is positive and the remainder fits within its int64 range.
	from := int64(b.Iteration()%uint64(w.rows)) + 1
	to := from%w.rows + 1
	operation := transfer{from, to}

	err := b.Transaction(ctx, bench.TransactionOptions{
		Name: "transfer", Isolation: bench.IsoReadCommitted,
		Retry: bench.RetryOptions{MaxAttempts: transferAttempts},
	}, operation.run)
	if err == nil {
		w.transfers.Add(ctx, 1)
	}

	return err
}

type transfer struct{ from, to int64 }

func (t transfer) run(ctx context.Context, tx *bench.Tx) error {
	if err := tx.Exec(ctx, "UPDATE stroppy_example_accounts SET balance = balance - 1 WHERE id = :id",
		map[string]any{"id": t.from}); err != nil {
		return err
	}

	return tx.Exec(ctx, "UPDATE stroppy_example_accounts SET balance = balance + 1 WHERE id = :id",
		map[string]any{"id": t.to})
}

func (w *accountWork) report(final bench.ReportContext) (bench.ReportContribution, error) {
	return bench.ReportContribution{
		Status: report.WorkloadReportOK,
		Data: struct {
			Rows               int64   `json:"rows"`
			Transfers          float64 `json:"transfers"`
			MeasurementSeconds float64 `json:"measurement_seconds"`
		}{w.rows, final.Metrics["account_transfers"].Total, final.Measurements["transfer"]},
	}, nil
}

func (w *accountWork) verify(ctx context.Context, b *bench.Bench) error {
	// Noop exercises generation and transaction paths without database contents.
	if b.DriverTypeName() == bench.DriverNoop {
		return nil
	}

	values, err := b.QueryValues[account](ctx, "SELECT id, balance FROM stroppy_example_accounts", nil)
	if err != nil {
		return err
	}

	var total int64
	for _, value := range values {
		total += value.Balance
	}

	if int64(len(values)) != w.rows || total != w.rows*initialBalance {
		return fmt.Errorf("%w: rows=%d balance=%d", errAccountContents, len(values), total)
	}

	_, err = b.QueryValue[account](ctx, "SELECT id, balance FROM stroppy_example_accounts WHERE id = :id",
		map[string]any{"id": int64(1)})

	return err
}
