package bench

import (
	"errors"
	"fmt"

	"github.com/stroppy-io/stroppy/pkg/common/proto/stroppy"
	"github.com/stroppy-io/stroppy/pkg/datagen/dgproto"
)

var (
	errUnknownDriverType   = errors.New("unknown driver type")
	errUnknownErrorMode    = errors.New("unknown error mode")
	errUnknownTxIsolation  = errors.New("unknown tx isolation")
	errUnknownInsertMethod = errors.New("unknown insert method")
)

// String-typed enums a Go workload authors with; resolved to the proto enums the
// driver layer consumes. Ports helpers.ts string<->enum maps.

type DriverTypeName string

const (
	DriverPostgres DriverTypeName = "postgres"
	DriverMySQL    DriverTypeName = "mysql"
	DriverPicodata DriverTypeName = "picodata"
	DriverYDB      DriverTypeName = "ydb"
	DriverNoop     DriverTypeName = "noop"
	DriverCSV      DriverTypeName = "csv"
)

func ParseDriverType(s string) (stroppy.DriverConfig_DriverType, error) {
	switch s {
	case "", "postgres":
		return stroppy.DriverConfig_DRIVER_TYPE_POSTGRES, nil
	case "mysql":
		return stroppy.DriverConfig_DRIVER_TYPE_MYSQL, nil
	case "picodata", "pico":
		return stroppy.DriverConfig_DRIVER_TYPE_PICODATA, nil
	case "ydb":
		return stroppy.DriverConfig_DRIVER_TYPE_YDB, nil
	case "noop":
		return stroppy.DriverConfig_DRIVER_TYPE_NOOP, nil
	case "csv":
		return stroppy.DriverConfig_DRIVER_TYPE_CSV, nil
	default:
		return 0, fmt.Errorf("%w %q", errUnknownDriverType, s)
	}
}

// DriverTypeNameFromProto reverse-maps the proto driver enum to the authoring
// string name.
func DriverTypeNameFromProto(t stroppy.DriverConfig_DriverType) DriverTypeName {
	switch t {
	case stroppy.DriverConfig_DRIVER_TYPE_POSTGRES:
		return DriverPostgres
	case stroppy.DriverConfig_DRIVER_TYPE_MYSQL:
		return DriverMySQL
	case stroppy.DriverConfig_DRIVER_TYPE_PICODATA:
		return DriverPicodata
	case stroppy.DriverConfig_DRIVER_TYPE_YDB:
		return DriverYDB
	case stroppy.DriverConfig_DRIVER_TYPE_NOOP:
		return DriverNoop
	case stroppy.DriverConfig_DRIVER_TYPE_CSV:
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

func ParseErrorMode(s string) (stroppy.DriverConfig_ErrorMode, error) {
	switch s {
	case "", "silent":
		return stroppy.DriverConfig_ERROR_MODE_SILENT, nil
	case "log":
		return stroppy.DriverConfig_ERROR_MODE_LOG, nil
	case "throw":
		return stroppy.DriverConfig_ERROR_MODE_THROW, nil
	case "fail":
		return stroppy.DriverConfig_ERROR_MODE_FAIL, nil
	case "abort":
		return stroppy.DriverConfig_ERROR_MODE_ABORT, nil
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

func ParseTxIsolation(s string) (stroppy.TxIsolationLevel, error) {
	switch s {
	case "", "db_default":
		return stroppy.TxIsolationLevel_UNSPECIFIED, nil
	case "read_uncommitted":
		return stroppy.TxIsolationLevel_READ_UNCOMMITTED, nil
	case "read_committed":
		return stroppy.TxIsolationLevel_READ_COMMITTED, nil
	case "repeatable_read":
		return stroppy.TxIsolationLevel_REPEATABLE_READ, nil
	case "serializable":
		return stroppy.TxIsolationLevel_SERIALIZABLE, nil
	case "conn":
		return stroppy.TxIsolationLevel_CONNECTION_ONLY, nil
	case "none":
		return stroppy.TxIsolationLevel_NONE, nil
	default:
		return 0, fmt.Errorf("%w %q", errUnknownTxIsolation, s)
	}
}

type InsertMethodName string

const (
	InsertPlainQuery InsertMethodName = "plain_query"
	InsertPlainBulk  InsertMethodName = "plain_bulk"
	InsertColumnar   InsertMethodName = "columnar"
	InsertNative     InsertMethodName = "native"
)

func ParseInsertMethod(s string) (dgproto.InsertMethod, error) {
	switch s {
	case "", "plain_query":
		return dgproto.InsertMethod_PLAIN_QUERY, nil
	case "plain_bulk":
		return dgproto.InsertMethod_PLAIN_BULK, nil
	case "columnar":
		return dgproto.InsertMethod_COLUMNAR, nil
	case "native":
		return dgproto.InsertMethod_NATIVE, nil
	default:
		return 0, fmt.Errorf("%w %q", errUnknownInsertMethod, s)
	}
}
