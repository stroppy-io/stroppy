package profilecmd

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/stroppy-io/stroppy/v6/pkg/profiling"
)

func TestCLIFormsAndPrecedence(t *testing.T) {
	for _, test := range []struct {
		name   string
		manual bool
		args   []string
		want   []string
	}{
		{"cobra after command", false, []string{"run", "--pprof", "127.0.0.1:0"}, nil},
		{"cobra before command", false, []string{"--pprof=127.0.0.1:0", "run"}, nil},
		{"manual", true, []string{"run", "simple", "--pprof", "127.0.0.1:0"}, []string{"simple"}},
		{"manual equals", true, []string{"run", "simple", "--pprof=127.0.0.1:0"}, []string{"simple"}},
		{"manual inherited", true, []string{"--pprof=127.0.0.1:0", "run", "simple"}, []string{"simple"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("STROPPY_PPROF", "invalid")

			errCommand := io.ErrUnexpectedEOF
			command := &cobra.Command{Use: "run", DisableFlagParsing: test.manual}
			command.RunE = func(cmd *cobra.Command, args []string) error {
				if !slices.Equal(args, test.want) {
					t.Fatalf("args = %v, want %v", args, test.want)
				}

				session := cmd.Context().Value(sessionKey{}).(*profiling.Session)

				response, err := http.Get("http://" + session.Addr() + "/debug/pprof/heap")
				if err != nil {
					t.Fatal(err)
				}

				response.Body.Close()

				if response.StatusCode != http.StatusOK {
					t.Fatalf("status = %d", response.StatusCode)
				}

				return errCommand
			}
			root := &cobra.Command{Use: "stroppy", SilenceErrors: true, SilenceUsage: true}
			root.AddCommand(command)
			Configure(root)

			var output bytes.Buffer
			root.SetErr(&output)
			root.SetArgs(test.args)

			if err := root.ExecuteContext(t.Context()); !errors.Is(err, errCommand) {
				t.Fatalf("command error = %v", err)
			}

			if !strings.Contains(output.String(), "pprof listening on http://127.0.0.1:") {
				t.Fatalf("diagnostics = %q", output.String())
			}
		})
	}
}

func TestManualFlagValidationAndHelp(t *testing.T) {
	for _, test := range []struct {
		args      []string
		wantError bool
	}{
		{[]string{"--pprof"}, true},
		{[]string{"--pprof", "--help"}, true},
		{[]string{"--pprof-block-rate=bad"}, true},
		{[]string{"--pprof-mutex-fraction=-1"}, true},
		{[]string{"--pprof=invalid"}, true},
		{[]string{"--pprof=invalid", "--help"}, false},
		{[]string{"--", "--pprof=invalid"}, false},
	} {
		t.Run(strings.Join(test.args, " "), func(t *testing.T) {
			root := &cobra.Command{
				Use: "stroppy", DisableFlagParsing: true,
				RunE: func(*cobra.Command, []string) error { return nil },
			}
			Configure(root)
			root.SetOut(io.Discard)
			root.SetErr(io.Discard)
			root.SetArgs(test.args)

			err := root.ExecuteContext(t.Context())
			if (err != nil) != test.wantError {
				t.Fatalf("error = %v, want error = %t", err, test.wantError)
			}
		})
	}
}

func TestExplicitDisableOverridesEnvironment(t *testing.T) {
	t.Setenv("STROPPY_PPROF", "127.0.0.1:0")

	root := &cobra.Command{Use: "stroppy", RunE: func(cmd *cobra.Command, _ []string) error {
		session := cmd.Context().Value(sessionKey{}).(*profiling.Session)
		if session.Addr() != "" {
			t.Fatal("explicit disable ignored")
		}

		return nil
	}}
	Configure(root)
	root.SetArgs([]string{"--pprof="})

	if err := root.ExecuteContext(t.Context()); err != nil {
		t.Fatal(err)
	}
}

func TestForwardReleasesListenerAndPreservesOverrides(t *testing.T) {
	selected := profiling.Config{Address: "127.0.0.1:0", BlockProfileRate: 1000, MutexProfileFraction: 5}
	ctx := profiling.WithConfig(t.Context(), selected)

	session, err := profiling.Start(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	ctx = context.WithValue(ctx, sessionKey{}, session)

	environment, err := Forward(ctx)
	if err != nil {
		t.Fatal(err)
	}

	for _, value := range []string{
		"STROPPY_PPROF=127.0.0.1:0", "STROPPY_PPROF_BLOCK_RATE=1000", "STROPPY_PPROF_MUTEX_FRACTION=5",
	} {
		if !slices.Contains(environment, value) {
			t.Fatalf("missing environment %s", value)
		}
	}

	listener, err := net.Listen("tcp", session.Addr())
	if err != nil {
		t.Fatalf("parent listener still bound: %v", err)
	}

	listener.Close()
}
