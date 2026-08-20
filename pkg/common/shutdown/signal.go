package shutdown

import (
	"context"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
)

// Exit statuses surfaced by the run command (see cmd/stroppy/commands/root.go).
const (
	// ForcedExitCode is the exit status after a second SIGINT/SIGTERM aborts a
	// graceful teardown that has not finished. Distinct from the generic error
	// status (1) so operators can tell a forced abort from a plain error.
	ForcedExitCode = 2
)

const (
	// exitCodeSignalBase is the shell-convention offset for a graceful-cancel
	// exit status: 128 + signal number.
	exitCodeSignalBase = 128

	// signalBufferSize sizes the signal channel. It holds the first
	// SIGINT/SIGTERM plus one more, so a second signal queues for the forced
	// exit path without dropping as soon as the handler is installed.
	signalBufferSize = 2
)

// ExitCodeFor returns the shell-convention exit status (128 + signal number)
// for a graceful cancellation triggered by sig: 130 for SIGINT, 143 for
// SIGTERM. Returns 1 when sig is not a recognizable signal.
func ExitCodeFor(sig os.Signal) int {
	s, ok := sig.(syscall.Signal)
	if !ok {
		return 1
	}

	return exitCodeSignalBase + int(s)
}

// NotifyContext returns a context canceled by the first SIGINT/SIGTERM, a stop
// func, and an exitStatus func reporting the graceful-shutdown exit code derived
// from that first signal. A second signal after cancellation invokes
// force(ForcedExitCode) as a bounded escape hatch for a hung teardown; force
// defaults to os.Exit.
//
// Call stop exactly once when the command is done. It releases the OS handler,
// drops any already-delivered-but-unconsumed signal, and cancels ctx so no
// handler goroutine outlives the command.
func NotifyContext(parent context.Context, force func(int)) (
	ctx context.Context, stop func(), exitStatus func() int,
) {
	var cancel context.CancelFunc

	ctx, cancel = context.WithCancel(parent)

	sigCh := make(chan os.Signal, signalBufferSize)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	if force == nil {
		force = os.Exit
	}

	var (
		stopped  atomic.Bool
		firstSig atomic.Int32 // syscall.Signal number of the first signal; 0 = none yet
	)

	done := monitorSignals(sigCh, cancel, force, &stopped, &firstSig)

	stop = func() {
		stopped.Store(true) // gate: no force can fire once stop begins
		signal.Stop(sigCh)  // no new deliveries
		drainSignals(sigCh) // drop already-buffered signals so monitor can't pick them
		close(done)
		cancel()
	}

	exitStatus = func() int {
		if sig := syscall.Signal(firstSig.Load()); sig != 0 {
			return ExitCodeFor(sig)
		}

		return ExitCodeFor(syscall.SIGINT)
	}

	return ctx, stop, exitStatus
}

// monitorSignals runs the cancel-then-force loop and returns a done channel used
// to stop it. The first signal cancels ctx and records its number; a subsequent
// signal forces exit. Once stopped is set, no signal is handled further.
func monitorSignals(
	sigCh <-chan os.Signal,
	cancel context.CancelFunc,
	force func(int),
	stopped *atomic.Bool,
	firstSig *atomic.Int32,
) chan struct{} {
	done := make(chan struct{})

	go func() {
		for {
			select {
			case sig := <-sigCh:
				if stopped.Load() {
					return
				}

				if firstSig.CompareAndSwap(0, sigNumber(sig)) {
					cancel() // first signal → graceful teardown
				} else {
					force(ForcedExitCode) // second signal → forced exit
				}
			case <-done:
				return
			}
		}
	}()

	return done
}

func sigNumber(sig os.Signal) int32 {
	s, ok := sig.(syscall.Signal)
	if !ok {
		return 0
	}

	//nolint:gosec // G115: sig is a bounded Unix signal number (SIGINT=2, SIGTERM=15), well within int32
	return int32(s)
}

func drainSignals(sigCh <-chan os.Signal) {
	for {
		select {
		case <-sigCh:
		default:
			return
		}
	}
}
