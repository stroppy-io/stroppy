package config

import (
	"fmt"
	"os"
	"path"

	"github.com/spf13/cobra"
	"github.com/thediveo/enumflag"
	"go.uber.org/zap"

	"github.com/stroppy-io/stroppy/internal/common"
	"github.com/stroppy-io/stroppy/internal/config"
	"github.com/stroppy-io/stroppy/pkg/common/logger"
	"github.com/stroppy-io/stroppy/proto/build/go/proto/stroppy"
)

const (
	configNewWorkdirFlagName = "workdir"
	configNewFormatFlagName  = "format"
	configNewNameFlagName    = "name"
	configTPCCFlagName       = "tpcc"
)
const defaultTPCCFactor = 10

var NewConfigCmd = &cobra.Command{ //nolint: gochecknoglobals
	Use:   fmt.Sprintf("new --%s <dirpath>", configNewWorkdirFlagName),
	Short: "Generate default stroppy config",
	Long:  ``,
	RunE: func(cmd *cobra.Command, _ []string) error {
		output, err := cmd.Flags().GetString(configNewWorkdirFlagName)
		if err != nil {
			return err
		}
		format, err := config.NewFormatFromString(
			cmd.PersistentFlags().Lookup(configNewFormatFlagName).Value.String(),
		)
		if err != nil {
			return err
		}
		configName, err := cmd.Flags().GetString(configNewNameFlagName)
		if err != nil {
			return err
		}

		isTpcc := cmd.Flags().Changed(configTPCCFlagName)
		warehouseMax, err := cmd.Flags().GetUint(configTPCCFlagName)
		if err != nil {
			return err
		}
		var protoConfig *stroppy.ConfigFile
		if isTpcc { // TODO: proper --preset tpcc|simple|etc.. option
			protoConfig = config.NewTPCCConfig(int32(warehouseMax))
		} else {
			protoConfig = config.NewExampleConfig()
		}

		runConfStr, err := MarshalConfig(protoConfig, format.FormatConfigName(configName))
		if err != nil {
			return err
		}

		err = os.MkdirAll(output, common.FolderMode)
		if err != nil {
			return err
		}

		err = os.WriteFile(
			path.Join(output, format.FormatConfigName(configName)),
			runConfStr,
			common.FileMode,
		)
		if err != nil {
			return err
		}

		logger.Global().
			WithOptions(zap.WithCaller(false)).
			Info("Config generated! Happy benchmarking!", zap.String(
				"config_path",
				path.Join(output, format.FormatConfigName(configName)),
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
	NewConfigCmd.PersistentFlags().String(
		configNewNameFlagName,
		DefaultConfigName,
		"name of the config file",
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
	NewConfigCmd.PersistentFlags().
		Uint(configTPCCFlagName, defaultTPCCFactor, "whether to use tpc-c test preset and W factor")
}
