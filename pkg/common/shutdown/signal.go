package shutdown

import (
	"context"
	"os"
	"os/signal"
	"syscall"
)

// Exit statuses surfaced by the run command (see cmd/stroppy/commands/root.go).
const (
	// CanceledExitCode is the process exit status after a graceful shutdown
	// requested by the first SIGINT/SIGTERM. It follows the shell convention
	// for termination by signal 2 (SIGINT): 128 + signal number.
	CanceledExitCode = 130

	// ForcedExitCode is the exit status after a second SIGINT/SIGTERM aborts a
	// graceful teardown that has not finished. Distinct from CanceledExitCode so
	// operators can tell a forced abort from a clean cancellation.
	ForcedExitCode = 1
)

// NotifyContext returns ctx derived from parent plus a stop func. The first
// SIGINT or SIGTERM cancels ctx, which requests graceful shutdown; a second
// signal after that invokes force(ForcedExitCode) as a bounded escape hatch for
// teardown that hangs. force defaults to os.Exit; pass a stub in tests.
//
// Call stop exactly once when the command is finished. It releases the OS
// signal handler and cancels ctx, so no handler goroutine outlives the command.
func NotifyContext(parent context.Context, force func(int)) (context.Context, func()) {
	ctx, cancel := context.WithCancel(parent)

	sigCh := make(chan os.Signal, 2)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	if force == nil {
		force = os.Exit
	}

	done := monitorSignals(sigCh, cancel, force)

	stop := func() {
		signal.Stop(sigCh)
		close(done)
		cancel()
	}

	return ctx, stop
}

// monitorSignals runs the cancel-then-force loop and returns a done channel used
// to stop it. Split out so the loop is testable without delivering real signals.
func monitorSignals(sigCh <-chan os.Signal, cancel context.CancelFunc, force func(int)) chan struct{} {
	done := make(chan struct{})

	go func() {
		first := true

		for {
			select {
			case <-sigCh:
				if first {
					first = false
					cancel()
				} else {
					force(ForcedExitCode)
				}
			case <-done:
				return
			}
		}
	}()

	return done
}
