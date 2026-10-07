package bench

import (
	"errors"
	"io"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Logger is a concurrency-safe structured logger. Logging does not choose an
// execution outcome; return an error or Fatal to change control flow.
type Logger struct{ backend *zap.Logger }

// LoggerFromBackend adapts implementation-side logging without exposing it in run requests.
func LoggerFromBackend(log *zap.Logger) Logger { return Logger{backend: log} }

var errOddFields = errors.New("fields must be nonempty string/value pairs")

// NewLogger writes structured JSON records to a caller-owned writer.
func NewLogger(output io.Writer) Logger {
	encoder := zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig())

	return Logger{zap.New(zapcore.NewCore(encoder, zapcore.Lock(zapcore.AddSync(output)), zapcore.DebugLevel))}
}

// With returns a logger with copied additional fields.
func (l Logger) With(fields ...any) Logger {
	if l.backend == nil {
		return l
	}

	return Logger{l.backend.With(fieldPairs(fields)...)}
}

func (l Logger) Debug(message string, fields ...any) {
	if l.backend != nil {
		l.backend.Debug(message, fieldPairs(fields)...)
	}
}

func (l Logger) Info(message string, fields ...any) {
	if l.backend != nil {
		l.backend.Info(message, fieldPairs(fields)...)
	}
}

func (l Logger) Warn(message string, fields ...any) {
	if l.backend != nil {
		l.backend.Warn(message, fieldPairs(fields)...)
	}
}

func (l Logger) Error(message string, fields ...any) {
	if l.backend != nil {
		l.backend.Error(message, fieldPairs(fields)...)
	}
}

func fieldPairs(fields []any) []zap.Field {
	if len(fields)%tagPairSize != 0 {
		invalid("logger", errOddFields)
	}

	out := make([]zap.Field, 0, len(fields)/tagPairSize)
	for i := 0; i < len(fields); i += tagPairSize {
		key, ok := fields[i].(string)
		if !ok || key == "" {
			invalid("logger", errOddFields)
		}

		out = append(out, zap.Any(key, fields[i+1]))
	}

	return out
}
