// Package config holds the plain-Go application configuration types for run,
// workload, driver, pool, logger, exporter, and isolation settings. It replaces
// the frozen protobuf types that previously lived under
// pkg/common/proto/stroppy: the JSON field names are preserved verbatim, but
// the protobuf reflection, descriptors, validators, and oneof wrappers are gone.
//
//go:generate go run github.com/stroppy-io/stroppy/internal/jsonschema-gen -out ../../docs/jsonschema/run.schema.json
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// DriverType identifies a database driver implementation.
type DriverType int32

const (
	DriverTypeUnspecified DriverType = 0
	DriverTypePostgres    DriverType = 1
	DriverTypeMySQL       DriverType = 2
	DriverTypePicodata    DriverType = 3
	DriverTypeYDB         DriverType = 4
	DriverTypeNoop        DriverType = 5
	DriverTypeCSV         DriverType = 6
)

var driverTypeNames = map[DriverType]string{
	DriverTypeUnspecified: "unspecified",
	DriverTypePostgres:    "postgres",
	DriverTypeMySQL:       "mysql",
	DriverTypePicodata:    "picodata",
	DriverTypeYDB:         "ydb",
	DriverTypeNoop:        "noop",
	DriverTypeCSV:         "csv",
}

func (d DriverType) String() string {
	if name, ok := driverTypeNames[d]; ok {
		return name
	}

	return fmt.Sprintf("DriverType(%d)", int32(d))
}

// DriverTypeValues returns every concrete driver type in enum order, excluding
// the unspecified sentinel.
func DriverTypeValues() []DriverType {
	return []DriverType{
		DriverTypePostgres,
		DriverTypeMySQL,
		DriverTypePicodata,
		DriverTypeYDB,
		DriverTypeNoop,
		DriverTypeCSV,
	}
}

// ErrorMode selects how query and insert errors are handled.
type ErrorMode int32

const (
	ErrorModeUnspecified ErrorMode = 0
	ErrorModeSilent      ErrorMode = 1
	ErrorModeLog         ErrorMode = 2
	ErrorModeThrow       ErrorMode = 3
	ErrorModeFail        ErrorMode = 4
	ErrorModeAbort       ErrorMode = 5
)

// TxIsolationLevel is the isolation level of a database transaction.
type TxIsolationLevel int32

const (
	TxIsolationLevelUnspecified     TxIsolationLevel = 0
	TxIsolationLevelReadUncommitted TxIsolationLevel = 1
	TxIsolationLevelReadCommitted   TxIsolationLevel = 2
	TxIsolationLevelRepeatableRead  TxIsolationLevel = 3
	TxIsolationLevelSerializable    TxIsolationLevel = 4
	TxIsolationLevelConnectionOnly  TxIsolationLevel = 5
	TxIsolationLevelNone            TxIsolationLevel = 6
)

// LogLevel is the minimum log level to output.
type LogLevel string

const (
	LogLevelDebug LogLevel = "LOG_LEVEL_DEBUG"
	LogLevelInfo  LogLevel = "LOG_LEVEL_INFO"
	LogLevelWarn  LogLevel = "LOG_LEVEL_WARN"
	LogLevelError LogLevel = "LOG_LEVEL_ERROR"
	LogLevelFatal LogLevel = "LOG_LEVEL_FATAL"
)

// LogMode is the logging output mode.
type LogMode string

const (
	LogModeDevelopment LogMode = "LOG_MODE_DEVELOPMENT"
	LogModeProduction  LogMode = "LOG_MODE_PRODUCTION"
)

var (
	errInvalidLogLevel = errors.New("invalid log level")
	errInvalidLogMode  = errors.New("invalid log mode")
)

// LogLevelValues returns every declared LogLevel constant in canonical order.
func LogLevelValues() []LogLevel {
	return []LogLevel{
		LogLevelDebug,
		LogLevelInfo,
		LogLevelWarn,
		LogLevelError,
		LogLevelFatal,
	}
}

