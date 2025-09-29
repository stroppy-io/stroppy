package config

import (
	"github.com/stroppy-io/stroppy/pkg/utils"
	"os"
	"path"

	"github.com/spf13/cobra"
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
