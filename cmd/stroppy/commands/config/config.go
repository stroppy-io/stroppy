package config

import (
	"github.com/spf13/cobra"
)

var ConfigCommand = &cobra.Command{ //nolint: gochecknoglobals
	Use:   "config",
	Short: "Config commands for stroppy",
	Long:  ``,
}

func init() { //nolint: gochecknoinits // allow in cmd
	ConfigCommand.AddCommand(newConfigCmd)
	ConfigCommand.AddCommand(validateCmd)
}
