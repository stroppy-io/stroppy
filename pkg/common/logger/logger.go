package logger

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
	"sync/atomic"

	mysql "github.com/go-sql-driver/mysql"
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
	redactedSecret = "xxxxx"
	redactedDSN    = "<redacted-dsn>"
)

// RedactDSN masks credentials in supported connection-string grammars. Inputs
// that cannot be classified unambiguously never appear in diagnostics.
func RedactDSN(dsn string) string {
	if dsn == "" || dsn == redactedDSN {
		return dsn
	}

	if hasLeadingSchemeLike(dsn) {
		if !hasLeadingRFCASCIIScheme(dsn) {
			return redactedDSN
		}

		redacted, ok := redactURI(dsn)
		if !ok {
			return redactedDSN
		}

		if _, mysqlOK := redactMySQL(dsn); mysqlOK {
			return redactedDSN
		}

		return redacted
	}

	assignments, conninfoOK := parseConninfo(dsn)

	mysqlRedacted, mysqlOK := redactMySQL(dsn)
	if conninfoOK {
		if mysqlOK || hasMySQLStructure(dsn) && hasMySQLPasswordEnvelope(dsn) {
			return redactedDSN
		}

		return redactConninfo(dsn, assignments)
	}

	if mysqlOK {
		return mysqlRedacted
	}

	return redactedDSN
}

func hasLeadingSchemeLike(dsn string) bool {
	separator := strings.Index(dsn, "://")
	if separator <= 0 {
		return false
	}

	return !strings.ContainsAny(dsn[:separator], ":@/?#")
}

func hasLeadingRFCASCIIScheme(dsn string) bool {
	separator := strings.Index(dsn, "://")
	if separator <= 0 || !isASCIIAlpha(dsn[0]) {
		return false
	}

	for index := 1; index < separator; index++ {
		value := dsn[index]
		if !isASCIIAlpha(value) && !isASCIIDigit(value) && value != '+' && value != '-' && value != '.' {
			return false
		}
	}

	return true
}

func redactURI(dsn string) (string, bool) {
	parsed, err := url.Parse(dsn)
	if err != nil || parsed.Opaque != "" || parsed.Host == "" {
		return "", false
	}

	values, err := url.ParseQuery(parsed.RawQuery)
	if err != nil {
		return "", false
	}

	for key, values := range values {
		secret, certain := classifyQueryKey(key)
		if !certain {
			return "", false
		}

		if secret {
			for index := range values {
				values[index] = redactedSecret
			}
		}
	}

	parsed.RawQuery = values.Encode()
	if parsed.User != nil {
		if _, present := parsed.User.Password(); present {
			parsed.User = url.UserPassword(parsed.User.Username(), redactedSecret)
		}
	}

	return parsed.Redacted(), true
}

func redactMySQL(dsn string) (redacted string, ok bool) {
	defer func() {
		if recover() != nil {
			redacted, ok = "", false
		}
	}()

	if !hasMySQLStructure(dsn) {
		return "", false
	}

	cfg, err := mysql.ParseDSN(dsn)
	if err != nil {
		return "", false
	}

	passwordPresent := hasMySQLPasswordEnvelope(dsn)
	if cfg.Passwd != "" && !passwordPresent {
		return "", false
	}

	if passwordPresent {
		cfg.Passwd = redactedSecret
	}

	for key := range cfg.Params {
		secret, certain := classifyQueryKey(key)
		if !certain {
			return "", false
		}

		if secret {
			cfg.Params[key] = redactedSecret
		}
	}

	formatted := cfg.FormatDSN()
	if passwordPresent && !strings.Contains(formatted, redactedSecret) {
		return "", false
	}

	return formatted, true
}

type mysqlEnvelope struct {
	userinfoEnd int
	valid       bool
}

func parseMySQLEnvelope(dsn string) mysqlEnvelope {
	databaseSlash := strings.LastIndexByte(dsn, '/')
	if databaseSlash < 0 {
		return mysqlEnvelope{userinfoEnd: -1}
	}

	end := len(dsn)
	if queryStart := strings.IndexByte(dsn[databaseSlash+1:], '?'); queryStart >= 0 {
		end = databaseSlash + queryStart + 1
	}

	for at := databaseSlash - 1; at >= 0; at-- {
		if dsn[at] == '@' && isMySQLEndpoint(dsn, at+1, end) {
			return mysqlEnvelope{userinfoEnd: at, valid: true}
		}
	}

	if isMySQLEndpoint(dsn, 0, end) {
		return mysqlEnvelope{userinfoEnd: -1, valid: true}
	}

	return mysqlEnvelope{userinfoEnd: -1}
}

func isMySQLEndpoint(dsn string, start, end int) bool {
	if start >= end {
		return false
	}

	if dsn[start] == '/' {
		return true
	}

	suffix := dsn[start:end]
	open := strings.IndexByte(suffix, '(')

	slash := strings.IndexByte(suffix, '/')
	if open >= 0 && (slash < 0 || open < slash) {
		closing := strings.IndexByte(suffix[open+1:], ')')
		if closing < 0 {
			return false
		}

		return open+closing+2 < len(suffix) && suffix[open+closing+2] == '/'
	}

	return slash > 0
}

func hasMySQLStructure(dsn string) bool {
	return parseMySQLEnvelope(dsn).valid
}

