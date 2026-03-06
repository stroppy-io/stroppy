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
	sqlName    string // basename of SQL file in temp dir (empty if no SQL)
	tempDir    string
	config     *Probeprint
	k6RunArgs  []string // pass args directly to 'k6 run <k6RunArgs>'
	filesInTmp []string
	steps      []string // --steps: only run these steps
	noSteps    []string // --no-steps: skip these steps
}

// NewScriptRunner creates a new ScriptRunner for the given resolved input.
func NewScriptRunner(input *ResolvedInput, k6RunArgs, steps, noSteps []string) (*ScriptRunner, error) {
	lg := logger.Global().
		Named("script_runner").
		WithOptions(zap.WithCaller(false), zap.AddStacktrace(zap.FatalLevel))

	lg.Info("Resolved script",
		zap.String("name", input.Script.Name),
		zap.String("source", input.Script.Source.String()),
	)

	if input.SQL != nil {
		lg.Info("Resolved SQL",
			zap.String("name", input.SQL.Name),
			zap.String("source", input.SQL.Source.String()),
		)
	}

	// Create temp directory
	tempDir, tmpFiles, err := CreateAndInitTempDir(lg, input)
	if err != nil {
		return nil, fmt.Errorf("error while creating temporary dir: %w", err)
	}

	scriptPath := filepath.Join(tempDir, input.Script.Name)

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

	// Validate --steps / --no-steps against probed steps.
	if err := validateStepNames(config.Steps, steps, "--steps"); err != nil {
		return nil, err
	}

	if err := validateStepNames(config.Steps, noSteps, "--no-steps"); err != nil {
		return nil, err
	}

	// Update logger with config if available
	if config.GlobalConfig.GetLogger() != nil {
		lg = logger.NewFromProtoConfig(config.GlobalConfig.GetLogger()).
			Named("script_runner").
			WithOptions(zap.WithCaller(false))
	}

	lg.Debug("Got k6 args", zap.Strings("k6Args", k6RunArgs))

	sqlName := ""
	if input.SQL != nil {
		sqlName = input.SQL.Name
	}

	return &ScriptRunner{
		logger:     lg,
		scriptPath: scriptPath,
		sqlName:    sqlName,
		config:     config,
		tempDir:    tempDir,
		k6RunArgs:  k6RunArgs,
		filesInTmp: tmpFiles,
		steps:      steps,
		noSteps:    noSteps,
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
	input *ResolvedInput,
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

	// Copy script to temp directory
	if err := writeResolvedFile(input.Script, tempDir); err != nil {
		return "", nil, fmt.Errorf("failed to copy script: %w", err)
	}

	filenames = append(filenames, input.Script.Name)

	// Copy SQL to temp directory if present
	if input.SQL != nil {
		if err := writeResolvedFile(*input.SQL, tempDir); err != nil {
			return "", nil, fmt.Errorf("failed to copy SQL file %q: %w", input.SQL.Name, err)
		}

		filenames = append(filenames, input.SQL.Name)
	}

	return tempDir, filenames, nil
}

// writeResolvedFile writes a ResolvedFile to the target directory.
func writeResolvedFile(rf ResolvedFile, targetDir string) error {
	dest := filepath.Join(targetDir, rf.Name)
	if rf.Content != nil {
		return os.WriteFile(dest, rf.Content, common.FileMode)
	}

	return copyFile(rf.Path, dest)
}

func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}

	return os.WriteFile(dst, data, common.FileMode)
}

var (
	ErrNotADir      = errors.New("is not a directory")
	errNoSteps      = errors.New("script has no steps")
	errUnknownSteps = errors.New("unknown steps")
)

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

	if r.sqlName != "" {
		envs = append(envs, "SQL_FILE="+path.Join(r.tempDir, r.sqlName))
	}

	if len(r.steps) > 0 {
		envs = append(envs, "STROPPY_STEPS="+strings.Join(r.steps, ","))
	}

	if len(r.noSteps) > 0 {
		envs = append(envs, "STROPPY_NO_STEPS="+strings.Join(r.noSteps, ","))
	}

	return envs
}

// validateStepNames checks that all names exist in the probed steps.
func validateStepNames(probed, requested []string, flag string) error {
	if len(requested) == 0 {
		return nil
	}

	if len(probed) == 0 {
		return fmt.Errorf("%s %v: %w", flag, requested, errNoSteps)
	}

	known := make(map[string]struct{}, len(probed))
	for _, s := range probed {
		known[s] = struct{}{}
	}

	var unknown []string

	for _, s := range requested {
		if _, ok := known[s]; !ok {
			unknown = append(unknown, s)
		}
	}

	if len(unknown) > 0 {
		return fmt.Errorf("%s %v (available: %v): %w", flag, unknown, probed, errUnknownSteps)
	}

	return nil
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
