package pool

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/stroppy-io/stroppy/v6/pkg/config"
	"github.com/stroppy-io/stroppy/v6/pkg/driver"
)

type routineIsolationKey struct{}

type routineIsolation struct {
	original string
	current  string
}

const routineIsolationData = "stroppy.routineIsolation"

func WithRoutineIsolation(ctx context.Context, isolation config.TxIsolationLevel) context.Context {
	return context.WithValue(ctx, routineIsolationKey{}, isolation)
}

func prepareRoutineConnection(ctx context.Context, conn *pgx.Conn) (bool, error) {
	// PrepareConn owns this physical connection exclusively; no shared cache lock is needed.
	state, _ := conn.PgConn().CustomData()[routineIsolationData].(*routineIsolation)

	isolation, routine := ctx.Value(routineIsolationKey{}).(config.TxIsolationLevel)
	if !routine {
		if state == nil {
			return true, nil
		}

		if err := setRoutineIsolation(ctx, conn, state, state.original); err != nil {
			return false, err
		}

		delete(conn.PgConn().CustomData(), routineIsolationData)

		return true, nil
	}

	var requested string

	if isolation != config.TxIsolationLevelUnspecified {
		name, err := driver.IsolationSQL(isolation)
		if err != nil {
			return true, err
		}

		requested = strings.ToLower(name)
	}

	if state == nil {
		state = &routineIsolation{}

		err := conn.QueryRow(ctx, "SHOW default_transaction_isolation", pgx.QueryExecModeExec).Scan(&state.original)
		if err != nil {
			return false, err
		}

		state.current = state.original
		conn.PgConn().CustomData()[routineIsolationData] = state
	}

	if requested == "" {
		requested = state.original
	}

	if err := setRoutineIsolation(ctx, conn, state, requested); err != nil {
		return false, err
	}

	return true, nil
}

func setRoutineIsolation(ctx context.Context, conn *pgx.Conn, state *routineIsolation, requested string) error {
	if state.current == requested {
		return nil
	}

	_, err := conn.Exec(ctx, "SELECT set_config('default_transaction_isolation', $1, false)",
		pgx.QueryExecModeExec, requested)
	if err != nil {
		return err
	}

	state.current = requested

	return nil
}
