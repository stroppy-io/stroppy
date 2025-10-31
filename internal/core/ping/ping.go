package ping

import (
	"context"
	"time"

	"go.uber.org/zap"

	"github.com/stroppy-io/stroppy-cloud-panel/internal/core/probes"
	"github.com/stroppy-io/stroppy/pkg/core/logger"
)

const (
	loggerName = "probe-ping-task"
)

type ProtoHealth interface {
	Shutdown()
	Resume()
}

func ProtoHealthCheckPing(
	appName string,
	server ProtoHealth,
	probe probes.Probe,
	interval,
	timeout time.Duration,
) context.CancelFunc {
	lg := logger.NewStructLogger(loggerName)
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		lg.Debug("start ping probe task", zap.Duration("interval", interval), zap.Duration("timeout", timeout))
		for {
			select {
			case <-time.After(interval):
				status := probes.WaitProbe(ctx, probe, timeout)
				switch status {
				case probes.FailureProbe:
					lg.Error("failure of probe", zap.String("app", appName))
					server.Shutdown()
				case probes.TimeoutProbe:
					lg.Warn("timeout of probe", zap.String("app", appName))
					server.Shutdown()
				case probes.SuccessProbe:
					lg.Info("success of probe", zap.String("app", appName))
					server.Resume()
				}
			case <-ctx.Done():
				lg.Debug("stop ping probe task")
				return
			}
		}
	}()

	return cancel
}
