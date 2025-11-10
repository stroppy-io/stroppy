package logger

import (
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	stroppy "github.com/stroppy-io/stroppy/proto/build/go/proto/stroppy"
)

func LevelFromProtoConfig(config stroppy.LoggerConfig_LogLevel) zapcore.Level {
	switch config {
	case stroppy.LoggerConfig_LOG_LEVEL_DEBUG:
		return zap.DebugLevel
	case stroppy.LoggerConfig_LOG_LEVEL_INFO:
		return zap.InfoLevel
	case stroppy.LoggerConfig_LOG_LEVEL_WARN:
		return zap.WarnLevel
	case stroppy.LoggerConfig_LOG_LEVEL_ERROR:
		return zap.ErrorLevel
	case stroppy.LoggerConfig_LOG_LEVEL_FATAL:
		return zap.FatalLevel
	default:
		return zap.DebugLevel
	}
}

func ModeFromProtoConfig(mode stroppy.LoggerConfig_LogMode) LogMod {
	switch mode {
	case stroppy.LoggerConfig_LOG_MODE_DEVELOPMENT:
		return DevelopmentMod
	case stroppy.LoggerConfig_LOG_MODE_PRODUCTION:
		return ProductionMod
	default:
		return DevelopmentMod
	}
}

func NewFromProtoConfig(config *stroppy.LoggerConfig) *zap.Logger {
	return NewFromConfig(&Config{
		LogMod:   ModeFromProtoConfig(config.GetLogMode()),
		LogLevel: LevelFromProtoConfig(config.GetLogLevel()).String(),
	})
}