// LogModeValues returns every declared LogMode constant in canonical order.
func LogModeValues() []LogMode {
	return []LogMode{LogModeDevelopment, LogModeProduction}
}

// LevelShort returns the short (zap-style) spelling used by --log-level and
// LOG_LEVEL for a configured LogLevel constant. It returns "" when unset.
func (l LogLevel) LevelShort() string {
	switch l {
	case LogLevelDebug:
		return "debug"
	case LogLevelInfo:
		return "info"
	case LogLevelWarn:
		return "warn"
	case LogLevelError:
		return "error"
	case LogLevelFatal:
		return "fatal"
	default:
		return ""
	}
}

// ModeShort returns the short spelling used by --log-mode and LOG_MODE for a
// configured LogMode constant. It returns "" when unset.
func (m LogMode) ModeShort() string {
	switch m {
	case LogModeDevelopment:
		return "development"
	case LogModeProduction:
		return "production"
	default:
		return ""
	}
}

// ValidateLogLevel validates a CLI/env log level value and returns its
// canonical short spelling. It rejects values outside the LogLevel set.
func ValidateLogLevel(value string) (string, error) {
	switch strings.ToLower(value) {
	case "debug":
		return "debug", nil
	case "info":
		return "info", nil
	case "warn":
		return "warn", nil
	case "error":
		return "error", nil
	case "fatal":
		return "fatal", nil
	default:
		return "", fmt.Errorf("%w: %q (want debug|info|warn|error|fatal)", errInvalidLogLevel, value)
	}
}

// ValidateLogMode validates a CLI/env log mode value and returns its canonical
// short spelling. It rejects values outside the LogMode set.
func ValidateLogMode(value string) (string, error) {
	switch strings.ToLower(value) {
	case "development":
		return "development", nil
	case "production":
		return "production", nil
	default:
		return "", fmt.Errorf("%w: %q (want development|production)", errInvalidLogMode, value)
	}
}

// UnmarshalJSON rejects values outside the LogLevel constant set. The frozen
// protobuf schema encoded this as a strict enum; an untyped string would
// otherwise silently accept any value.
func (l *LogLevel) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		*l = ""

		return nil
	}

	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}

	for _, valid := range LogLevelValues() {
		if s == string(valid) {
			*l = valid

			return nil
		}
	}

	return fmt.Errorf("%w: %q", errInvalidLogLevel, s)
}

// UnmarshalJSON rejects values outside the LogMode constant set. The frozen
// protobuf schema encoded this as a strict enum; an untyped string would
// otherwise silently accept any value.
func (m *LogMode) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		*m = ""

		return nil
	}

	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}

	for _, valid := range LogModeValues() {
		if s == string(valid) {
			*m = valid

			return nil
		}
	}

	return fmt.Errorf("%w: %q", errInvalidLogMode, s)
}

// RunConfig is the top-level stroppy config file schema.
type RunConfig struct {
	Version string                      `json:"version,omitempty"`
	Script  *string                     `json:"script,omitempty"`
	SQL     *string                     `json:"sql,omitempty"`
	Global  *GlobalConfig               `json:"global,omitempty"`
	Drivers map[uint32]*DriverRunConfig `json:"drivers,omitempty"`
	Env     map[string]string           `json:"env,omitempty"`
	Steps   []string                    `json:"steps,omitempty"`
	NoSteps []string                    `json:"noSteps,omitempty"`
}

func (c *RunConfig) GetScript() string {
	if c != nil && c.Script != nil {
		return *c.Script
	}

	return ""
}

func (c *RunConfig) GetSQL() string {
	if c != nil && c.SQL != nil {
		return *c.SQL
	}

	return ""
}

