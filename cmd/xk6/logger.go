package xk6

import (
	"io"

	zaphook "github.com/Sytten/logrus-zap-hook"
	"github.com/sirupsen/logrus"
	"go.uber.org/zap"
)

func NewZapFieldLogger(lg *zap.Logger) logrus.FieldLogger { //nolint:ireturn
	log := logrus.New()
	log.ReportCaller = true   // So Zap reports the right caller
	log.SetOutput(io.Discard) // Prevent logrus from writing its logs

	hook, _ := zaphook.NewZapHook(lg)

	log.AddHook(hook)

	return log
}
