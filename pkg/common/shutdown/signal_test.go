package shutdown

import (
	"context"
	"os"
	"syscall"
	"testing"
	"time"
)

func TestMonitorSignalsCancelThenForce(t *testing.T) {
	sigCh := make(chan os.Signal, 2)
	ctx, cancel := context.WithCancel(context.Background())

	forced := make(chan int, 1)
	done := monitorSignals(sigCh, cancel, func(code int) { forced <- code })
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

	sigCh <- syscall.SIGTERM

	select {
	case code := <-forced:
		if code != ForcedExitCode {
			t.Fatalf("force code = %d, want %d", code, ForcedExitCode)
		}
	case <-time.After(time.Second):
		t.Fatal("second signal did not trigger forced exit")
	}
}

func TestNotifyContextStopCancelsAndReleases(t *testing.T) {
	ctx, stop := NotifyContext(context.Background(), func(int) {})

	stop()

	if err := ctx.Err(); err != context.Canceled {
		t.Fatalf("ctx.Err() = %v, want context.Canceled", err)
	}
}

func TestNotifyContextDeliversSignals(t *testing.T) {
	if testing.Short() {
		t.Skip("delivers real signals to the test process")
	}

	forced := make(chan int, 1)
	ctx, stop := NotifyContext(context.Background(), func(code int) { forced <- code })
	defer stop()

	if err := syscall.Kill(os.Getpid(), syscall.SIGINT); err != nil {
		t.Fatalf("kill self: %v", err)
	}

	select {
	case <-ctx.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("SIGINT did not cancel the context")
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
