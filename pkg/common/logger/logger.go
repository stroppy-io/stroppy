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

	redacted := redactAuthorityUserinfo(dsn)
	redacted = redactMySQLUserinfo(redacted)
	redacted = redactConninfo(redacted)

	return redactQueryValues(redacted)
}

func redactAuthorityUserinfo(dsn string) string {
	schemeEnd := strings.Index(dsn, "://")
	if schemeEnd < 0 {
		return dsn
	}

	start := schemeEnd + len("://")
	end := authorityEnd(dsn, start)

	at := strings.LastIndexByte(dsn[start:end], '@')
	if at < 0 {
		return dsn
	}

	at += start

	colon := strings.IndexByte(dsn[start:at], ':')
	if colon < 0 {
		return dsn
	}

	return dsn[:start+colon+1] + redactedSecret + dsn[at:]
}

func authorityEnd(dsn string, start int) int {
	if delimiter := strings.IndexAny(dsn[start:], "/?#"); delimiter >= 0 {
		return start + delimiter
	}

	return len(dsn)
}

func isConninfoDSN(dsn string) bool {
	return nextConninfoAssignment(dsn, 0).key != ""
}

func redactMySQLUserinfo(dsn string) string {
	if strings.Contains(dsn, "://") {
		return dsn
	}

	at := mysqlUserinfoEnd(dsn)
	if at < 0 {
		at = malformedMySQLUserinfoEnd(dsn)
	}

	if at < 0 {
		return dsn
	}

	colon := strings.IndexByte(dsn[:at], ':')
	if colon < 0 {
		return dsn
	}

	return dsn[:colon+1] + redactedSecret + dsn[at:]
}

func mysqlUserinfoEnd(dsn string) int {
	userinfoEnd := -1

	for index := range dsn {
		if dsn[index] == '@' && hasMySQLEndpoint(dsn, index+1) {
			userinfoEnd = index
		}
	}

	return userinfoEnd
}

func hasMySQLEndpoint(dsn string, start int) bool {
	if start == len(dsn) {
		return false
	}

	if dsn[start] == '/' {
		return true
	}

	protocolEnd := mysqlProtocolEnd(dsn, start)
	if protocolEnd < 0 || protocolEnd+1 >= len(dsn) {
		return false
	}

	addressEnd := strings.IndexByte(dsn[protocolEnd+1:], ')')
	if addressEnd < 0 {
		return false
	}

	addressEnd += protocolEnd + 1

	return addressEnd+1 < len(dsn) && dsn[addressEnd+1] == '/'
}

func mysqlProtocolEnd(dsn string, start int) int {
	for index := start; index < len(dsn); index++ {
		switch dsn[index] {
		case '(':
			if index > start {
				return index
			}
		case '/', '?', '#', '@':
			return -1
		default:
			if unicode.IsSpace(rune(dsn[index])) {
				return -1
			}
		}
	}

	return -1
}

func malformedMySQLUserinfoEnd(dsn string) int {
	colon := strings.IndexByte(dsn, ':')
	if colon < 0 {
		return -1
	}

	at := strings.LastIndexByte(dsn[colon+1:], '@')
	if at < 0 {
		return -1
	}

	at += colon + 1
	if isConninfoDSN(dsn) && !looksLikeMySQLEndpoint(dsn, at+1) {
		return -1
	}

	return at
}

func looksLikeMySQLEndpoint(dsn string, start int) bool {
	for index := start; index < len(dsn); index++ {
		switch dsn[index] {
		case '(', '[', '/':
			return true
		case '?', '#', '@':
			return false
		default:
			if unicode.IsSpace(rune(dsn[index])) {
				return false
			}
		}
	}

	return false
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

type conninfoAssignment struct {
	key                  string
	valueStart, valueEnd int
	next                 int
}

func redactConninfo(dsn string) string {
	var result strings.Builder

	lastRedaction := 0

	for index := 0; index < len(dsn); {
		assignment := nextConninfoAssignment(dsn, index)
		index = assignment.next

		if !isSecretKey(assignment.key) {
			continue
		}

		result.WriteString(dsn[lastRedaction:assignment.valueStart])
		result.WriteString(redactedSecret)

		lastRedaction = assignment.valueEnd
	}

	if lastRedaction == 0 {
		return dsn
	}

	result.WriteString(dsn[lastRedaction:])

	return result.String()
}

func nextConninfoAssignment(dsn string, index int) conninfoAssignment {
	index = skipConninfoSpaces(dsn, index)

	keyStart := index
	keyEnd := conninfoKeyEnd(dsn, index)
	index = skipConninfoSpaces(dsn, keyEnd)

	if keyEnd == keyStart || index == len(dsn) || dsn[index] != '=' || !isConninfoKey(dsn[keyStart:keyEnd]) {
		return conninfoAssignment{next: skipConninfoToken(dsn, keyStart)}
	}

	valueStart := skipConninfoSpaces(dsn, index+1)
	valueEnd := conninfoValueEnd(dsn, valueStart)

	return conninfoAssignment{
		key:        dsn[keyStart:keyEnd],
		valueStart: valueStart,
		valueEnd:   valueEnd,
		next:       valueEnd,
	}
}

func skipConninfoSpaces(dsn string, index int) int {
	for index < len(dsn) && unicode.IsSpace(rune(dsn[index])) {
		index++
	}

	return index
}

func conninfoKeyEnd(dsn string, index int) int {
	for index < len(dsn) && !unicode.IsSpace(rune(dsn[index])) && dsn[index] != '=' {
		index++
	}

	return index
}

func skipConninfoToken(dsn string, index int) int {
	for index < len(dsn) && !unicode.IsSpace(rune(dsn[index])) {
		index++
	}

	return index
}

func isConninfoKey(key string) bool {
	for _, r := range key {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_' && r != '-' {
			return false
		}
	}

	return key != ""
}

func conninfoValueEnd(dsn string, start int) int {
	if start == len(dsn) || dsn[start] != '\'' {
		return unquotedConninfoValueEnd(dsn, start)
	}

	return quotedConninfoValueEnd(dsn, start)
}

func unquotedConninfoValueEnd(dsn string, start int) int {
	for start < len(dsn) {
		if dsn[start] == '\\' && start+1 < len(dsn) {
			start += 2

			continue
		}

		if unicode.IsSpace(rune(dsn[start])) {
			break
		}

		start++
	}

	return start
}

func quotedConninfoValueEnd(dsn string, start int) int {
	for index := start + 1; index < len(dsn); index++ {
		if dsn[index] == '\\' && index+1 < len(dsn) {
			index++

			continue
		}

		if dsn[index] == '\'' {
			index++
			for index < len(dsn) && !unicode.IsSpace(rune(dsn[index])) {
				index++
			}

			return index
		}
	}

	return len(dsn)
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
