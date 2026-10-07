// Package help implements the `stroppy help <topic>` subcommand.
package help

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"
)

var errUnknownTopic = errors.New("unknown help topic")

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
var Cmd = NewCommand()

// NewCommand creates an independent help command.
func NewCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "help [topic]",
		Short: "Show help about a topic",
		Long:  `Show extended help about a topic. Run without arguments to list available topics.`,
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return printTopicList(cmd.OutOrStdout())
			}

			name := strings.ToLower(args[0])
			for _, topic := range topics {
				if topic.Name == name {
					_, err := fmt.Fprint(cmd.OutOrStdout(), topic.Long)

					return err
				}
			}

			fmt.Fprintf(cmd.ErrOrStderr(), "stroppy help: unknown topic %q\n\n", args[0])
			_ = printTopicList(cmd.OutOrStdout())

			return fmt.Errorf("%w: %s", errUnknownTopic, args[0])
		},
	}
}

func printTopicList(output io.Writer) error {
	if _, err := fmt.Fprint(output, "Available help topics:\n\n"); err != nil {
		return err
	}

	for _, topic := range topics {
		if _, err := fmt.Fprintf(output, "  %-20s %s\n", topic.Name, topic.Short); err != nil {
			return err
		}
	}

	_, err := fmt.Fprint(output, "\nUse 'stroppy help <topic>' for details.\n")

	return err
}
