// Package help implements the `stroppy help <topic>` subcommand.
package help

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

// Topic is a help topic with a name, short description, and long content.
type Topic struct {
	Name  string
	Short string
	Long  string
}

var topics []Topic

// Register adds a topic to the help registry.
// Call this from init() functions in topic files.
func Register(t Topic) {
	topics = append(topics, t)
}

// Cmd is the cobra command for `stroppy help [topic]`.
var Cmd = &cobra.Command{
	Use:   "help [topic]",
	Short: "Show help about a topic",
	Long:  `Show extended help about a topic. Run without arguments to list available topics.`,
	// DisableFlagParsing so that topic names like "drivers" are not mistaken for flags.
	Args: cobra.ArbitraryArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			printTopicList()
			return nil
		}

		name := strings.ToLower(args[0])
		for _, t := range topics {
			if t.Name == name {
				fmt.Fprint(os.Stdout, t.Long)
				return nil
			}
		}

		fmt.Fprintf(os.Stderr, "stroppy help: unknown topic %q\n\n", args[0])
		printTopicList()
		return fmt.Errorf("unknown help topic: %s", args[0])
	},
}

func printTopicList() {
	fmt.Fprint(os.Stdout, "Available help topics:\n\n")
	for _, t := range topics {
		fmt.Fprintf(os.Stdout, "  %-20s %s\n", t.Name, t.Short)
	}
	fmt.Fprint(os.Stdout, "\nUse 'stroppy help <topic>' for details.\n")
}
