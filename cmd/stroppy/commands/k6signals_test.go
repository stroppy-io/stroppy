package commands

import (
	"os"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.k6.io/k6/cmd/state"
)

func sendInt(t *testing.T) {
	t.Helper()
	require.NoError(t, syscall.Kill(os.Getpid(), syscall.SIGINT))
}

func TestDoubleConfirmation_TimerExpiry(t *testing.T) {
	sigChan := make(chan os.Signal, 2)
	stopper := make(chan struct{}, 1)

	go doubleConfirmationSigInt(sigChan, stopper)
	time.Sleep(50 * time.Millisecond)

	// send signal to start the timer
	sendInt(t)

	// wait for the Timer to launch
	time.Sleep(6 * time.Second)

	sendInt(t)

	time.Sleep(100 * time.Millisecond)
	stopper <- struct{}{}
}

func TestDoubleConfirmation_SuccessExit(t *testing.T) {
	sigChan := make(chan os.Signal, 2)
	stopper := make(chan struct{}, 1)

	go doubleConfirmationSigInt(sigChan, stopper)
	time.Sleep(50 * time.Millisecond)

	// send signal to start the timer
	sendInt(t)

	// wait for the Timer to launch
	time.Sleep(2 * time.Second)

	sendInt(t)

	time.Sleep(100 * time.Millisecond)
	stopper <- struct{}{}
}

func TestInterceptMiddleware(t *testing.T) {
	gs := &state.GlobalState{}
	inteceptInteruptSignals(gs)

	ch := make(chan os.Signal, 1)
	gs.SignalNotify(ch, os.Interrupt)
	// Give the goroutine a moment to start.
	time.Sleep(50 * time.Millisecond)

	// SignalStop should not panic and should clean up.
	gs.SignalStop(ch)
}
