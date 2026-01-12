package runner

import (
	"cmp"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"syscall"
	"time"

	"go.uber.org/zap"

	"github.com/stroppy-io/stroppy/internal/common"
	"github.com/stroppy-io/stroppy/internal/static"
	"github.com/stroppy-io/stroppy/pkg/common/logger"
	stroppy "github.com/stroppy-io/stroppy/pkg/common/proto/stroppy"
	"github.com/stroppy-io/stroppy/pkg/common/shutdown"
)

// ScriptRunner runs TypeScript benchmark scripts with k6.
type ScriptRunner struct {
	logger     *zap.Logger
	scriptPath string
	sqlPath    string // optional SQL file path
	config     *ExtractedConfig
}

// NewScriptRunner creates a new ScriptRunner for the given script.
func NewScriptRunner(scriptPath, sqlPath string) (*ScriptRunner, error) {
	lg := logger.Global().
		Named("script_runner").
		WithOptions(zap.WithCaller(false), zap.AddStacktrace(zap.FatalLevel))

	// Validate script path
	if _, err := os.Stat(scriptPath); err != nil {
		return nil, fmt.Errorf("script file not found: %w", err)
	}

	// Validate SQL path if provided
	if sqlPath != "" {
		if _, err := os.Stat(sqlPath); err != nil {
			return nil, fmt.Errorf("SQL file %q not found: %w", sqlPath, err)
		}
	}

	// Extract config from script
	config, err := ExtractConfigFromScript(scriptPath)
	if err != nil {
		// Config extraction is optional - script might not call defineConfig
		lg.Warn("Could not extract config from script", zap.Error(err))

		config = &ExtractedConfig{
			GlobalConfig: &stroppy.GlobalConfig{},
		}
	}

	// Update logger with config if available
	if config.GlobalConfig.GetLogger() != nil {
		lg = logger.NewFromProtoConfig(config.GlobalConfig.GetLogger()).
			Named("script_runner").
			WithOptions(zap.WithCaller(false))
	}

	return &ScriptRunner{
		logger:     lg,
		scriptPath: scriptPath,
		sqlPath:    sqlPath,
		config:     config,
	}, nil
}

// Run executes the script with k6.
func (r *ScriptRunner) Run(ctx context.Context) error {
	// Create temp directory
	tempDir, err := os.MkdirTemp(os.TempDir(), "stroppy-k6-")
	if err != nil {
		return fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer os.RemoveAll(tempDir)

	r.logger.Info("Working directory", zap.String("path", tempDir))

	// Copy static files to temp directory
	if err := static.CopyAllStaticFilesToPath(tempDir, common.FileMode); err != nil {
		return fmt.Errorf("failed to copy static files: %w", err)
	}

	// Copy user's script to temp directory
	scriptName := filepath.Base(r.scriptPath)
	sqlName := filepath.Base(r.sqlPath)

	if r.sqlPath == "" { // copy single ts file
		if err := copyFile(r.scriptPath, path.Join(tempDir, scriptName)); err != nil {
			return fmt.Errorf("failed to copy script: %w", err)
		}
	} else { // copy ts + sql + add name in variable
		if err := copyFileWithPrepend(
			r.scriptPath,
			path.Join(tempDir, scriptName),
			fmt.Sprintf(`const __SQL_FILE = %q;`, sqlName),
		); err != nil {
			return fmt.Errorf("failed to copy script: %w", err)
		}

		if err := copyFile(r.sqlPath, path.Join(tempDir, sqlName)); err != nil {
			return fmt.Errorf("failed to copy SQL file %q: %w", sqlName, err)
		}
	}

	args := []string{"run", scriptName}

	envs := r.buildEnvVars()

	if r.config.GlobalConfig.GetExporter().GetOtlpExport() != nil {
		args, envs = r.addOtelExportArgs(args, envs)
	}

	r.logger.Debug("Running k6", zap.Strings("args", args))

	return r.runK6Binary(ctx, tempDir, args, envs)
}

// copyFile copies a file from src to dst.
func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}

	return os.WriteFile(dst, data, common.FileMode)
}

