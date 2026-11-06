package mysql

import (
	"context"
	"errors"
	"strings"

	gomysql "github.com/go-sql-driver/mysql"

	"github.com/stroppy-io/stroppy/v6/pkg/config"
	"github.com/stroppy-io/stroppy/v6/pkg/driver"
)

func (d *Driver) routineDriver(ctx context.Context, isolation config.TxIsolationLevel) (*Driver, error) {
	if isolation == config.TxIsolationLevelUnspecified {
		return d, nil
	}

	level, err := driver.DefaultIsolation(d.options.Config)
	if err != nil {
		return nil, err
	}

	if level == isolation {
		return d, nil
	}

	d.routineMu.Lock()
	defer d.routineMu.Unlock()

	if existing := d.routines[isolation]; existing != nil {
		return existing, nil
	}

	name, err := driver.IsolationSQL(isolation)
	if err != nil {
		return nil, err
	}

	cfg := *d.options.Config
	value := strings.ToLower(strings.ReplaceAll(name, " ", "_"))

	transaction := config.TransactionConfig{}
	if cfg.Transactions != nil {
		transaction = *cfg.Transactions
	}

	transaction.DefaultIsolation = &value
	cfg.Transactions = &transaction
	options := d.options
	options.Config = &cfg

	backend, err := NewDriver(ctx, options)
	if err != nil {
		return nil, err
	}

	if d.routines == nil {
		d.routines = make(map[config.TxIsolationLevel]*Driver)
	}

	d.routines[isolation] = backend

	return backend, nil
}

func (d *Driver) RunRoutine(
	ctx context.Context, isolation config.TxIsolationLevel, query string, args map[string]any,
) (*driver.QueryResult, error) {
	backend, err := d.routineDriver(ctx, isolation)
	if err != nil {
		return nil, &driver.RoutineError{Cause: err}
	}

	result, err := backend.RunQuery(ctx, query, args)
	if result != nil && result.Rows != nil {
		err = driver.JoinErrors(err, result.Rows.Close(), result.Rows.Err())
	}

	if err != nil {
		_, confirmed := errors.AsType[*gomysql.MySQLError](err)

		return result, &driver.RoutineError{Cause: err, RollbackConfirmed: confirmed, Ambiguous: !confirmed}
	}

	return result, nil
}
