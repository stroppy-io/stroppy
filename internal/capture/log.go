package capture

import (
	"encoding/json"
	"strings"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/stroppy-io/stroppy/v6/pkg/report"
)

type logCore struct {
	zapcore.LevelEnabler
	writer  *Writer
	encoder zapcore.Encoder
}

// Logger tees run-owned structured logs without changing output or effective level.
func (w *Writer) Logger(original *zap.Logger) *zap.Logger {
	if s := w.streams["logs"]; s == nil || !s.stats.Enabled {
		return original
	}

	encoder := zapcore.NewJSONEncoder(zapcore.EncoderConfig{MessageKey: "", TimeKey: "", LevelKey: "", NameKey: ""})

	return original.WithOptions(zap.WrapCore(func(core zapcore.Core) zapcore.Core {
		return zapcore.NewTee(core, &logCore{LevelEnabler: core, writer: w, encoder: encoder})
	}))
}

func (c *logCore) With(fields []zapcore.Field) zapcore.Core {
	cloned := c.encoder.Clone()
	for i := range fields {
		fields[i].AddTo(cloned)
	}

	return &logCore{LevelEnabler: c.LevelEnabler, writer: c.writer, encoder: cloned}
}

//nolint:gocritic // zapcore.Core requires value Entry arguments.
func (c *logCore) Check(entry zapcore.Entry, checked *zapcore.CheckedEntry) *zapcore.CheckedEntry {
	if c.Enabled(entry.Level) {
		return checked.AddCore(entry, c)
	}

	return checked
}

//nolint:gocritic // zapcore.Core requires value Entry arguments.
func (c *logCore) Write(entry zapcore.Entry, fields []zapcore.Field) error {
	if !c.writer.Enabled("logs") {
		c.writer.Dropped("logs")

		return nil
	}

	buffer, err := c.encoder.EncodeEntry(entry, fields)
	if err != nil {
		return err
	}
	defer buffer.Free()

	now := entry.Time
	if now.IsZero() {
		now = time.Now()
	}

	c.writer.Add("logs", report.CapturedLog{
		Time: now.UTC(), ElapsedSeconds: c.writer.Elapsed(now),
		CaptureID: c.writer.ID(), ReportID: c.writer.ReportID(),
		Level: entry.Level.String(), Logger: entry.LoggerName, Message: entry.Message,
		Fields: json.RawMessage(strings.TrimSpace(buffer.String())),
	})

	return nil
}
func (*logCore) Sync() error { return nil }
