package httpserv

import "go.uber.org/zap"

type corsLogger struct {
	lg *zap.SugaredLogger
}

func newCorsLogger(lg *zap.Logger) *corsLogger {
	return &corsLogger{
		lg: lg.Sugar(),
	}
}

func (lg *corsLogger) Printf(format string, args ...interface{}) {
	lg.lg.Debugf(format, args...)
}
