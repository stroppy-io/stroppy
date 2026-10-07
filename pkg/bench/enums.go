package bench

import (
	"fmt"

	"github.com/stroppy-io/stroppy/v6/pkg/config"
)

var (
	errUnknownDriverType  = inputError("unknown driver type")
	errUnknownTxIsolation = inputError("unknown tx isolation")
)

// String-typed enums authored by Go workloads and resolved to the config enums
// consumed by the driver layer.

type DriverTypeName string

const (
	DriverPostgres  DriverTypeName = "postgres"
	DriverMySQL     DriverTypeName = "mysql"
	DriverPicodata  DriverTypeName = "picodata"
	DriverYDB       DriverTypeName = "ydb"
	DriverNoop      DriverTypeName = "noop"
	DriverCSV       DriverTypeName = "csv"
	DriverRecording DriverTypeName = "recording"
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
	case "recording":
		return config.DriverTypeRecording, nil
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
	case config.DriverTypeRecording:
		return DriverRecording
	default:
		return ""
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
