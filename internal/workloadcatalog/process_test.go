package workloadcatalog

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRunCancellationIsBoundedForTermIgnoringProcess(t *testing.T) {
	if os.Getenv("STROPPY_IGNORE_TERM_HELPER") == "1" {
		for {
			time.Sleep(time.Second)
		}
	}

	store, err := OpenAt(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	artifact := filepath.Join(t.TempDir(), "helper")
	if err := copyCurrentExecutable(artifact); err != nil {
		t.Fatal(err)
	}

	if _, err := store.Publish(&Entry{Name: "custom/hang"}, artifact, false); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()

	started := time.Now()

	err = store.Run(
		ctx,
		"custom/hang",
		[]string{"-test.run", "TestRunCancellationIsBoundedForTermIgnoringProcess"},
		&Process{Env: append(os.Environ(), "STROPPY_IGNORE_TERM_HELPER=1")},
	)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Run() error = %v", err)
	}

	if elapsed := time.Since(started); elapsed > terminateGrace+killWait+time.Second {
		t.Fatalf("Run() cancellation took %s", elapsed)
	}
}

func copyCurrentExecutable(destination string) error {
	current, err := os.Executable()
	if err != nil {
		return err
	}

	data, err := os.ReadFile(current)
	if err != nil {
		return err
	}

	return os.WriteFile(destination, data, 0o700)
}
