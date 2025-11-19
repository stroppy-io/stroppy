package workflow

import (
	"time"

	"github.com/stroppy-io/stroppy-cloud-panel/internal/proto/panel"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type TaskLogger struct {
	taskToWrite *panel.WorkflowTask
	logger      *zap.Logger
}

func NewTaskLogger(
	logger *zap.Logger,
	task *panel.WorkflowTask,
) *TaskLogger {
	return &TaskLogger{
		taskToWrite: task,
		logger: logger.With(
			zap.String("task_id", task.GetId()),
			zap.String("task_type", task.GetTaskType().String()),
		),
	}
}

func (t TaskLogger) logToTask(level zapcore.Level, message string, zapFields ...zap.Field) {
	if !t.logger.Core().Enabled(level) {
		return
	}
	enc := zapcore.NewJSONEncoder(zapcore.EncoderConfig{})
	buffer, err := enc.EncodeEntry(zapcore.Entry{
		Level:   level,
		Message: message,
		Time:    time.Now(),
	}, zapFields)
	if err != nil {
		t.logger.Error("failed to encode log record", zap.Error(err))
		return
	}
	t.taskToWrite.Logs = append(t.taskToWrite.Logs, &panel.LogRecord{
		LogLine: buffer.Bytes(),
	})
}

func (t TaskLogger) Debug(msg string, fields ...zap.Field) {
	fields = append(fields, zap.String("task_status", t.taskToWrite.GetStatus().String()))
	t.logger.Debug(msg, fields...)
	t.logToTask(zapcore.DebugLevel, msg, fields...)
}

func (t TaskLogger) Info(msg string, fields ...zap.Field) {
	fields = append(fields, zap.String("task_status", t.taskToWrite.GetStatus().String()))
	t.logger.Info(msg, fields...)
	t.logToTask(zapcore.InfoLevel, msg, fields...)
}

func (t TaskLogger) Warn(msg string, fields ...zap.Field) {
	fields = append(fields, zap.String("task_status", t.taskToWrite.GetStatus().String()))
	t.logger.Warn(msg, fields...)
	t.logToTask(zapcore.WarnLevel, msg, fields...)
}

func (t TaskLogger) Error(msg string, fields ...zap.Field) {
	fields = append(fields, zap.String("task_status", t.taskToWrite.GetStatus().String()))
	t.logger.Error(msg, fields...)
	t.logToTask(zapcore.ErrorLevel, msg, fields...)
}