// DriverRunConfig is the user-facing driver configuration for the config file.
type DriverRunConfig struct {
	DriverType            *string               `json:"driverType,omitempty"`
	URL                   *string               `json:"url,omitempty"`
	Pool                  *PoolConfig           `json:"pool,omitempty"`
	ErrorMode             *string               `json:"errorMode,omitempty"`
	BulkSize              *int32                `json:"bulkSize,omitempty"`
	CaCertFile            *string               `json:"caCertFile,omitempty"`
	AuthToken             *string               `json:"authToken,omitempty"`
	AuthUser              *string               `json:"authUser,omitempty"`
	AuthPassword          *string               `json:"authPassword,omitempty"`
	TLSInsecureSkipVerify *bool                 `json:"tlsInsecureSkipVerify,omitempty"`
	DefaultInsertMethod   *string               `json:"defaultInsertMethod,omitempty"`
	Postgres              *PostgresConfig       `json:"postgres,omitempty"`
	SQL                   *SQLConfig            `json:"sql,omitempty"`
	InsertProgress        *InsertProgressConfig `json:"insertProgress,omitempty"`
}

func (c *DriverRunConfig) GetDriverType() string {
	if c != nil && c.DriverType != nil {
		return *c.DriverType
	}

	return ""
}

func (c *DriverRunConfig) GetURL() string {
	if c != nil && c.URL != nil {
		return *c.URL
	}

	return ""
}

func (c *DriverRunConfig) GetErrorMode() string {
	if c != nil && c.ErrorMode != nil {
		return *c.ErrorMode
	}

	return ""
}

func (c *DriverRunConfig) GetBulkSize() int32 {
	if c != nil && c.BulkSize != nil {
		return *c.BulkSize
	}

	return 0
}

func (c *DriverRunConfig) GetCaCertFile() string {
	if c != nil && c.CaCertFile != nil {
		return *c.CaCertFile
	}

	return ""
}

func (c *DriverRunConfig) GetAuthToken() string {
	if c != nil && c.AuthToken != nil {
		return *c.AuthToken
	}

	return ""
}

func (c *DriverRunConfig) GetAuthUser() string {
	if c != nil && c.AuthUser != nil {
		return *c.AuthUser
	}

	return ""
}

func (c *DriverRunConfig) GetAuthPassword() string {
	if c != nil && c.AuthPassword != nil {
		return *c.AuthPassword
	}

	return ""
}

func (c *DriverRunConfig) GetTLSInsecureSkipVerify() bool {
	if c != nil && c.TLSInsecureSkipVerify != nil {
		return *c.TLSInsecureSkipVerify
	}

	return false
}

func (c *DriverRunConfig) GetDefaultInsertMethod() string {
	if c != nil && c.DefaultInsertMethod != nil {
		return *c.DefaultInsertMethod
	}

	return ""
}

// PoolConfig is a sugar pool block accepted on the config-file driver. It maps
// to PostgresConfig or SQLConfig based on driver type.
type PoolConfig struct {
	MaxConns                 *int32  `json:"maxConns,omitempty"`
	MinConns                 *int32  `json:"minConns,omitempty"`
	MinIdleConns             *int32  `json:"minIdleConns,omitempty"`
	MaxConnLifetime          *string `json:"maxConnLifetime,omitempty"`
	MaxConnIdleTime          *string `json:"maxConnIdleTime,omitempty"`
	TraceLogLevel            *string `json:"traceLogLevel,omitempty"`
	DefaultQueryExecMode     *string `json:"defaultQueryExecMode,omitempty"`
	DescriptionCacheCapacity *int32  `json:"descriptionCacheCapacity,omitempty"`
	StatementCacheCapacity   *int32  `json:"statementCacheCapacity,omitempty"`
	MaxOpenConns             *int32  `json:"maxOpenConns,omitempty"`
	MaxIdleConns             *int32  `json:"maxIdleConns,omitempty"`
	ConnMaxLifetime          *string `json:"connMaxLifetime,omitempty"`
	ConnMaxIdleTime          *string `json:"connMaxIdleTime,omitempty"`
}

func (c *PoolConfig) GetMaxConns() int32 {
	if c != nil && c.MaxConns != nil {
		return *c.MaxConns
	}

	return 0
}

func (c *PoolConfig) GetMinConns() int32 {
	if c != nil && c.MinConns != nil {
		return *c.MinConns
	}

	return 0
}

