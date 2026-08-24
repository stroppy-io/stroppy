package shutdown

import (
	"context"
	"os"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

func TestMonitorSignalsCancelThenForce(t *testing.T) {
	sigCh := make(chan os.Signal, 2)
	ctx, cancel := context.WithCancel(context.Background())

	var (
		stopped  atomic.Bool
		firstSig atomic.Int32
	)

	forced := make(chan int, 1)
	done := monitorSignals(sigCh, cancel, func(code int) { forced <- code }, &stopped, &firstSig)

	defer close(done)

	sigCh <- syscall.SIGINT

	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("first signal did not cancel the context")
	}

	if err := ctx.Err(); err != context.Canceled {
		t.Fatalf("ctx.Err() = %v, want context.Canceled", err)
	}

	if got := firstSig.Load(); got != int32(syscall.SIGINT) {
		t.Fatalf("firstSig = %d, want %d", got, int32(syscall.SIGINT))
	}

	sigCh <- syscall.SIGTERM

	select {
	case code := <-forced:
		if code != ForcedExitCode {
			t.Fatalf("force code = %d, want %d", code, ForcedExitCode)
		}
	case <-time.After(time.Second):
		t.Fatal("second signal did not trigger forced exit")
	}

	if !stopped.Load() {
		t.Fatal("forced exit did not claim the stop gate")
	}
}

func TestMonitorSignalsStoppedIgnoresSignal(t *testing.T) {
	sigCh := make(chan os.Signal, 2)
	ctx, cancel := context.WithCancel(context.Background())

	var (
		stopped  atomic.Bool
		firstSig atomic.Int32
	)

	stopped.Store(true)

	forced := make(chan int, 1)
	done := monitorSignals(sigCh, cancel, func(code int) { forced <- code }, &stopped, &firstSig)

	defer close(done)

	sigCh <- syscall.SIGINT

	select {
	case code := <-forced:
		t.Fatalf("force fired after stop (code %d)", code)
	case <-ctx.Done():
		t.Fatal("context canceled after stop")
	case <-time.After(100 * time.Millisecond):
		// expected: the signal is ignored once stopped is set
	}
}

func TestDrainSignals(t *testing.T) {
	ch := make(chan os.Signal, 2)
	ch <- syscall.SIGINT

	ch <- syscall.SIGTERM

	drainSignals(ch)

	select {
	case sig := <-ch:
		t.Fatalf("drainSignals left signal %v in the channel", sig)
	default:
	}
}

func TestExitCodeFor(t *testing.T) {
	cases := []struct {
		sig  os.Signal
		want int
	}{
		{syscall.SIGINT, 130},
		{syscall.SIGTERM, 143},
	}

	for _, tc := range cases {
		if got := ExitCodeFor(tc.sig); got != tc.want {
			t.Errorf("ExitCodeFor(%v) = %d, want %d", tc.sig, got, tc.want)
		}
	}
}

func TestNotifyContextExitStatusWithoutSignal(t *testing.T) {
	_, stop, exitStatus := NotifyContext(context.Background(), func(int) {})
	defer stop()

	if got := exitStatus(); got != 1 {
		t.Fatalf("exitStatus() = %d, want 1", got)
	}
}

func TestNotifyContextStopCancelsAndReleases(t *testing.T) {
	ctx, stop, _ := NotifyContext(context.Background(), func(int) {})

	stop()

	if err := ctx.Err(); err != context.Canceled {
		t.Fatalf("ctx.Err() = %v, want context.Canceled", err)
	}
}

func TestNotifyContextSIGTERMExitStatus(t *testing.T) {
	if testing.Short() {
		t.Skip("delivers real signals to the test process")
	}

	ctx, stop, exitStatus := NotifyContext(context.Background(), func(int) {})
	defer stop()

	if err := syscall.Kill(os.Getpid(), syscall.SIGTERM); err != nil {
		t.Fatalf("kill self: %v", err)
	}

	select {
	case <-ctx.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("SIGTERM did not cancel the context")
	}

	if got := exitStatus(); got != ExitCodeFor(syscall.SIGTERM) {
		t.Fatalf("exitStatus() = %d, want %d", got, ExitCodeFor(syscall.SIGTERM))
	}
}

func TestNotifyContextDeliversSignals(t *testing.T) {
	if testing.Short() {
		t.Skip("delivers real signals to the test process")
	}

	forced := make(chan int, 1)
	ctx, stop, exitStatus := NotifyContext(context.Background(), func(code int) { forced <- code })

	defer stop()

	if err := syscall.Kill(os.Getpid(), syscall.SIGINT); err != nil {
		t.Fatalf("kill self: %v", err)
	}

	select {
	case <-ctx.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("SIGINT did not cancel the context")
	}

	if got := exitStatus(); got != ExitCodeFor(syscall.SIGINT) {
		t.Fatalf("exitStatus() = %d, want %d", got, ExitCodeFor(syscall.SIGINT))
	}

	if err := syscall.Kill(os.Getpid(), syscall.SIGTERM); err != nil {
		t.Fatalf("kill self: %v", err)
	}

	select {
	case code := <-forced:
		if code != ForcedExitCode {
			t.Fatalf("force code = %d, want %d", code, ForcedExitCode)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("second signal did not trigger forced exit")
	}
}
