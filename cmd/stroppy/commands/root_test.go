package commands

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/spf13/cobra"

	"github.com/stroppy-io/stroppy/pkg/bench"
	"github.com/stroppy-io/stroppy/pkg/driver"
	_ "github.com/stroppy-io/stroppy/pkg/driver/noop"
)

func TestExitCodeFor(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		cancelCode int
		err        error
		want       int
	}{
		{"success", 130, nil, 0},
		{"canceled sigint", 130, context.Canceled, 130},
		{"canceled sigterm", 143, context.Canceled, 143},
		{"wrapped canceled", 130, fmt.Errorf("run: %w", context.Canceled), 130},
		{"other error", 130, errors.New("boom"), 1},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := exitCodeFor(tc.cancelCode, tc.err); got != tc.want {
				t.Fatalf("exitCodeFor(%d, %v) = %d, want %d", tc.cancelCode, tc.err, got, tc.want)
			}
		})
	}
}

func TestCommandErrorExitAndDiagnosticContract(t *testing.T) {
	registerCommandErrorWorkloads()

	ordinaryCode, ordinaryOutput := executeTestCommand(t, []string{
		"run", "test/command-ordinary-error", "-d", "noop", "--iterations", "3", "--vus", "1",
	})
	if ordinaryCode != 0 {
		t.Fatalf("ordinary-error exit code = %d, output = %q, want 0", ordinaryCode, ordinaryOutput)
	}

	if commandOrdinaryErrorWorkload.calls.Load() != 3 {
		t.Fatalf("ordinary-error iterations = %d, want 3", commandOrdinaryErrorWorkload.calls.Load())
	}

	fatalCode, fatalOutput := executeTestCommand(t, []string{
		"run", "test/command-fatal-error", "-d", "noop", "--iterations", "10", "--vus", "2",
	})
	if fatalCode != 1 {
		t.Fatalf("fatal exit code = %d, output = %q, want 1", fatalCode, fatalOutput)
	}

	if got := strings.Count(fatalOutput, "fatal command sentinel"); got != 1 {
		t.Fatalf("fatal diagnostic occurrences = %d, output = %q, want 1", got, fatalOutput)
	}
}

func executeTestCommand(t *testing.T, args []string) (int, string) {
	t.Helper()

	var output bytes.Buffer

	clearCommandContexts(rootCmd)
	rootCmd.SetArgs(args)
	rootCmd.SetOut(&output)
	rootCmd.SetErr(&output)
	t.Cleanup(func() {
		rootCmd.SetArgs(nil)
		rootCmd.SetOut(nil)
		rootCmd.SetErr(nil)
		clearCommandContexts(rootCmd)
	})

	return execute(), output.String()
}

func clearCommandContexts(cmd *cobra.Command) {
	cmd.SetContext(nil) //nolint:staticcheck // nil lets ExecuteContext propagate its context to child commands.

	for _, child := range cmd.Commands() {
		clearCommandContexts(child)
	}
}

func TestClearCommandContextsAllowsExecuteContextPropagation(t *testing.T) {
	t.Parallel()

	root := &cobra.Command{Use: "root"}
	child := &cobra.Command{
		Use: "child",
		Run: func(cmd *cobra.Command, _ []string) {
			if err := cmd.Context().Err(); !errors.Is(err, context.Canceled) {
				t.Errorf("child context error = %v, want context.Canceled", err)
			}
		},
	}
	root.AddCommand(child)
	root.SetArgs([]string{"child"})
	child.SetContext(context.TODO())
	clearCommandContexts(root)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := root.ExecuteContext(ctx); err != nil {
		t.Fatalf("ExecuteContext() error = %v", err)
	}
}

var (
	registerCommandErrorWorkloadsOnce sync.Once
	commandOrdinaryErrorWorkload      *commandErrorWorkload
)

func registerCommandErrorWorkloads() {
	registerCommandErrorWorkloadsOnce.Do(func() {
		bench.Register(func() bench.Workload {
			commandOrdinaryErrorWorkload = &commandErrorWorkload{name: "test/command-ordinary-error"}

			return commandOrdinaryErrorWorkload
		})
		bench.Register(func() bench.Workload {
			return &commandErrorWorkload{name: "test/command-fatal-error", fatal: true}
		})
	})
}

type commandErrorWorkload struct {
	name  string
	fatal bool
	calls atomic.Int64
}

func (w *commandErrorWorkload) Name() string                            { return w.name }
func (*commandErrorWorkload) Define(*bench.Def) error                   { return nil }
func (*commandErrorWorkload) Setup(context.Context, *bench.Bench) error { return nil }
func (w *commandErrorWorkload) Iterate(ctx context.Context, b *bench.Bench) error {
	w.calls.Add(1)

	if !w.fatal {
		return errors.New("ordinary command failure")
	}

	policy := b.TxRetryPolicy(bench.TxRetryPolicyOptions{
		MaxAttempts: 1,
		Actions: bench.ErrorActionMap{ //nolint:exhaustive // test overrides one kind
			driver.ErrorKindUnknown: bench.ErrorActionFatal,
		},
	})

	return bench.Retry0(ctx, policy, func() error { return errors.New("fatal command sentinel") })
}

func (*commandErrorWorkload) Teardown(context.Context, *bench.Bench) error { return nil }