func (c *PoolConfig) GetMinIdleConns() int32 {
	if c != nil && c.MinIdleConns != nil {
		return *c.MinIdleConns
	}

	return 0
}

func (c *PoolConfig) GetMaxConnLifetime() string {
	if c != nil && c.MaxConnLifetime != nil {
		return *c.MaxConnLifetime
	}

	return ""
}

func (c *PoolConfig) GetMaxConnIdleTime() string {
	if c != nil && c.MaxConnIdleTime != nil {
		return *c.MaxConnIdleTime
	}

	return ""
}

func (c *PoolConfig) GetTraceLogLevel() string {
	if c != nil && c.TraceLogLevel != nil {
		return *c.TraceLogLevel
	}

	return ""
}

func (c *PoolConfig) GetDefaultQueryExecMode() string {
	if c != nil && c.DefaultQueryExecMode != nil {
		return *c.DefaultQueryExecMode
	}

	return ""
}

func (c *PoolConfig) GetDescriptionCacheCapacity() int32 {
	if c != nil && c.DescriptionCacheCapacity != nil {
		return *c.DescriptionCacheCapacity
	}

	return 0
}

func (c *PoolConfig) GetStatementCacheCapacity() int32 {
	if c != nil && c.StatementCacheCapacity != nil {
		return *c.StatementCacheCapacity
	}

	return 0
}

func (c *PoolConfig) GetMaxOpenConns() int32 {
	if c != nil && c.MaxOpenConns != nil {
		return *c.MaxOpenConns
	}

	return 0
}

func (c *PoolConfig) GetMaxIdleConns() int32 {
	if c != nil && c.MaxIdleConns != nil {
		return *c.MaxIdleConns
	}

	return 0
}

func (c *PoolConfig) GetConnMaxLifetime() string {
	if c != nil && c.ConnMaxLifetime != nil {
		return *c.ConnMaxLifetime
	}

	return ""
}

func (c *PoolConfig) GetConnMaxIdleTime() string {
	if c != nil && c.ConnMaxIdleTime != nil {
		return *c.ConnMaxIdleTime
	}

	return ""
}

// DriverConfig is the runtime driver configuration consumed by the drivers and
// bench engine. It is assembled internally, never JSON-decoded directly.
type DriverConfig struct {
	URL        string     `json:"url,omitempty"`
	DriverType DriverType `json:"driverType,omitempty"`
	// InsertMethod is the resolved canonical method name; empty means the
	// workload's own InsertRequest.Method.
	InsertMethod          string                `json:"insertMethod,omitempty"`
	BulkSize              *int32                `json:"bulkSize,omitempty"`
	ErrorMode             ErrorMode             `json:"errorMode,omitempty"`
	Postgres              *PostgresConfig       `json:"postgres,omitempty"`
	SQL                   *SQLConfig            `json:"sql,omitempty"`
	CaCertFile            *string               `json:"caCertFile,omitempty"`
	AuthToken             *string               `json:"authToken,omitempty"`
	AuthUser              *string               `json:"authUser,omitempty"`
	AuthPassword          *string               `json:"authPassword,omitempty"`
	TLSInsecureSkipVerify *bool                 `json:"tlsInsecureSkipVerify,omitempty"`
	InsertProgress        *InsertProgressConfig `json:"insertProgress,omitempty"`
}

func (c *DriverConfig) GetInsertMethod() string {
	if c != nil {
		return c.InsertMethod
	}

	return ""
}

func (c *DriverConfig) GetBulkSize() int32 {
	if c != nil && c.BulkSize != nil {
		return *c.BulkSize
	}

	return 0
}

func (c *DriverConfig) GetCaCertFile() string {
	if c != nil && c.CaCertFile != nil {
		return *c.CaCertFile
	}

	return ""
}

func (c *DriverConfig) GetAuthToken() string {
	if c != nil && c.AuthToken != nil {
		return *c.AuthToken
	}

	return ""
}

func (c *DriverConfig) GetAuthUser() string {
	if c != nil && c.AuthUser != nil {
		return *c.AuthUser
	}

	return ""
}

