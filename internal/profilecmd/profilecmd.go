package profilecmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/stroppy-io/stroppy/v6/pkg/profiling"
)

type sessionKey struct{}

type commandSession struct {
	session *profiling.Session
}

const deferredAnnotation = "stroppy.profiling.deferred"

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

// Defer lets routing commands decide whether this process or a child owns the listener.
func Defer(command *cobra.Command) {
	if command.Annotations == nil {
		command.Annotations = make(map[string]string)
	}

	command.Annotations[deferredAnnotation] = "true"
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

		if _, nested := ctx.Value(sessionKey{}).(*commandSession); nested {
			return run(cmd, args)
		}

		owner := &commandSession{}

		cmd.SetContext(context.WithValue(ctx, sessionKey{}, owner))
		defer func() { err = errors.Join(err, owner.session.Close()) }()

		if cmd.Annotations[deferredAnnotation] != "true" {
			if err := Activate(cmd); err != nil {
				return err
			}
		}

		return run(cmd, args)
	}
}

// Activate binds and announces diagnostics only once local execution has been selected.
func Activate(command *cobra.Command) error {
	if command.Context() == nil {
		return nil
	}

	owner, ok := command.Context().Value(sessionKey{}).(*commandSession)
	if !ok || owner.session != nil {
		return nil
	}

	session, err := profiling.Start(command.Context(), nil)
	if err != nil {
		return err
	}

	owner.session = session
	if session.Addr() != "" {
		fmt.Fprintf(command.ErrOrStderr(), "pprof listening on http://%s/debug/pprof/\n", session.Addr())
	}

	return nil
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
	}

	if _, nested := command.Context().Value(sessionKey{}).(*commandSession); nested {
		inherited, err := profiling.Resolve(command.Context(), nil)
		if err != nil {
			return profiling.Config{}, nil, err
		}

		values["pprof"] = inherited.Address
		values["pprof-block-rate"] = strconv.Itoa(inherited.BlockProfileRate)
		values["pprof-mutex-fraction"] = strconv.Itoa(inherited.MutexProfileFraction)
	}

	for flag := range environmentNames {
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

// Forward passes the selected options to the process that executes a delegated command.
func Forward(ctx context.Context) ([]string, error) {
	selected, err := profiling.Resolve(ctx, nil)
	if err != nil {
		return nil, err
	}

	if owner, ok := ctx.Value(sessionKey{}).(*commandSession); ok {
		if err := owner.session.Close(); err != nil {
			return nil, err
		}
	}

	environment := slices.DeleteFunc(os.Environ(), func(value string) bool {
		name, _, _ := strings.Cut(value, "=")
		for _, variable := range environmentNames {
			if name == variable {
				return true
			}
		}

		return false
	})

	return append(environment,
		"STROPPY_PPROF="+selected.Address,
		"STROPPY_PPROF_BLOCK_RATE="+strconv.Itoa(selected.BlockProfileRate),
		"STROPPY_PPROF_MUTEX_FRACTION="+strconv.Itoa(selected.MutexProfileFraction),
	), nil
}
