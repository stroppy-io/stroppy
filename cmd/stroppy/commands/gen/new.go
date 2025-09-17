package gen

import (
	"cmp"
	"fmt"
	"os"
	"path"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/thediveo/enumflag"
	"go.uber.org/zap"

	"github.com/stroppy-io/stroppy/pkg/core/logger"

	configCmd "github.com/stroppy-io/stroppy/cmd/stroppy/commands/config"
	"github.com/stroppy-io/stroppy/internal/common"
	"github.com/stroppy-io/stroppy/internal/config"
	"github.com/stroppy-io/stroppy/internal/static"
)

const (
	configNewWorkdirFlagName = "workdir"
	configNewFormatFlagName  = "format"
	configNewDevFlagName     = "dev"
)

var Cmd = &cobra.Command{ //nolint: gochecknoglobals
	Use:   "new",
	Short: "Generate default stroppy workdir",
	Long: `
This command generates default stroppy workdir structure include config file,
k6 script template, ts requirements and Makefile for run.
`,
	RunE: func(cmd *cobra.Command, _ []string) error {
		log := logger.Global().WithOptions(zap.WithCaller(false))
		output, err := cmd.Flags().GetString(configNewWorkdirFlagName)
		if err != nil {
			return err
		}
		format, err := config.NewFormatFromString(cmd.PersistentFlags().Lookup(configNewFormatFlagName).Value.String())
		if err != nil {
			return err
		}
		example := config.NewExampleConfig()

		runConfStr, err := configCmd.MarshalConfig(example, format.FormatConfigName(configCmd.DefaultConfigName))
		if err != nil {
			return err
		}
		err = os.MkdirAll(output, common.FolderMode)
		if err != nil {
			return err
		}
		err = os.WriteFile(
			path.Join(output, format.FormatConfigName(configCmd.DefaultConfigName)),
			runConfStr,
			common.FileMode,
		)
		if err != nil {
			return err
		}

		log.Info("Config generated! Happy benchmarking!", zap.String(
			"config_path",
			path.Join(output, format.FormatConfigName(configCmd.DefaultConfigName)),
		))

		files := static.StaticFiles
		if cmd.PersistentFlags().Lookup(configNewDevFlagName).Value.String() == "true" {
			files = append(files, static.DevStaticFiles...)
		}
		err = static.CopyStaticFilesToPath(output, common.FileMode, files...)
		if err != nil {
			return err
		}

		err = static.CopyStaticFilesToPath(output, common.FileMode)
		if err != nil {
			return err
		}

		// Copy self to workdir
		execPath, err := os.Executable()
		if err != nil {
			return fmt.Errorf("failed to get self binary path: %w", err) //nolint: err113
		}

		pathToWriteItself := path.Join(output, "stroppy")
		absTo, errTo := filepath.Abs(filepath.Clean(pathToWriteItself))
		absFrom, errFrom := filepath.Abs(filepath.Clean(execPath))
		if err = cmp.Or(errTo, errFrom); err != nil {
			return err
		}
		if absTo == absFrom {
			return nil // executable already in correct place
		}

		execBin, err := os.ReadFile(execPath)
		if err != nil {
			return fmt.Errorf("failed to read self binary file: %w", err) //nolint: err113
		}

		err = os.WriteFile(pathToWriteItself, execBin, common.FileMode)
		if err != nil {
			return fmt.Errorf("failed to write self binary file: %w", err)
		}

		err = os.Chmod(pathToWriteItself, common.FolderMode)
		if err != nil {
			return fmt.Errorf("failed to chmod self binary file: %w", err)
		}

		return nil
	},
}

var configFormatFlag config.Format //nolint: gochecknoglobals // allow in cmd as flag

func init() { //nolint: gochecknoinits // allow in cmd
	Cmd.PersistentFlags().String(
		configNewWorkdirFlagName,
		configCmd.DefaultWorkdirPath,
		"work directory",
	)

	Cmd.PersistentFlags().Var(
		enumflag.New(
			&configFormatFlag,
			configNewFormatFlagName,
			config.FormatIDs,
			enumflag.EnumCaseInsensitive,
		),
		configNewFormatFlagName,
		"output config format, json or yaml",
	)

	Cmd.PersistentFlags().Bool(
		configNewDevFlagName,
		false,
		"generate dev environment, includes package.json, Makefile.dev and ts requirements",
	)
}
