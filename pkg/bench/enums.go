package bench

import (
	"errors"
	"fmt"

	"github.com/stroppy-io/stroppy/pkg/config"
)

var (
	errUnknownDriverType  = errors.New("unknown driver type")
	errUnknownErrorMode   = errors.New("unknown error mode")
	errUnknownTxIsolation = errors.New("unknown tx isolation")
)

// String-typed enums a Go workload authors with; resolved to the config enums
// the driver layer consumes. Ports helpers.ts string<->enum maps.

type DriverTypeName string

const (
	DriverPostgres DriverTypeName = "postgres"
	DriverMySQL    DriverTypeName = "mysql"
	DriverPicodata DriverTypeName = "picodata"
	DriverYDB      DriverTypeName = "ydb"
	DriverNoop     DriverTypeName = "noop"
	DriverCSV      DriverTypeName = "csv"
)

func ParseDriverType(s string) (config.DriverType, error) {
	switch s {
	case "", "postgres":
		return config.DriverTypePostgres, nil
	case "mysql":
		return config.DriverTypeMySQL, nil
	case "picodata", "pico":
		return config.DriverTypePicodata, nil
	case "ydb":
		return config.DriverTypeYDB, nil
	case "noop":
		return config.DriverTypeNoop, nil
	case "csv":
		return config.DriverTypeCSV, nil
	default:
		return 0, fmt.Errorf("%w %q", errUnknownDriverType, s)
	}
}

// DriverTypeNameOf reverse-maps the driver enum to the authoring string name.
func DriverTypeNameOf(t config.DriverType) DriverTypeName {
	switch t {
	case config.DriverTypePostgres:
		return DriverPostgres
	case config.DriverTypeMySQL:
		return DriverMySQL
	case config.DriverTypePicodata:
		return DriverPicodata
	case config.DriverTypeYDB:
		return DriverYDB
	case config.DriverTypeNoop:
		return DriverNoop
	case config.DriverTypeCSV:
		return DriverCSV
	default:
		return ""
	}
}

type ErrorModeName string

const (
	ErrorSilent ErrorModeName = "silent"
	ErrorLog    ErrorModeName = "log"
	ErrorThrow  ErrorModeName = "throw"
	ErrorFail   ErrorModeName = "fail"
	ErrorAbort  ErrorModeName = "abort"
)

func ParseErrorMode(s string) (config.ErrorMode, error) {
	switch s {
	case "", "silent":
		return config.ErrorModeSilent, nil
	case "log":
		return config.ErrorModeLog, nil
	case "throw":
		return config.ErrorModeThrow, nil
	case "fail":
		return config.ErrorModeFail, nil
	case "abort":
		return config.ErrorModeAbort, nil
	default:
		return 0, fmt.Errorf("%w %q", errUnknownErrorMode, s)
	}
}

type TxIsolationName string

const (
	IsoDBDefault       TxIsolationName = "db_default"
	IsoReadUncommitted TxIsolationName = "read_uncommitted"
	IsoReadCommitted   TxIsolationName = "read_committed"
	IsoRepeatableRead  TxIsolationName = "repeatable_read"
	IsoSerializable    TxIsolationName = "serializable"
	IsoConn            TxIsolationName = "conn"
	IsoNone            TxIsolationName = "none"
)

func ParseTxIsolation(s string) (config.TxIsolationLevel, error) {
	switch s {
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
	case "conn":
		return config.TxIsolationLevelConnectionOnly, nil
	case "none":
		return config.TxIsolationLevelNone, nil
	default:
		return 0, fmt.Errorf("%w %q", errUnknownTxIsolation, s)
	}
}