func (c *DriverConfig) GetAuthPassword() string {
	if c != nil && c.AuthPassword != nil {
		return *c.AuthPassword
	}

	return ""
}

func (c *DriverConfig) GetTLSInsecureSkipVerify() bool {
	if c != nil && c.TLSInsecureSkipVerify != nil {
		return *c.TLSInsecureSkipVerify
	}

	return false
}

// PostgresConfig is PostgreSQL-specific pool and connection configuration.
type PostgresConfig struct {
	TraceLogLevel            *string `json:"traceLogLevel,omitempty"`
	MaxConnLifetime          *string `json:"maxConnLifetime,omitempty"`
	MaxConnIdleTime          *string `json:"maxConnIdleTime,omitempty"`
	MaxConns                 *int32  `json:"maxConns,omitempty"`
	MinConns                 *int32  `json:"minConns,omitempty"`
	MinIdleConns             *int32  `json:"minIdleConns,omitempty"`
	DefaultQueryExecMode     *string `json:"defaultQueryExecMode,omitempty"`
	DescriptionCacheCapacity *int32  `json:"descriptionCacheCapacity,omitempty"`
	StatementCacheCapacity   *int32  `json:"statementCacheCapacity,omitempty"`
}

func (c *PostgresConfig) GetTraceLogLevel() string {
	if c != nil && c.TraceLogLevel != nil {
		return *c.TraceLogLevel
	}

	return ""
}

func (c *PostgresConfig) GetMaxConnLifetime() string {
	if c != nil && c.MaxConnLifetime != nil {
		return *c.MaxConnLifetime
	}

	return ""
}

func (c *PostgresConfig) GetMaxConnIdleTime() string {
	if c != nil && c.MaxConnIdleTime != nil {
		return *c.MaxConnIdleTime
	}

	return ""
}

func (c *PostgresConfig) GetMaxConns() int32 {
	if c != nil && c.MaxConns != nil {
		return *c.MaxConns
	}

	return 0
}

func (c *PostgresConfig) GetMinConns() int32 {
	if c != nil && c.MinConns != nil {
		return *c.MinConns
	}

	return 0
}

func (c *PostgresConfig) GetMinIdleConns() int32 {
	if c != nil && c.MinIdleConns != nil {
		return *c.MinIdleConns
	}

	return 0
}

func (c *PostgresConfig) GetDefaultQueryExecMode() string {
	if c != nil && c.DefaultQueryExecMode != nil {
		return *c.DefaultQueryExecMode
	}

	return ""
}

func (c *PostgresConfig) GetDescriptionCacheCapacity() int32 {
	if c != nil && c.DescriptionCacheCapacity != nil {
		return *c.DescriptionCacheCapacity
	}

	return 0
}

func (c *PostgresConfig) GetStatementCacheCapacity() int32 {
	if c != nil && c.StatementCacheCapacity != nil {
		return *c.StatementCacheCapacity
	}

	return 0
}

// SQLConfig is generic database/sql pool settings for SQL-based drivers.
type SQLConfig struct {
	MaxOpenConns    *int32  `json:"maxOpenConns,omitempty"`
	MaxIdleConns    *int32  `json:"maxIdleConns,omitempty"`
	ConnMaxLifetime *string `json:"connMaxLifetime,omitempty"`
	ConnMaxIdleTime *string `json:"connMaxIdleTime,omitempty"`
}

func (c *SQLConfig) GetMaxOpenConns() int32 {
	if c != nil && c.MaxOpenConns != nil {
		return *c.MaxOpenConns
	}

	return 0
}

func (c *SQLConfig) GetMaxIdleConns() int32 {
	if c != nil && c.MaxIdleConns != nil {
		return *c.MaxIdleConns
	}

	return 0
}

func (c *SQLConfig) GetConnMaxLifetime() string {
	if c != nil && c.ConnMaxLifetime != nil {
		return *c.ConnMaxLifetime
	}

	return ""
}

