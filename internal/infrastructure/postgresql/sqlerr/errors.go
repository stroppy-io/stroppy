package sqlerr

import (
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
)

const (
	sqlBuildStageErr = "sql build stage: "
	sqlExecStageErr  = "sql exec stage: "
	sqlScanStageErr  = "sql sqlscan stage: "
)

type ErroredRow struct {
	BuilderErr error
}

func (b *ErroredRow) Scan(...any) error {
	return b.BuilderErr
}

type executorError struct {
	err   error
	stage string
}

func (e *executorError) Error() string {
	return fmt.Sprintf("%s %s", e.stage, e.err)
}

func (e *executorError) Unwrap() error {
	return e.err
}

func SqlBuildErr(err error) error {
	return &executorError{
		err:   err,
		stage: sqlBuildStageErr,
	}
}

func SqlExecErr(err error) error {
	return &executorError{
		err:   err,
		stage: sqlExecStageErr,
	}
}

func SqlScanErr(err error) error {
	return &executorError{
		err:   err,
		stage: sqlScanStageErr,
	}
}

func IsPgxErr(err, target error) bool {
	var cast *executorError
	ok := errors.As(err, &cast)
	if ok {
		return errors.Is(cast.err, target)
	}

	return errors.Is(err, target)
}

func IsNotFound(err error) bool {
	if errors.Is(err, pgx.ErrNoRows) {
		return true
	}
	return IsPgxErr(err, pgx.ErrNoRows)
}

func IsConstraintError(err error) bool {
	return IsUniqueConstraintError(err) || IsForeignKeyConstraintError(err)
}

// IsUniqueConstraintError reports if the error resulted from a DB uniqueness constraint violation.
// e.g. duplicate value in unique index.
func IsUniqueConstraintError(err error) bool {
	if err == nil {
		return false
	}
	for _, s := range []string{
		"violates unique constraint", // Postgres
		"UNIQUE constraint failed",   // SQLite
	} {
		if strings.Contains(err.Error(), s) {
			return true
		}
	}
	return false
}

// IsForeignKeyConstraintError reports if the error resulted from a database foreign-key constraint violation.
// e.g. parent row does not exist.
func IsForeignKeyConstraintError(err error) bool {
	if err == nil {
		return false
	}
	for _, s := range []string{
		"violates foreign key constraint", // Postgres
		"FOREIGN KEY constraint failed",   // SQLite
	} {
		if strings.Contains(err.Error(), s) {
			return true
		}
	}
	return false
}
