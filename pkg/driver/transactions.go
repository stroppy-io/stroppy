package driver

import (
	"context"
	"errors"
	"fmt"

	"github.com/stroppy-io/stroppy/v6/pkg/config"
)

var ErrTransactionIsolation = errors.New("unsupported transaction isolation")

// RoutineExecutor executes one self-contained atomic routine invocation.
type RoutineExecutor interface {
	RunRoutine(ctx context.Context, isolation config.TxIsolationLevel,
		query string, args map[string]any) (*QueryResult, error)
}

// RoutineError distinguishes a server-confirmed rollback from a lost commit acknowledgement.
type RoutineError struct {
	Cause             error
	RollbackConfirmed bool
	Ambiguous         bool
}

func (e *RoutineError) Error() string { return e.Cause.Error() }
func (e *RoutineError) Unwrap() error { return e.Cause }

func DefaultIsolation(cfg *config.DriverConfig) (config.TxIsolationLevel, error) {
	if cfg == nil || cfg.Transactions == nil || cfg.Transactions.DefaultIsolation == nil {
		return config.TxIsolationLevelUnspecified, nil
	}

	level, err := ParseIsolation(*cfg.Transactions.DefaultIsolation)
	if err != nil {
		return 0, err
	}

	if cfg.DriverType == config.DriverTypeYDB && level != config.TxIsolationLevelUnspecified &&
		level != config.TxIsolationLevelSerializable {
		return 0, fmt.Errorf("%w: YDB requires serializable", ErrTransactionIsolation)
	}

	return level, nil
}

func ParseIsolation(value string) (config.TxIsolationLevel, error) {
	switch value {
	case "", "db_default":
		return config.TxIsolationLevelUnspecified, nil
	case "read_uncommitted":
		return config.TxIsolationLevelReadUncommitted, nil
	case "read_committed":
		return config.TxIsolationLevelReadCommitted, nil
	case "repeatable_read":
		return config.TxIsolationLevelRepeatableRead, nil
	case "serializable":
		return config.TxIsolationLevelSerializable, nil
	default:
		return 0, fmt.Errorf("%w: %q", ErrTransactionIsolation, value)
	}
}

func FoldBegin(cfg *config.DriverConfig) bool {
	return cfg == nil || cfg.Transactions == nil || cfg.Transactions.FoldBegin == nil || *cfg.Transactions.FoldBegin
}

func IsolationSQL(level config.TxIsolationLevel) (string, error) {
	switch level {
	case config.TxIsolationLevelReadUncommitted:
		return "READ UNCOMMITTED", nil
	case config.TxIsolationLevelReadCommitted:
		return "READ COMMITTED", nil
	case config.TxIsolationLevelRepeatableRead:
		return "REPEATABLE READ", nil
	case config.TxIsolationLevelSerializable:
		return "SERIALIZABLE", nil
	default:
		return "", fmt.Errorf("%w: %d", ErrTransactionIsolation, level)
	}
}
