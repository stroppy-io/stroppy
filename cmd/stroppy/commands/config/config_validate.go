package config

import (
	"os"

	"github.com/spf13/cobra"
	"go.uber.org/zap"

	"github.com/stroppy-io/stroppy/internal/common"
	"github.com/stroppy-io/stroppy/internal/config"
	"github.com/stroppy-io/stroppy/pkg/common/logger"
	stroppy "github.com/stroppy-io/stroppy/pkg/common/proto"
)

const (
	configFlagName   = "config"
	configSetVersion = "set-version"
)

var validateCmd = &cobra.Command{ //nolint: gochecknoglobals
	Use:   "validate",
	Short: "Validate given config",
	Long:  ``,
	RunE: func(cmd *cobra.Command, _ []string) error {
		log := logger.Global().WithOptions(zap.WithCaller(false))
		configPath, err := cmd.Flags().GetString(configFlagName)
		if err != nil {
			return err
		}

		cfg, err := config.LoadAndValidateConfig(configPath)
		if err != nil {
			return err
		}

		log.Info(
			"Config is valid! Happy benchmarking!",
			zap.String("config_path", configPath),
		)

		setVersion, err := cmd.Flags().GetBool(configSetVersion)
		if err != nil {
			return err
		}
		if setVersion {
			cfg.GetGlobal().Version = stroppy.Version
			runConfStr, err := MarshalConfig(cfg.ConfigFile, configPath)
			if err != nil {
				return err
			}
			err = os.WriteFile(configPath, runConfStr, common.FileMode)
			if err != nil {
				return err
			}
			log.Info(
				"Config version set to current version",
				zap.String("config_path", configPath),
				zap.String("version", stroppy.Version),
			)

		}

		return nil
	},
}

func init() { //nolint: gochecknoinits // allow in cmd

	// TODO: add steps to run with

	validateCmd.PersistentFlags().String(
		configFlagName,
		DefaultConfigFullPath,
		"path to config",
	)

	validateCmd.PersistentFlags().Bool(
		configSetVersion,
		false,
		"set version in config to current version",
	)
}
