package authoring

import (
	"context"
	"errors"
	"fmt"

	"github.com/stroppy-io/stroppy/v6/pkg/bench"
)

var (
	Prepared           = bench.Test{Name: "example/prepared", Define: definePrepared}
	errPreparedCounter = errors.New("prepared counter contents changed")
)

const (
	preparedWorkers    = 4
	preparedIterations = 100
)

func definePrepared(d *bench.Def) error {
	run := bench.RunParameters(&d.Param, bench.RunDefaults{Workers: preparedWorkers, Iterations: preparedIterations})
	created := false

	d.Execution.Step("create", func(ctx context.Context, b *bench.Bench) error {
		err := b.Exec(ctx, `CREATE TABLE stroppy_example_prepared (
			worker_id BIGINT PRIMARY KEY, counter_value BIGINT NOT NULL
		)`, nil)
		created = err == nil

		return err
	})
	d.Execution.WorkerStep("increment", initializePreparedCounter, run.Policy())
	d.Execution.Step("drop", func(ctx context.Context, b *bench.Bench) error {
		if !created {
			return nil
		}

		return b.Exec(ctx, "DROP TABLE stroppy_example_prepared", nil)
	}, bench.Always(accountCleanupTimeout))

	return d.Execution.Err()
}

func initializePreparedCounter(ctx context.Context, b *bench.Bench) (bench.Action, error) {
	session, err := b.Session(ctx)
	if err != nil {
		return nil, err
	}

	workerID := int64(b.Worker())
	if err := session.Exec(ctx, `INSERT INTO stroppy_example_prepared (worker_id, counter_value)
		VALUES (:worker_id, 0)`, map[string]any{"worker_id": workerID}); err != nil {
		return nil, err
	}

	increment, err := session.Prepare(ctx, bench.QueryHandle{
		Name: "counter/increment",
		Text: `UPDATE stroppy_example_prepared SET counter_value = counter_value + 1 WHERE worker_id = :worker_id`,
	})
	if err != nil {
		return nil, err
	}

	read, err := session.Prepare(ctx, bench.QueryHandle{
		Name: "counter/read",
		Text: "SELECT counter_value FROM stroppy_example_prepared WHERE worker_id = :worker_id",
	})
	if err != nil {
		return nil, err
	}

	var committed int64

	args := map[string]any{"worker_id": workerID}

	return func(ctx context.Context, b *bench.Bench) error {
		err := b.LogicalOperation(func() error {
			return session.BeginTx(ctx, bench.BeginOpts{
				Name: "increment", Isolation: bench.IsoReadCommitted,
			}, func(tx *bench.Tx) error {
				if err := tx.ExecPrepared(ctx, increment, args); err != nil {
					return err
				}

				actual, err := tx.QueryValuePrepared[int64](ctx, read, args)
				if err != nil {
					return err
				}

				if actual != committed+1 {
					return fmt.Errorf("%w: worker %d: got %d, expected %d", errPreparedCounter, workerID, actual, committed+1)
				}

				return nil
			})
		})
		if err != nil {
			return bench.Fatal(err)
		}

		committed++

		return nil
	}, nil
}
