package k6run

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"time"

	"go.uber.org/zap"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/stroppy-io/stroppy/pkg/core/logger"
	stroppy "github.com/stroppy-io/stroppy/pkg/core/proto"
	"github.com/stroppy-io/stroppy/pkg/core/shutdown"
	"github.com/stroppy-io/stroppy/pkg/core/utils"
)

func runK6Binary(
	_ context.Context,
	lg *zap.Logger,
	binaryPath string,
	args ...string,
) error {
	binExec := exec.Command(binaryPath, args...)
	binExec.Stdout = os.Stdout
	binExec.Stderr = os.Stderr

	if err := binExec.Start(); err != nil {
		return fmt.Errorf("fail run k6 binary %s: %w", binaryPath, err)
	}

	shutdown.RegisterFn(func() {
		// Send a termination signal to the process
		if err := binExec.Process.Signal(syscall.SIGTERM); err != nil {
			lg.Error(
				"error sending SIGTERM to k6 binary",
				zap.String("binary_path", binaryPath),
				zap.Error(err),
			)
		}
		// Wait for the process to terminate gracefully
		time.Sleep(1 * time.Second)

		if binExec.ProcessState == nil || !binExec.ProcessState.Exited() {
			lg.Error(
				"k6 binary did not terminate gracefully, forcefully killing...",
				zap.String("binary_path", binaryPath),
			)

			if err := binExec.Process.Kill(); err != nil {
				lg.Error(
					"Error forcefully killing %s: %s",
					zap.String("binary_path", binaryPath),
					zap.Error(err),
				)
			}
		}
	})

	return binExec.Wait()
}

var (
	ErrRunContextIsNil = errors.New("run context is nil")
	ErrStepIsNil       = errors.New("step is nil")
	ErrConfigIsNil     = errors.New("config is nil")
)

func RunStep(
	ctx context.Context,
	lg *zap.Logger,
	runContext *stroppy.StepContext,
) error {
	currentLogger := lg.Named("K6Executor")

	if runContext == nil {
		return ErrRunContextIsNil
	}

	if runContext.GetStep() == nil {
		return ErrStepIsNil
	}

	if runContext.GetGlobalConfig().GetRun().GetK6Executor() == nil {
		return ErrConfigIsNil
	}

	contextStr, err := protojson.Marshal(runContext)
	if err != nil {
		return err
	}

	baseArgs := []string{
		"run",
		runContext.GetGlobalConfig().GetRun().GetK6Executor().GetK6ScriptPath(),
		"-econtext=" + string(contextStr),
	}

	if runContext.GetGlobalConfig().GetRun().GetK6Executor().GetOtlpExport() != nil {
		baseArgs = k6ArgsOtelExport(runContext, baseArgs)
	}

	baseArgs = append(
		baseArgs,
		runContext.GetGlobalConfig().GetRun().GetK6Executor().GetK6BinaryArgs()...,
	)
	currentLogger.Debug("Running K6", zap.String("args", fmt.Sprintf("%v", baseArgs)))
	logger.SetLoggerEnv(
		logger.LevelFromProtoConfig(
			runContext.GetGlobalConfig().GetRun().GetLogger().GetLogLevel(),
		),
		logger.ModeFromProtoConfig(runContext.GetGlobalConfig().GetRun().GetLogger().GetLogMode()),
	)

	return runK6Binary(
		ctx,
		currentLogger,
		runContext.GetGlobalConfig().GetRun().GetK6Executor().GetK6BinaryPath(),
		baseArgs...,
	)
}

// k6ArgsOtelExport setups k6 OpenTelemetry exporter.
// Docs: https://grafana.com/docs/k6/latest/results-output/real-time/opentelemetry/#opentelemetry
func k6ArgsOtelExport(runContext *stroppy.StepContext, baseArgs []string) []string {
	export := runContext.GetGlobalConfig().GetRun().GetK6Executor().GetOtlpExport()
	os.Setenv(
		"K6_OTEL_METRIC_PREFIX",
		utils.StringOrDefault(
			export.GetOtlpMetricsPrefix(),
			"k6_",
		),
	)
	os.Setenv(
		"K6_OTEL_SERVICE_NAME",
		fmt.Sprintf("%s_%s",
			runContext.GetGlobalConfig().GetBenchmark().GetName(),
			runContext.GetStep().GetName()),
	)

	insecure := "false" // secure by default
	if export.GetOtlpEndpointInsecure() {
		insecure = "true"
	}

	if export.GetOtlpGrpcEndpoint() != "" {
		os.Setenv("K6_OTEL_GRPC_EXPORTER_INSECURE", insecure)
		os.Setenv("K6_OTEL_GRPC_EXPORTER_ENDPOINT", "localhost:4317")
	} else {
		os.Setenv("K6_OTEL_HTTP_EXPORTER_INSECURE", insecure)
		os.Setenv("K6_OTEL_HTTP_EXPORTER_ENDPOINT", utils.StringOrDefault(
			export.GetOtlpHttpEndpoint(),
			"localhost:4318",
		))
		os.Setenv("K6_OTEL_HTTP_EXPORTER_URL_PATH", utils.StringOrDefault(
			export.GetOtlpHttpExporterUrlPath(),
			"/v1/metrics",
		))
	}

	baseArgs = append(baseArgs, "--out", "experimental-opentelemetry")

	return baseArgs
}
