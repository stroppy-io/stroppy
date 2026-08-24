package logger

import (
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"unicode"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// LogMod names a logger output mode.
type LogMod string

const (
	DevelopmentMod LogMod = "development"
	ProductionMod  LogMod = "production"
)

// Config is the plain-Go logger configuration.
type Config struct {
	LogMod   LogMod `default:"development" mapstructure:"mod"   validate:"oneof=production development"`
	LogLevel string `default:"debug"       mapstructure:"level" validate:"oneof=debug info warn error fatal"`
}

var (
	errInvalidLogMode = errors.New("invalid log mode")
	globalLogger      atomic.Pointer[zap.Logger]
)

func init() {
	globalLogger.Store(newDefault())
}

// Init validates and installs a new process-wide logger. Invalid settings leave
// the previously installed logger unchanged.
func Init(level, mode string, opts ...zap.Option) error {
	built, err := build(level, mode, opts...)
	if err != nil {
		return err
	}

	globalLogger.Store(built)

	return nil
}

// NewFromConfig installs a logger built from a plain-Go configuration.
//
// It preserves the historical panic-on-invalid-config behavior; prefer Init
// when configuration errors must be returned to the caller.
func NewFromConfig(cfg *Config, opts ...zap.Option) *zap.Logger {
	if cfg == nil {
		panic("logger config is nil")
	}

	if err := Init(cfg.LogLevel, string(cfg.LogMod), opts...); err != nil {
		panic(err)
	}

	return Global()
}

func build(level, mode string, opts ...zap.Option) (*zap.Logger, error) {
	zapLevel, err := zapcore.ParseLevel(level)
	if err != nil {
		return nil, fmt.Errorf("invalid log level %q: %w", level, err)
	}

	logMod, err := ParseMode(mode)
	if err != nil {
		return nil, err
	}

	built, err := newZapCfg(logMod, zapLevel).Build(opts...)
	if err != nil {
		return nil, err
	}

	return built, nil
}

// ParseMode validates a logger output mode.
func ParseMode(mode string) (LogMod, error) {
	switch LogMod(mode) {
	case DevelopmentMod, ProductionMod:
		return LogMod(mode), nil
	default:
		return "", fmt.Errorf("%w: %q (want %q or %q)", errInvalidLogMode, mode, DevelopmentMod, ProductionMod)
	}
}

// Global returns the process-wide logger. It is always non-nil.
func Global() *zap.Logger {
	return globalLogger.Load()
}

// newDefault creates the startup logger used before Init runs.
func newDefault(opts ...zap.Option) *zap.Logger {
	logger, err := newZapCfg(DevelopmentMod, zapcore.DebugLevel).Build(opts...)
	if err != nil {
		panic(err)
	}

	return logger
}

// newZapCfg creates a zap config for the given mode and level.
func newZapCfg(mod LogMod, logLevel zapcore.Level) zap.Config {
	var cfg zap.Config

	switch mod {
	case ProductionMod:
		cfg = zap.NewProductionConfig()
	case DevelopmentMod:
		cfg = zap.NewDevelopmentConfig()
	default:
		cfg = zap.NewDevelopmentConfig()
	}

	cfg.Level.SetLevel(logLevel)
	cfg.DisableStacktrace = true

	return cfg
}

// StructLogger is an alias for *zap.Logger included in project structs.
type StructLogger = *zap.Logger

// NewStructLogger returns a named child of the process-wide logger.
func NewStructLogger(name string) StructLogger {
	return Global().Named(name)
}

const (
	redactedSecret           = "xxxxx"
	queryDecodePasses        = 2
	percentEscapeDigits      = 2
	hexAlphabetBase     byte = 10
)

// RedactDSN masks credentials in database URLs and DSNs while preserving the
// endpoint, database path, and non-secret query options.
func RedactDSN(dsn string) string {
	if dsn == "" {
		return dsn
	}

	return redactUserinfo(redactQueryValues(dsn))
}

func redactQueryValues(dsn string) string {
	queryStart := strings.IndexByte(dsn, '?')
	if queryStart < 0 {
		return dsn
	}

	queryEnd := strings.IndexByte(dsn[queryStart+1:], '#')
	if queryEnd >= 0 {
		queryEnd += queryStart + 1
	} else {
		queryEnd = len(dsn)
	}

	query := dsn[queryStart+1 : queryEnd]

	parts := strings.Split(query, "&")
	for index, part := range parts {
		key, _, hasValue := strings.Cut(part, "=")
		if hasValue && isSecretKey(key) {
			parts[index] = key + "=" + redactedSecret
		}
	}

	return dsn[:queryStart+1] + strings.Join(parts, "&") + dsn[queryEnd:]
}

func isSecretKey(key string) bool {
	decoded := decodeQueryKey(key)

	return hasSecretKeySuffix(normalizeQueryKey(decoded)) ||
		hasSecretKeySuffix(normalizeQueryKey(stripMalformedEscapes(decoded)))
}

func decodeQueryKey(value string) string {
	for range queryDecodePasses {
		decoded := urlQueryUnescape(value)
		if decoded == value {
			break
		}

		value = decoded
	}

	return value
}

func normalizeQueryKey(value string) string {
	var normalized strings.Builder

	for _, r := range value {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			normalized.WriteRune(unicode.ToLower(r))
		}
	}

	return normalized.String()
}

func hasSecretKeySuffix(key string) bool {
	for _, suffix := range []string{
		"password", "passwd", "pwd", "secret", "token", "credential", "credentials", "apikey",
	} {
		if strings.HasSuffix(key, suffix) {
			return true
		}
	}

	return false
}

func stripMalformedEscapes(value string) string {
	var result strings.Builder

	for index := 0; index < len(value); index++ {
		if value[index] != '%' {
			result.WriteByte(value[index])

			continue
		}

		index += min(percentEscapeDigits, len(value)-index-1)
	}

	return result.String()
}

func urlQueryUnescape(value string) string {
	var result strings.Builder

	for index := 0; index < len(value); index++ {
		if value[index] == '+' {
			result.WriteByte(' ')

			continue
		}

		if value[index] != '%' || index+percentEscapeDigits >= len(value) {
			result.WriteByte(value[index])

			continue
		}

		high, okHigh := fromHex(value[index+1])

		low, okLow := fromHex(value[index+percentEscapeDigits])
		if !okHigh || !okLow {
			result.WriteByte(value[index])

			continue
		}

		result.WriteByte(high<<4 | low)

		index += percentEscapeDigits
	}

	return result.String()
}

func fromHex(value byte) (byte, bool) {
	switch {
	case value >= '0' && value <= '9':
		return value - '0', true
	case value >= 'a' && value <= 'f':
		return value - 'a' + hexAlphabetBase, true
	case value >= 'A' && value <= 'F':
		return value - 'A' + hexAlphabetBase, true
	default:
		return 0, false
	}
}

func redactUserinfo(dsn string) string {
	end := len(dsn)
	if queryStart := strings.IndexByte(dsn, '?'); queryStart >= 0 {
		end = queryStart
	}

	start := 0
	if scheme := strings.Index(dsn[:end], "://"); scheme >= 0 {
		start = scheme + len("://")
	}

	at := strings.LastIndexByte(dsn[start:end], '@')
	if at < 0 {
		return dsn
	}

	at += start

	userinfo := dsn[start:at]

	colon := strings.LastIndexByte(userinfo, ':')
	if colon < 0 {
		return dsn
	}

	colon += start

	return dsn[:colon+1] + redactedSecret + dsn[at:]
}