func copyFileWithPrepend(src, dst, prepend string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}

	return os.WriteFile(dst, append([]byte(prepend), data...), common.FileMode)
}

// buildEnvVars builds environment variables for k6 execution.
func (r *ScriptRunner) buildEnvVars() []string {
	envs := os.Environ() // inherit parent environment

	// Add logger configuration
	if r.config.GlobalConfig.GetLogger() != nil {
		loggerEnvs := logger.PrepareLoggerEnvs(
			logger.LevelFromProtoConfig(r.config.GlobalConfig.GetLogger().GetLogLevel()),
			logger.ModeFromProtoConfig(r.config.GlobalConfig.GetLogger().GetLogMode()),
		)
		envs = append(envs, loggerEnvs...)
	}

	return envs
}

// addOtelExportArgs adds OpenTelemetry exporter arguments and environment variables.
//
//nolint:nonamedreturns // required by gocritic unnamedResult
func (r *ScriptRunner) addOtelExportArgs(args, envs []string) (argsOut, envsOut []string) {
	export := r.config.GlobalConfig.GetExporter().GetOtlpExport()
	if export == nil {
		return args, envs
	}

	envs = append(envs,
		"K6_OTEL_METRIC_PREFIX="+cmp.Or(export.GetOtlpMetricsPrefix(), "k6_"),
		"K6_OTEL_SERVICE_NAME=stroppy",
	)

	insecure := "false"
	if export.GetOtlpEndpointInsecure() {
		insecure = "true"
	}

	if export.GetOtlpHeaders() != "" {
		envs = append(envs, "K6_OTEL_HEADERS="+export.GetOtlpHeaders())
	}

	if export.GetOtlpGrpcEndpoint() != "" {
		envs = append(envs,
			"K6_OTEL_GRPC_EXPORTER_INSECURE="+insecure,
			"K6_OTEL_GRPC_EXPORTER_ENDPOINT="+export.GetOtlpGrpcEndpoint(),
		)
	} else {
		envs = append(envs,
			"K6_OTEL_EXPORTER_TYPE=http",
			"K6_OTEL_HTTP_EXPORTER_INSECURE="+insecure,
			"K6_OTEL_HTTP_EXPORTER_ENDPOINT="+cmp.Or(export.GetOtlpHttpEndpoint(), "localhost:4318"),
			"K6_OTEL_HTTP_EXPORTER_URL_PATH="+cmp.Or(export.GetOtlpHttpExporterUrlPath(), "/v1/metrics"),
		)
	}

	args = append(args, "--out", "experimental-opentelemetry")

	return args, envs
}

// runK6Binary executes the k6 binary.
func (r *ScriptRunner) runK6Binary(ctx context.Context, workdir string, args, envs []string) error {
	binaryPath := path.Join(workdir, static.K6PluginFileName.String())

	binExec := exec.CommandContext(ctx, binaryPath, args...)
	binExec.Env = envs
	binExec.Dir = workdir
	binExec.Stdout = os.Stdout
	binExec.Stderr = os.Stderr

	if err := binExec.Start(); err != nil {
		return fmt.Errorf("failed to start k6: %w", err)
	}

	r.logger.Debug("Started k6", zap.String("cmd", binExec.String()))

	// Register shutdown handler
	shutdown.RegisterFn(func() {
		if err := binExec.Process.Signal(syscall.SIGTERM); err != nil {
			r.logger.Error("Error sending SIGTERM to k6", zap.Error(err))
		}

		time.Sleep(1 * time.Second)

		if binExec.ProcessState == nil || !binExec.ProcessState.Exited() {
			r.logger.Error("k6 did not terminate gracefully, killing...")

			if err := binExec.Process.Kill(); err != nil {
				r.logger.Error("Error killing k6", zap.Error(err))
			}
		}
	})

	return binExec.Wait()
}
