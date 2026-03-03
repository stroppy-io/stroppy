package runner

import (
	"cmp"
	"context"
	"errors"
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
	config     *Probeprint
	k6RunArgs  []string // pass args directly to 'k6 run <k6RunArgs>'
	filesInTmp []string
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
	tempDir, tmpFiles, err := CreateAndInitTempDir(lg, scriptPath, sqlPath)
	if err != nil {
		return nil, fmt.Errorf("error while creating temporary dir: %w", err)
	}

	scriptPath = filepath.Join(tempDir, filepath.Base(scriptPath))

	// Extract config from script
	config, err := ProbeScript(scriptPath)
	if err != nil {
		// Config extraction is optional - script might not call defineConfig
		lg.Warn("Could not extract config from script", zap.Error(err))

		config = &Probeprint{
			GlobalConfig: &stroppy.GlobalConfig{},
		}
	}

	lg.Debug("Got config extracted", zap.Any("config", config))

	// Update logger with config if available
	if config.GlobalConfig.GetLogger() != nil {
		lg = logger.NewFromProtoConfig(config.GlobalConfig.GetLogger()).
			Named("script_runner").
			WithOptions(zap.WithCaller(false))
	}

	lg.Debug("Got k6 args", zap.Strings("k6Args", k6RunArgs))

	return &ScriptRunner{
		logger:     lg,
		scriptPath: scriptPath,
		sqlPath:    sqlPath,
		config:     config,
		tempDir:    tempDir,
		k6RunArgs:  k6RunArgs,
		filesInTmp: tmpFiles,
	}, nil
}

// Run executes the script with k6.
func (r *ScriptRunner) Run(ctx context.Context) error {
	// For now it is oneshot run.
	// TODO: multi-run scripts
	defer os.RemoveAll(r.tempDir)

	args := []string{}

	envs := r.buildEnvVars()

	args, envs = r.addOtelExportArgs(args, envs)

	return r.runK6(ctx, args, envs)
}

func CreateAndInitTempDir(
	lg *zap.Logger,
	scriptPath, sqlPath string,
) (tempDir string, filenames []string, err error) {
	tempDir, err = os.MkdirTemp(os.TempDir(), "stroppy-k6-")
	if err != nil {
		return "", nil, fmt.Errorf("failed to create temp directory: %w", err)
	}

	lg.Info("Working directory", zap.String("path", tempDir))

	if err := static.CopyAllStaticFilesToPath(tempDir, common.FileMode); err != nil {
		return "", nil, fmt.Errorf("failed to copy static files: %w", err)
	}

	filenames = append(filenames, common.OutStr(static.StaticFiles)...)

	// Copy user's script to temp directory
	scriptName := filepath.Base(scriptPath)
	sqlName := filepath.Base(sqlPath)

	// copy single ts file
	if err := copyFile(scriptPath, path.Join(tempDir, scriptName)); err != nil {
		return "", nil, fmt.Errorf("failed to copy script: %w", err)
	}

	filenames = append(filenames, scriptName)

	if sqlPath != "" {
		if err := copyFile(sqlPath, path.Join(tempDir, sqlName)); err != nil {
			return "", nil, fmt.Errorf("failed to copy SQL file %q: %w", sqlName, err)
		}

		filenames = append(filenames, sqlName)
	}

	return tempDir, filenames, nil
}

func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}

	return os.WriteFile(dst, data, common.FileMode)
}

var ErrNotADir = errors.New("is not a directory")

func copyFiles(srcDir, dstDir string, excludeNames []string) (copied []string, err error) {
	srcInfo, err := os.Stat(srcDir)
	if err != nil {
		return nil, err
	}

	if !srcInfo.IsDir() {
		return nil, fmt.Errorf("%s: %w", srcDir, ErrNotADir)
	}

	dstInfo, err := os.Stat(dstDir)
	if err != nil {
		return nil, err
	}

	if !dstInfo.IsDir() {
		return nil, fmt.Errorf("%s: %w", dstDir, ErrNotADir)
	}

	entries, err := os.ReadDir(srcDir)
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue // Skip all directories
		}

		if slices.Contains(excludeNames, entry.Name()) {
			continue
		}

		srcPath := filepath.Join(srcDir, entry.Name())
		dstPath := filepath.Join(dstDir, entry.Name())

		if err := copyFile(srcPath, dstPath); err != nil {
			return copied, err
		}

		copied = append(copied, entry.Name())
	}

	return copied, nil
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

	if r.sqlPath != "" {
		envs = append(envs, "SQL_FILE="+path.Join(r.tempDir, filepath.Base(r.sqlPath)))
	}

	return envs
}

// addOtelExportArgs adds OpenTelemetry exporter arguments and environment variables.
func (r *ScriptRunner) addOtelExportArgs(args, envs []string) (argsOut, envsOut []string) {
	export := r.config.GlobalConfig.GetExporter().GetOtlpExport()
	if export == nil {
		r.logger.Debug("Have no OTEL configuration")

		return args, envs
	}

	r.logger.Debug("Got the OTEL configuration", zap.Any("config", export))

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
			"K6_OTEL_EXPORTER_PROTOCOL=grpc",
			"K6_OTEL_GRPC_EXPORTER_INSECURE="+insecure,
			"K6_OTEL_GRPC_EXPORTER_ENDPOINT="+export.GetOtlpGrpcEndpoint(),
		)
	} else {
		envs = append(envs,
			"K6_OTEL_EXPORTER_PROTOCOL=http/protobuf",
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

	os.Args = slices.Concat([]string{"k6", "run"}, r.k6RunArgs, args, []string{scriptName})

	r.logger.Debug("Running k6", zap.Strings("args", os.Args))

	// run the test
	k6cmd.Execute() // TODO: add exit code processing

	copied, err := copyFiles(r.tempDir, dirBefore, r.filesInTmp)
	r.logger.Debug(
		"Files copied back to user dir",
		zap.String("from", r.tempDir),
		zap.String("to", dirBefore),
		zap.Strings("copied", copied),
		zap.Error(err),
	)

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