func (c *SQLConfig) GetConnMaxIdleTime() string {
	if c != nil && c.ConnMaxIdleTime != nil {
		return *c.ConnMaxIdleTime
	}

	return ""
}

// InsertProgressConfig is periodic InsertSpec progress reporting.
type InsertProgressConfig struct {
	Enabled    *bool   `json:"enabled,omitempty"`
	Interval   *string `json:"interval,omitempty"`
	StallAfter *string `json:"stallAfter,omitempty"`
	Mode       *string `json:"mode,omitempty"`
}

func (c *InsertProgressConfig) GetEnabled() bool {
	if c != nil && c.Enabled != nil {
		return *c.Enabled
	}

	return false
}

func (c *InsertProgressConfig) GetInterval() string {
	if c != nil && c.Interval != nil {
		return *c.Interval
	}

	return ""
}

func (c *InsertProgressConfig) GetStallAfter() string {
	if c != nil && c.StallAfter != nil {
		return *c.StallAfter
	}

	return ""
}

func (c *InsertProgressConfig) GetMode() string {
	if c != nil && c.Mode != nil {
		return *c.Mode
	}

	return ""
}

// GlobalConfig holds global stroppy settings: logger, exporter, seed, run_id,
// and metadata.
type GlobalConfig struct {
	Version  string            `json:"version,omitempty"`
	RunID    string            `json:"runId,omitempty"`
	Seed     uint64            `json:"seed,omitempty"`
	Metadata map[string]string `json:"metadata,omitempty"`
	Logger   *LoggerConfig     `json:"logger,omitempty"`
	Exporter *ExporterConfig   `json:"exporter,omitempty"`
}

// LoggerConfig controls log levels and output formatting.
type LoggerConfig struct {
	LogLevel LogLevel `json:"logLevel,omitempty"`
	LogMode  LogMode  `json:"logMode,omitempty"`
}

// ExporterConfig contains named configuration for an OTLP exporter.
type ExporterConfig struct {
	Name       string      `json:"name,omitempty"`
	OtlpExport *OtlpExport `json:"otlpExport,omitempty"`
}

// OtlpExport contains configuration for exporting metrics via OTLP.
type OtlpExport struct {
	OtlpGrpcEndpoint        *string `json:"otlpGrpcEndpoint,omitempty"`
	OtlpHTTPEndpoint        *string `json:"otlpHttpEndpoint,omitempty"`
	OtlpHTTPExporterURLPath *string `json:"otlpHttpExporterUrlPath,omitempty"`
	OtlpEndpointInsecure    *bool   `json:"otlpEndpointInsecure,omitempty"`
	OtlpHeaders             *string `json:"otlpHeaders,omitempty"`
	OtlpMetricsPrefix       *string `json:"otlpMetricsPrefix,omitempty"`
}

func (o *OtlpExport) GetOtlpGrpcEndpoint() string {
	if o != nil && o.OtlpGrpcEndpoint != nil {
		return *o.OtlpGrpcEndpoint
	}

	return ""
}

func (o *OtlpExport) GetOtlpHTTPEndpoint() string {
	if o != nil && o.OtlpHTTPEndpoint != nil {
		return *o.OtlpHTTPEndpoint
	}

	return ""
}

func (o *OtlpExport) GetOtlpHTTPExporterURLPath() string {
	if o != nil && o.OtlpHTTPExporterURLPath != nil {
		return *o.OtlpHTTPExporterURLPath
	}

	return ""
}

func (o *OtlpExport) GetOtlpEndpointInsecure() bool {
	if o != nil && o.OtlpEndpointInsecure != nil {
		return *o.OtlpEndpointInsecure
	}

	return false
}

func (o *OtlpExport) GetOtlpHeaders() string {
	if o != nil && o.OtlpHeaders != nil {
		return *o.OtlpHeaders
	}

	return ""
}

func (o *OtlpExport) GetOtlpMetricsPrefix() string {
	if o != nil && o.OtlpMetricsPrefix != nil {
		return *o.OtlpMetricsPrefix
	}

	return ""
}
