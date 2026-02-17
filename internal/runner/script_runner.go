package runner

import (
	"cmp"
	"context"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	k6cmd "go.k6.io/k6/cmd"
	"go.uber.org/zap"

	"github.com/stroppy-io/stroppy/internal/common"
	"github.com/stroppy-io/stroppy/internal/static"
	"github.com/stroppy-io/stroppy/pkg/common/logger"
	stroppy "github.com/stroppy-io/stroppy/pkg/common/proto/stroppy"
)

// ScriptRunner runs TypeScript benchmark scripts with k6.
type ScriptRunner struct {
	logger     *zap.Logger
	scriptPath string
	sqlPath    string // optional SQL file path
	tempDir    string
	config     *ExtractedConfig
	k6RunArgs  []string
}

// NewScriptRunner creates a new ScriptRunner for the given script.
func NewScriptRunner(scriptPath, sqlPath string, k6RunArgs []string) (*ScriptRunner, error) {
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

	// Create temp directory
	tempDir, err := CreateAndInitTempDir(lg, scriptPath, sqlPath)
	if err != nil {
		return nil, fmt.Errorf("error while creating temporary dir: %w", err)
	}

	scriptPath = filepath.Join(tempDir, filepath.Base(scriptPath))

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
		tempDir:    tempDir,
		k6RunArgs:  k6RunArgs,
	}, nil
}

// Run executes the script with k6.
func (r *ScriptRunner) Run(ctx context.Context) error {
	defer os.RemoveAll(r.tempDir)

	args := []string{}

	envs := r.buildEnvVars()

	if r.config.GlobalConfig.GetExporter().GetOtlpExport() != nil {
		args, envs = r.addOtelExportArgs(args, envs)
	}

	return r.runK6(ctx, args, envs)
}

func CreateAndInitTempDir(
	lg *zap.Logger,
	scriptPath, sqlPath string,
) (tempDir string, err error) {
	tempDir, err = os.MkdirTemp(os.TempDir(), "stroppy-k6-")
	if err != nil {
		return "", fmt.Errorf("failed to create temp directory: %w", err)
	}

	lg.Info("Working directory", zap.String("path", tempDir))

	if err := static.CopyAllStaticFilesToPath(tempDir, common.FileMode); err != nil {
		return "", fmt.Errorf("failed to copy static files: %w", err)
	}

	// Copy user's script to temp directory
	scriptName := filepath.Base(scriptPath)
	sqlName := filepath.Base(sqlPath)

	if sqlPath == "" { // copy single ts file
		if err := copyFile(scriptPath, path.Join(tempDir, scriptName)); err != nil {
			return "", fmt.Errorf("failed to copy script: %w", err)
		}
	} else { // copy ts + sql + add name in variable
		if err := copyFileWithPrepend(
			scriptPath,
			path.Join(tempDir, scriptName),
			fmt.Sprintf(`const __SQL_FILE = %q;`, sqlName),
		); err != nil {
			return "", fmt.Errorf("failed to copy script: %w", err)
		}

		if err := copyFile(sqlPath, path.Join(tempDir, sqlName)); err != nil {
			return "", fmt.Errorf("failed to copy SQL file %q: %w", sqlName, err)
		}
	}

	return tempDir, nil
}

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
		envs = append(envs, // grpc is default http_exporter_type
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

	args = append(args, "--out", "opentelemetry")

	return args, envs
}

// runK6 executes the k6.
func (r *ScriptRunner) runK6(
	_ context.Context,
	args, envs []string,
) error {
	scriptName := filepath.Base(r.scriptPath)
	// dump state
	argsBefore := os.Args
	envsBefore := os.Environ()

	dirBefore, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get working dir: %w", err)
	}

	// set new state
	if err := setEnvs(envs); err != nil {
		return fmt.Errorf("failed to set eniroments for k6: %w", err)
	}

	if err := os.Chdir(r.tempDir); err != nil {
		return fmt.Errorf("failed cd to temporary %q: %w", r.tempDir, err)
	}

	os.Args = slices.Concat([]string{"k6", "run", scriptName}, r.k6RunArgs, args)

	r.logger.Debug("Running k6", zap.Strings("args", os.Args))

	// run the test
	k6cmd.Execute() // TODO: add exit code processing

	// restore state
	os.Clearenv()

	if err := setEnvs(envsBefore); err != nil {
		return fmt.Errorf("failed to restore eniroments: %w", err)
	}

	if err := os.Chdir(dirBefore); err != nil {
		return fmt.Errorf("failed cd origin %q: %w", dirBefore, err)
	}

	os.Args = argsBefore

	return nil
}

// setEnvs set environment variables in [os.Environ] compatible format.
// If env already exists then do nothig.
func setEnvs(envs []string) error {
	for _, env := range envs {
		const keyPlusValue = 2

		kv := strings.SplitN(env, "=", keyPlusValue)

		key, value := kv[0], kv[1]
		if _, present := os.LookupEnv(key); present {
			continue // do not override user envs
		}

		if err := os.Setenv(key, value); err != nil {
			return fmt.Errorf("failed to setenv '%s=%s': %w", key, value, err)
		}
	}

	return nil
}
