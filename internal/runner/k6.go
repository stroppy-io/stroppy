package runner

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path"
	"syscall"
	"time"

	"go.uber.org/zap"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/stroppy-io/stroppy/internal/common"
	"github.com/stroppy-io/stroppy/internal/static"
	"github.com/stroppy-io/stroppy/pkg/common/logger"
	stroppy "github.com/stroppy-io/stroppy/pkg/common/proto"
	"github.com/stroppy-io/stroppy/pkg/common/shutdown"
	"github.com/stroppy-io/stroppy/pkg/utils"
)

func runK6Binary(
	_ context.Context,
	lg *zap.Logger,
	workdir, binaryPath string,
	args, envs []string,
) error {
	binExec := exec.Cmd{
		Env:  envs,
		Path: binaryPath,
		Args: append([]string{binaryPath}, args...), // avoid to strip first arg
		Dir:  workdir,
	}
	binExec.Stdout = os.Stdout
	binExec.Stderr = os.Stderr

	if err := binExec.Start(); err != nil {
		return fmt.Errorf("fail run k6 binary %s: %w", binaryPath, err)
	}

	lg.Debug("Exec k6", zap.String("cmd", binExec.String()))

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

func RunStepInK6(
	ctx context.Context,
	lg *zap.Logger,
	runContext *stroppy.StepContext,
) error {
	tempDir, err := os.MkdirTemp(os.TempDir(), fmt.Sprintf("stroppy-k6-%s-%s",
		runContext.GetConfig().GetRunId(),
		runContext.GetStep().GetName()))
	if err != nil {
		return fmt.Errorf("failed to create temp directory: %w", err)
	}

	err = static.CopyStaticFilesToPath(
		tempDir,
		common.FileMode,
		static.K6BenchmarkFileName,
		static.K6PluginFileName,
		static.ProtoTSFileName,
		static.ProtoJSFileName,
	)
	if err != nil {
		return fmt.Errorf("failed to copy static files: %w", err)
	}

	lg.Info("Working dir is: ", zap.String("tmp_dir", tempDir))

	defer os.RemoveAll(tempDir)

	contextStr, err := protojson.Marshal(runContext)
	if err != nil {
		return err
	}

	baseArgs := []string{"run", static.K6BenchmarkFileName.String()}
	envs := []string{"context=" + string(contextStr)}

	if runContext.GetExporter().GetOtlpExport() != nil {
		baseArgs, envs = k6ArgsOtelExport(runContext, baseArgs, envs)
	}

	baseArgs = append(
		baseArgs,
		runContext.GetExecutor().GetK6().GetK6Args()...,
	)

	lg.Debug("Running K6", zap.Any("args", baseArgs))

	envs = append(envs,
		logger.PrepareLoggerEnvs(
			logger.LevelFromProtoConfig(runContext.GetConfig().GetLogger().GetLogLevel()),
			logger.ModeFromProtoConfig(runContext.GetConfig().GetLogger().GetLogMode()),
		)...,
	)

	return runK6Binary(
		ctx,
		lg,
		tempDir,
		path.Join(tempDir, static.K6PluginFileName.String()),
		baseArgs,
		envs,
	)
}

// k6ArgsOtelExport setups k6 OpenTelemetry exporter.
// Docs: https://grafana.com/docs/k6/latest/results-output/real-time/opentelemetry/#opentelemetry
//
//nolint:nonamedreturns // an unnamed returns are confusing and trigers other linter
func k6ArgsOtelExport(
	runContext *stroppy.StepContext,
	baseArgs, envs []string,
) (argsOut, envsOut []string) {
	export := runContext.GetExporter().GetOtlpExport()
	if export == nil {
		return baseArgs, envs
	}

	envs = append(
		envs,
		"K6_OTEL_METRIC_PREFIX="+utils.StringOrDefault(export.GetOtlpMetricsPrefix(), "k6_"),
		"K6_OTEL_SERVICE_NAME=stroppy")
	// TODO: do we really need to add benchmark and step name?
	// fmt.Sprintf("stroppy_%s_%s",
	// runContext.GetConfig().GetBenchmark().GetName(),
	// runContext.GetStep().GetName()),

	insecure := "false" // secure by default
	if export.GetOtlpEndpointInsecure() {
		insecure = "true"
	}

	if export.GetOtlpHeaders() != "" {
		envs = append(envs, "K6_OTEL_HEADERS="+export.GetOtlpHeaders())
	}

	if export.GetOtlpGrpcEndpoint() != "" {
		envs = append(envs,
			"K6_OTEL_GRPC_EXPORTER_INSECURE="+insecure,
			"K6_OTEL_GRPC_EXPORTER_ENDPOINT=localhost:4317")
	} else {
		envs = append(envs, "K6_OTEL_EXPORTER_TYPE=http",
			"K6_OTEL_HTTP_EXPORTER_INSECURE="+insecure,
			"K6_OTEL_HTTP_EXPORTER_ENDPOINT="+
				utils.StringOrDefault(export.GetOtlpHttpEndpoint(), "localhost:4318"),
			"K6_OTEL_HTTP_EXPORTER_URL_PATH="+
				utils.StringOrDefault(export.GetOtlpHttpExporterUrlPath(), "/v1/metrics"),
		)
	}

	baseArgs = append(baseArgs, "--out", "experimental-opentelemetry")

	return baseArgs, envs
}
