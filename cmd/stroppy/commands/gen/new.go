package gen

import (
	"cmp"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"go.uber.org/zap"

	"github.com/stroppy-io/stroppy/examples"
	"github.com/stroppy-io/stroppy/internal/common"
	"github.com/stroppy-io/stroppy/internal/static"
	"github.com/stroppy-io/stroppy/pkg/common/logger"
)

const (
	workdirFlagName = "workdir"
	presetFlagName  = "preset"
)

var Cmd = &cobra.Command{ //nolint: gochecknoglobals
	Use:   "new",
	Short: "Generate stroppy development environment",
	Long: fmt.Sprintf(`
Generate a stroppy development environment with TypeScript support.

This command creates a working directory with:
- Proto files (stroppy.pb.js, stroppy.pb.ts)
- Helper files (helpers.ts, parse_sql.ts)
- Package.json for TypeScript types
- K6 binary (stroppy-k6)
- Optional preset example script

Available presets: %s

Examples:
  stroppy gen new --workdir ./my-benchmark
  stroppy gen new --workdir ./my-benchmark --preset tpcc
  stroppy gen new --workdir ./my-benchmark --preset execute_sql
`, strings.Join(examples.AvailablePresets(), ", ")),
	RunE: func(cmd *cobra.Command, _ []string) error {
		log := logger.Global().WithOptions(zap.WithCaller(false))

		output, err := cmd.Flags().GetString(workdirFlagName)
		if err != nil {
			return err
		}

		preset, err := cmd.Flags().GetString(presetFlagName)
		if err != nil {
			return err
		}

		// Create output directory
		err = os.MkdirAll(output, common.FolderMode)
		if err != nil {
			return fmt.Errorf("failed to create output directory: %w", err)
		}

		// Copy static files (protobuf, helpers, parse_sql, k6 binary)
		allFiles := append(static.StaticFiles, static.DevStaticFiles...)
		err = static.CopyStaticFilesToPath(output, common.FileMode, allFiles...)
		if err != nil {
			return fmt.Errorf("failed to copy static files: %w", err)
		}

		log.Info("Static files copied",
			zap.String("path", output),
			zap.Int("files", len(allFiles)),
		)

		// Copy preset if specified
		if preset != "" {
			presetType := examples.Preset(preset)
			err = examples.CopyPresetToPath(output, presetType, common.FileMode)
			if err != nil {
				return fmt.Errorf("failed to copy preset: %w", err)
			}
			log.Info("Preset copied", zap.String("preset", preset))
		}

		// Copy stroppy binary to workdir
		execPath, err := os.Executable()
		if err != nil {
			return fmt.Errorf("failed to get self binary path: %w", err)
		}

		pathToWriteItself := path.Join(output, "stroppy")
		absTo, errTo := filepath.Abs(filepath.Clean(pathToWriteItself))
		absFrom, errFrom := filepath.Abs(filepath.Clean(execPath))
		if err = cmp.Or(errTo, errFrom); err != nil {
			return err
		}

		if absTo != absFrom {
			execBin, err := os.ReadFile(execPath)
			if err != nil {
				return fmt.Errorf("failed to read self binary file: %w", err)
			}

			err = os.WriteFile(pathToWriteItself, execBin, common.FileMode)
			if err != nil {
				return fmt.Errorf("failed to write self binary file: %w", err)
			}

			err = os.Chmod(pathToWriteItself, common.FolderMode)
			if err != nil {
				return fmt.Errorf("failed to chmod self binary file: %w", err)
			}
		}

		log.Info("Development environment generated! Happy benchmarking!",
			zap.String("path", output),
		)

		// Log usage instructions
		log.Info("Files included: stroppy.pb.ts, stroppy.pb.js, helpers.ts, parse_sql.ts, package.json, stroppy-k6, stroppy")

		if preset != "" {
			log.Info("Preset files included", zap.String("preset", preset))
		}

		log.Info("To run your benchmark: cd " + output + " && ./stroppy run <your_script.ts>")
		log.Info("For development with TypeScript types: npm install")

		return nil
	},
}

func init() { //nolint: gochecknoinits // allow in cmd
	Cmd.PersistentFlags().String(
		workdirFlagName,
		".",
		"output directory for development environment",
	)

	Cmd.PersistentFlags().String(
		presetFlagName,
		"",
		fmt.Sprintf("preset example to include (%s)", strings.Join(examples.AvailablePresets(), ", ")),
	)
}
