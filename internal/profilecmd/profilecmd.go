package profilecmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/stroppy-io/stroppy/v6/pkg/profiling"
)

type sessionKey struct{}

var errFlagValue = errors.New("profiling flag requires a value")

var environmentNames = map[string]string{
	"pprof":                "STROPPY_PPROF",
	"pprof-block-rate":     "STROPPY_PPROF_BLOCK_RATE",
	"pprof-mutex-fraction": "STROPPY_PPROF_MUTEX_FRACTION",
}

// Configure also wraps manually parsed commands so standalone and managed applications share diagnostics.
func Configure(root *cobra.Command) {
	root.PersistentFlags().String("pprof", "",
		"serve Go profiles on loopback (env STROPPY_PPROF; disabled by default)")
	root.PersistentFlags().String("pprof-block-rate", "",
		"block sampling rate in nanoseconds (env STROPPY_PPROF_BLOCK_RATE)")
	root.PersistentFlags().String("pprof-mutex-fraction", "",
		"sample one in N mutex events (env STROPPY_PPROF_MUTEX_FRACTION)")
	wrap(root)
}

func wrap(command *cobra.Command) {
	if command.RunE != nil {
		command.RunE = profiled(command.RunE)
	}

	for _, child := range command.Commands() {
		wrap(child)
	}
}

func profiled(run func(*cobra.Command, []string) error) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) (err error) {
		selected, args, err := resolve(cmd, args)
		if err != nil {
			return err
		}

		previous := cmd.Context()
		ctx := profiling.WithConfig(previous, selected)

		cmd.SetContext(ctx)
		defer cmd.SetContext(previous)

		if isHelp(cmd, args) {
			return run(cmd, args)
		}

		session, err := profiling.Start(ctx, &selected)
		if err != nil {
			return err
		}

		defer func() { err = errors.Join(err, session.Close()) }()

		cmd.SetContext(context.WithValue(ctx, sessionKey{}, session))

		if session.Addr() != "" {
			fmt.Fprintf(cmd.ErrOrStderr(), "pprof listening on http://%s/debug/pprof/\n", session.Addr())
		}

		return run(cmd, args)
	}
}

func isHelp(command *cobra.Command, args []string) bool {
	if command.Name() == "help" {
		return true
	}

	for _, arg := range args {
		if arg == "--" {
			break
		}

		if arg == "--help" || arg == "-h" {
			return true
		}
	}

	return false
}

func resolve(command *cobra.Command, args []string) (profiling.Config, []string, error) {
	values := make(map[string]string, len(environmentNames))
	for flag, name := range environmentNames {
		values[flag] = os.Getenv(name)

		selected := command.Flags().Lookup(flag)
		if selected == nil {
			selected = command.InheritedFlags().Lookup(flag)
		}

		if selected != nil && selected.Changed {
			values[flag] = selected.Value.String()
		}
	}

	if command.DisableFlagParsing {
		filtered, err := filterArgs(args, values)
		if err != nil {
			return profiling.Config{}, nil, err
		}

		args = filtered
	}

	selected := profiling.Config{Address: values["pprof"]}
	for name, destination := range map[string]*int{
		"pprof-block-rate":     &selected.BlockProfileRate,
		"pprof-mutex-fraction": &selected.MutexProfileFraction,
	} {
		if values[name] != "" {
			value, err := strconv.Atoi(values[name])
			if err != nil {
				return profiling.Config{}, nil, fmt.Errorf("--%s: %w", name, err)
			}

			*destination = value
		}
	}

	return selected, args, nil
}

func filterArgs(args []string, values map[string]string) ([]string, error) {
	filtered := make([]string, 0, len(args))
	for index := 0; index < len(args); index++ {
		arg := args[index]
		if arg == "--" {
			filtered = append(filtered, args[index:]...)

			break
		}

		name, value, inline := strings.Cut(strings.TrimPrefix(arg, "--"), "=")
		if _, exists := environmentNames[name]; !exists || !strings.HasPrefix(arg, "--") {
			filtered = append(filtered, arg)

			continue
		}

		if !inline {
			index++
			if index == len(args) || strings.HasPrefix(args[index], "--") {
				return nil, fmt.Errorf("%w: --%s", errFlagValue, name)
			}

			value = args[index]
		}

		values[name] = value
	}

	return filtered, nil
}

// Forward releases the parent's listener before a delegated process binds the same address.
func Forward(ctx context.Context) ([]string, error) {
	selected, err := profiling.Resolve(ctx, nil)
	if err != nil {
		return nil, err
	}

	if session, ok := ctx.Value(sessionKey{}).(*profiling.Session); ok {
		if err := session.Close(); err != nil {
			return nil, err
		}
	}

	return append(os.Environ(),
		"STROPPY_PPROF="+selected.Address,
		"STROPPY_PPROF_BLOCK_RATE="+strconv.Itoa(selected.BlockProfileRate),
		"STROPPY_PPROF_MUTEX_FRACTION="+strconv.Itoa(selected.MutexProfileFraction),
	), nil
}
