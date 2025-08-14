package config

import (
	"os"
	"path"

	"github.com/spf13/cobra"
	"github.com/thediveo/enumflag"
	"go.uber.org/zap"

	"github.com/stroppy-io/stroppy-core/pkg/logger"

	"github.com/stroppy-io/stroppy/internal/common"
	"github.com/stroppy-io/stroppy/internal/config"
)

const (
	configNewWorkdirFlagName = "workdir"
	configNewFormatFlagName  = "format"
)

var NewConfigCmd = &cobra.Command{ //nolint: gochecknoglobals
	Use:   "new --output <output>",
	Short: "Generate default stroppy config",
	Long:  ``,
	RunE: func(cmd *cobra.Command, _ []string) error {
		output, err := cmd.Flags().GetString(configNewWorkdirFlagName)
		if err != nil {
			return err
		}
		format, err := config.NewFormatFromString(cmd.PersistentFlags().Lookup(configNewFormatFlagName).Value.String())
		if err != nil {
			return err
		}

		example := config.NewExampleConfig()

		runConfStr, err := MarshalConfig(example, format.FormatConfigName(DefaultConfigName))
		if err != nil {
			return err
		}

		err = os.MkdirAll(output, common.FolderMode)
		if err != nil {
			return err
		}

		err = os.WriteFile(
			path.Join(output, format.FormatConfigName(DefaultConfigName)),
			runConfStr,
			common.FileMode,
		)
		if err != nil {
			return err
		}

		logger.Global().WithOptions(zap.WithCaller(false)).Info("Config generated! Happy benchmarking!", zap.String(
			"config_path",
			path.Join(output, format.FormatConfigName(DefaultConfigName)),
		))

		return nil
	},
}

var configFormatFlag config.Format //nolint: gochecknoglobals // allow in cmd as flag

func init() { //nolint: gochecknoinits // allow in cmd
	NewConfigCmd.PersistentFlags().String(
		configNewWorkdirFlagName,
		DefaultWorkdirPath,
		"work directory",
	)
	NewConfigCmd.PersistentFlags().Var(
		enumflag.New(
			&configFormatFlag,
			configNewFormatFlagName,
			config.FormatIDs,
			enumflag.EnumCaseInsensitive,
		),
		configNewFormatFlagName,
		"output config format, json or yaml",
	)

	NewConfigCmd.PersistentFlags().Lookup(configNewFormatFlagName).NoOptDefVal = config.FormatJSON.String()
}
