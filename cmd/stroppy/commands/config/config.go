package config

import (
	"os"
	"path"

	"github.com/spf13/cobra"

	"github.com/stroppy-io/stroppy-core/pkg/utils"
)

var TopLevelCommand = &cobra.Command{ //nolint: gochecknoglobals
	Use:   "config",
	Short: "Config commands for stroppy",
	Long:  ``,
}

func init() { //nolint: gochecknoinits // allow in cmd
	TopLevelCommand.AddCommand(NewConfigCmd)
	TopLevelCommand.AddCommand(validateCmd)
}

var (
	DefaultConfigFileName = DefaultConfigFormat.FormatConfigName(DefaultConfigName) //nolint: gochecknoglobals
	DefaultWorkdirPath    = utils.Must(os.Getwd())                                  //nolint: gochecknoglobals
	DefaultConfigFullPath = path.Join(                                              //nolint: gochecknoglobals
		DefaultWorkdirPath,
		DefaultConfigFileName,
	)
)