func hasMySQLPasswordEnvelope(dsn string) bool {
	envelope := parseMySQLEnvelope(dsn)

	return envelope.valid && envelope.userinfoEnd >= 0 && strings.IndexByte(dsn[:envelope.userinfoEnd], ':') >= 0
}

type conninfoAssignment struct {
	key                  string
	valueStart, valueEnd int
}

func parseConninfo(dsn string) ([]conninfoAssignment, bool) {
	index := skipASCIIWhitespace(dsn, 0)
	if index == len(dsn) {
		return nil, false
	}

	assignments := make([]conninfoAssignment, 0)

	for index < len(dsn) {
		keyStart := index
		for index < len(dsn) && dsn[index] != '=' && !isASCIIWhitespace(dsn[index]) {
			if !isConninfoKeyByte(dsn[index]) {
				return nil, false
			}

			index++
		}

		if index == keyStart || index == len(dsn) || dsn[index] != '=' {
			return nil, false
		}

		key := dsn[keyStart:index]
		index++

		valueStart := index

		valueEnd, next, ok := conninfoValueSpan(dsn, index)
		if !ok {
			return nil, false
		}

		assignments = append(assignments, conninfoAssignment{
			key:        key,
			valueStart: valueStart,
			valueEnd:   valueEnd,
		})

		if next == len(dsn) {
			return assignments, true
		}

		if !isASCIIWhitespace(dsn[next]) {
			return nil, false
		}

		index = skipASCIIWhitespace(dsn, next)
	}

	return assignments, true
}

func conninfoValueSpan(dsn string, start int) (end, next int, ok bool) {
	if start < len(dsn) && dsn[start] == '\'' {
		return quotedConninfoValueSpan(dsn, start)
	}

	return unquotedConninfoValueSpan(dsn, start)
}

func quotedConninfoValueSpan(dsn string, start int) (end, next int, ok bool) {
	for index := start + 1; index < len(dsn); index++ {
		if dsn[index] == '\\' {
			if index+1 == len(dsn) {
				return 0, 0, false
			}

			index++

			continue
		}

		if dsn[index] == '\'' {
			return index + 1, index + 1, true
		}
	}

	return 0, 0, false
}

func unquotedConninfoValueSpan(dsn string, start int) (end, next int, ok bool) {
	index := start
	for index < len(dsn) && !isASCIIWhitespace(dsn[index]) {
		if dsn[index] == '\\' {
			if index+1 == len(dsn) {
				return 0, 0, false
			}

			index += 2

			continue
		}

		index++
	}

	return index, index, true
}

func redactConninfo(dsn string, assignments []conninfoAssignment) string {
	var result strings.Builder

	last := 0

	for _, assignment := range assignments {
		if !isSecretName(assignment.key) {
			continue
		}

		result.WriteString(dsn[last:assignment.valueStart])
		result.WriteString(redactedSecret)

		last = assignment.valueEnd
	}

	if last == 0 {
		return dsn
	}

	result.WriteString(dsn[last:])

	return result.String()
}

func skipASCIIWhitespace(dsn string, index int) int {
	for index < len(dsn) && isASCIIWhitespace(dsn[index]) {
		index++
	}

	return index
}

func isASCIIWhitespace(value byte) bool {
	switch value {
	case ' ', '\t', '\n', '\r', '\v', '\f':
		return true
	default:
		return false
	}
}

func isConninfoKeyByte(value byte) bool {
	return isASCIIAlpha(value) || isASCIIDigit(value) || value == '_' || value == '-'
}

func classifyQueryKey(key string) (secret, certain bool) {
	decoded, ok := decodeQueryKey(key)
	if !ok {
		return false, false
	}

	normalized, ok := normalizeSecretName(decoded)
	if !ok {
		return false, false
	}

	return hasSecretSuffix(normalized), true
}

func decodeQueryKey(key string) (string, bool) {
	for range 2 {
		decoded, err := url.QueryUnescape(key)
		if err != nil {
			return "", false
		}

		if decoded == key {
			break
		}

		key = decoded
	}

	if strings.Contains(key, "%") {
		return "", false
	}

	return key, true
}

func isSecretName(name string) bool {
	normalized, ok := normalizeSecretName(name)

	return ok && hasSecretSuffix(normalized)
}

func normalizeSecretName(name string) (string, bool) {
	var normalized strings.Builder

	for index := range len(name) {
		value := name[index]
		switch {
		case isASCIIAlpha(value):
			normalized.WriteByte(toASCIILower(value))
		case isASCIIDigit(value):
			normalized.WriteByte(value)
		case value == '_', value == '-', value == '.':
		default:
			return "", false
		}
	}

	return normalized.String(), true
}

func hasSecretSuffix(name string) bool {
	for _, suffix := range []string{
		"password", "passwd", "pwd", "secret", "token", "credential", "credentials", "apikey",
	} {
		if strings.HasSuffix(name, suffix) {
			return true
		}
	}

	return false
}

func isASCIIAlpha(value byte) bool {
	return value >= 'a' && value <= 'z' || value >= 'A' && value <= 'Z'
}

func isASCIIDigit(value byte) bool {
	return value >= '0' && value <= '9'
}

func toASCIILower(value byte) byte {
	if value >= 'A' && value <= 'Z' {
		return value + ('a' - 'A')
	}

	return value
}
