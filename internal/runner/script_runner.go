package runner

import (
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
func NewScriptRunner(scriptPath string, sqlPath string) (*ScriptRunner, error) {
	lg := logger.Global().Named("script_runner").WithOptions(zap.WithCaller(false))

	// Validate script path
	if _, err := os.Stat(scriptPath); err != nil {
		return nil, fmt.Errorf("script file not found: %w", err)
	}

	// Validate SQL path if provided
	if sqlPath != "" {
		if _, err := os.Stat(sqlPath); err != nil {
			return nil, fmt.Errorf("SQL file not found: %w", err)
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
	if err := r.copyStaticFiles(tempDir); err != nil {
		return fmt.Errorf("failed to copy static files: %w", err)
	}

	// Copy user's script to temp directory
	scriptName := filepath.Base(r.scriptPath)
	if err := r.copyFile(r.scriptPath, path.Join(tempDir, scriptName)); err != nil {
		return fmt.Errorf("failed to copy script: %w", err)
	}

	// Copy SQL file if explicitly provided
	if r.sqlPath != "" {
		sqlName := filepath.Base(r.sqlPath)
		if err := r.copyFile(r.sqlPath, path.Join(tempDir, sqlName)); err != nil {
			return fmt.Errorf("failed to copy SQL file: %w", err)
		}
	}

	// Copy all SQL files from script's directory (for scripts that use open())
	if err := r.copySQLFilesFromScriptDir(tempDir); err != nil {
		r.logger.Warn("Could not copy SQL files from script directory", zap.Error(err))
	}

	// Build k6 arguments
	args := []string{"run", scriptName}

	// Build environment variables
	envs := r.buildEnvVars()

	// Add OTLP exporter args if configured
	if r.config.GlobalConfig.GetExporter().GetOtlpExport() != nil {
		args, envs = r.addOtelExportArgs(args, envs)
	}

	r.logger.Debug("Running k6", zap.Strings("args", args))

	// Run k6
	return r.runK6Binary(ctx, tempDir, args, envs)
}

// copyStaticFiles copies required static files to the temp directory.
func (r *ScriptRunner) copyStaticFiles(tempDir string) error {
	return static.CopyStaticFilesToPath(
		tempDir,
		common.FileMode,
		static.StaticFiles...,
	)
}

// copyFile copies a file from src to dst.
func (r *ScriptRunner) copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}

	return os.WriteFile(dst, data, common.FileMode)
}

// copySQLFilesFromScriptDir copies all SQL files from the script's directory.
func (r *ScriptRunner) copySQLFilesFromScriptDir(tempDir string) error {
	scriptDir := filepath.Dir(r.scriptPath)

	entries, err := os.ReadDir(scriptDir)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		if filepath.Ext(entry.Name()) == ".sql" {
			srcPath := filepath.Join(scriptDir, entry.Name())

			dstPath := path.Join(tempDir, entry.Name())
			if err := r.copyFile(srcPath, dstPath); err != nil {
				return err
			}
		}
	}

	return nil
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
func (r *ScriptRunner) addOtelExportArgs(args, envs []string) ([]string, []string) {
	export := r.config.GlobalConfig.GetExporter().GetOtlpExport()
	if export == nil {
		return args, envs
	}

	envs = append(envs,
		"K6_OTEL_METRIC_PREFIX="+stringOrDefault(export.GetOtlpMetricsPrefix(), "k6_"),
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
			"K6_OTEL_HTTP_EXPORTER_ENDPOINT="+stringOrDefault(export.GetOtlpHttpEndpoint(), "localhost:4318"),
			"K6_OTEL_HTTP_EXPORTER_URL_PATH="+stringOrDefault(export.GetOtlpHttpExporterUrlPath(), "/v1/metrics"),
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

// stringOrDefault returns the value if non-empty, otherwise the default.
func stringOrDefault(value, defaultValue string) string {
	if value == "" {
		return defaultValue
	}

	return value
}
