package logger

import (
	"errors"
	"fmt"
	"net/url"
	"strings"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var errInvalidLogMode = errors.New("invalid log mode")

// LogMod names a logger output mode.
type LogMod string

const (
	DevelopmentMod LogMod = "development"
	ProductionMod  LogMod = "production"
)

// global is the single process-wide logger. It starts as a development/debug
// logger so commands that never run a workload (probe, version, help) still
// have output, and is replaced by Init before drivers and workloads start.
var global = newDefault()

// Init configures the process-wide logger from the already-resolved level and
// mode. level is a zap level name (debug, info, warn, error, fatal); mode is
// one of LogMod. An unknown level or mode returns an error and leaves the
// current logger untouched. Call once per run, before drivers/workloads start.
func Init(level, mode string, opts ...zap.Option) error {
	zapLevel, err := zapcore.ParseLevel(level)
	if err != nil {
		return fmt.Errorf("invalid log level %q: %w", level, err)
	}

	logMod, err := ParseMode(mode)
	if err != nil {
		return err
	}

	built, err := newZapCfg(logMod, zapLevel).Build(opts...)
	if err != nil {
		return err
	}

	global = built

	return nil
}

// ParseMode validates mode and returns the matching LogMod.
func ParseMode(mode string) (LogMod, error) {
	switch LogMod(mode) {
	case DevelopmentMod, ProductionMod:
		return LogMod(mode), nil
	default:
		return "", fmt.Errorf("%w: %q (want %q or %q)", errInvalidLogMode, mode, DevelopmentMod, ProductionMod)
	}
}

// Global returns the single process-wide logger.
func Global() *zap.Logger {
	return global
}

// newDefault creates the startup logger used before Init runs.
func newDefault(opts ...zap.Option) *zap.Logger {
	logger, _ := newZapCfg(DevelopmentMod, zapcore.DebugLevel).Build(opts...)

	return logger
}

// newZapCfg creates a zap config for the given mode and level.
func newZapCfg(mod LogMod, logLevel zapcore.Level) zap.Config {
	var cfg zap.Config

	switch mod {
	case ProductionMod:
		cfg = zap.NewProductionConfig()
		cfg.Level.SetLevel(logLevel)
	default:
		cfg = zap.NewDevelopmentConfig()
		cfg.Level.SetLevel(logLevel)
	}

	cfg.DisableStacktrace = true

	return cfg
}

// StructLogger is an alias for *zap.Logger included in project structs.
type StructLogger = *zap.Logger

// NewStructLogger returns a named child of the process-wide logger.
func NewStructLogger(name string) StructLogger {
	return Global().Named(name)
}

// redactedPassword is the marker that replaces credential material in a DSN.
const redactedPassword = "xxxxx"

// RedactDSN masks the password/userinfo in a database URL or DSN so
// credentials never reach the log at any level. It covers standard URLs
// (postgres://user:pass@host/db) and driver-specific DSNs such as go-mysql's
// user:pass@tcp(host:port)/db.
func RedactDSN(dsn string) string {
	if dsn == "" {
		return dsn
	}

	if u, err := url.Parse(dsn); err == nil && u.User != nil {
		if _, hasPassword := u.User.Password(); hasPassword {
			u.User = url.UserPassword(u.User.Username(), redactedPassword)
		}

		return u.String()
	}

	return redactUserinfo(dsn)
}

// redactUserinfo masks the password in a colon-delimited userinfo prefix for
// DSNs that fall outside the URL grammar (no scheme).
func redactUserinfo(dsn string) string {
	at := strings.Index(dsn, "@")
	if at < 0 {
		return dsn
	}

	colon := strings.Index(dsn[:at], ":")
	if colon < 0 {
		return dsn
	}

	return dsn[:colon] + ":" + redactedPassword + dsn[at:]
}
