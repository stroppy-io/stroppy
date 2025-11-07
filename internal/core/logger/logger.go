package logger

import (
	"os"
	"strings"
	"sync/atomic"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type LogMod string

const (
	DevelopmentMod LogMod = "development"
	ProductionMod  LogMod = "production"
)

type Config struct {
	LogMod   LogMod `default:"production" mapstructure:"mod"   validate:"oneof=production development"`
	LogLevel string `default:"info"       mapstructure:"level" validate:"oneof=debug info warn error"`
}

var globalLogger = atomic.Pointer[zap.Logger]{} //nolint:gochecknoglobals // global logger needed for all app.

func init() { //nolint: gochecknoinits // allow
	setGlobalLogger(newDefault())
}

// newDefault creates new default logger.
func newDefault(opts ...zap.Option) *zap.Logger {
	cfg := newZapCfg(DevelopmentMod, zapcore.DebugLevel)
	logger, _ := cfg.Build(opts...)

	return logger
}

// newZapCfg creates new zap config.
func newZapCfg(mod LogMod, logLevel zapcore.Level) zap.Config {
	var cfg zap.Config

	switch mod {
	case ProductionMod:
		cfg = zap.NewProductionConfig()
		cfg.Level.SetLevel(logLevel)
	case DevelopmentMod:
		cfg = zap.NewDevelopmentConfig()
	default:
		cfg = zap.NewDevelopmentConfig()
	}

	return cfg
}

// NewFromConfig creates new logger from config.
func NewFromConfig(cfg *Config, opts ...zap.Option) *zap.Logger {
	level, parseErr := zapcore.ParseLevel(cfg.LogLevel)
	if parseErr != nil {
		panic(parseErr)
	}

	zapCfg := newZapCfg(cfg.LogMod, level)

	logger, err := zapCfg.Build(opts...)
	if err != nil {
		panic(err)
	}

	setGlobalLogger(logger)

	return Global()
}

const (
	envLogLevel = "LOG_LEVEL"
	envLogMod   = "LOG_MODE"
)

func SetLoggerEnv(level zapcore.Level, mod LogMod) {
	os.Setenv(envLogLevel, strings.ToLower(level.String()))
	os.Setenv(envLogMod, strings.ToLower(string(mod)))
}

func PrepareLoggerEnvs(level zapcore.Level, mod LogMod) []string {
	return []string{
		envLogLevel + "=" + strings.ToLower(level.String()),
		envLogMod + "=" + strings.ToLower(string(mod)),
	}
}

func NewFromEnv(opts ...zap.Option) *zap.Logger {
	cfg := &Config{
		LogLevel: os.Getenv(envLogLevel),
		LogMod:   LogMod(os.Getenv(envLogMod)),
	}

	return NewFromConfig(cfg, opts...)
}

func setGlobalLogger(logger *zap.Logger) {
	globalLogger.Store(logger)
}

// Global returns the global logger.
func Global() *zap.Logger {
	return globalLogger.Load()
}

// StructLogger is an alias for *zap.Logger included in project struct.
type StructLogger = *zap.Logger

// NewStructLogger returns a new StructLogger with the given name.
func NewStructLogger(name string) StructLogger {
	return Global().Named(name)
}
