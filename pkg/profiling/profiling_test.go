package profiling

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

func cleanEnvironment(t *testing.T) {
	t.Helper()

	for _, name := range []string{"STROPPY_PPROF", "STROPPY_PPROF_BLOCK_RATE", "STROPPY_PPROF_MUTEX_FRACTION"} {
		t.Setenv(name, "")
	}
}

func TestResolvePrecedence(t *testing.T) {
	cleanEnvironment(t)
	t.Setenv("STROPPY_PPROF", "127.0.0.1:6060")
	t.Setenv("STROPPY_PPROF_BLOCK_RATE", "1000")
	t.Setenv("STROPPY_PPROF_MUTEX_FRACTION", "10")

	selected, err := Resolve(t.Context(), nil)
	if err != nil || selected != (Config{Address: "127.0.0.1:6060", BlockProfileRate: 1000, MutexProfileFraction: 10}) {
		t.Fatalf("environment = %+v, %v", selected, err)
	}

	ctx := WithConfig(t.Context(), Config{Address: "127.0.0.1:7070"})

	selected, err = Resolve(ctx, nil)
	if err != nil || selected.Address != "127.0.0.1:7070" || selected.BlockProfileRate != 0 {
		t.Fatalf("context = %+v, %v", selected, err)
	}

	selected, err = Resolve(ctx, &Config{})
	if err != nil || selected != (Config{}) {
		t.Fatalf("explicit disable = %+v, %v", selected, err)
	}

	t.Setenv("STROPPY_PPROF_MUTEX_FRACTION", "invalid")

	if _, err := Resolve(t.Context(), nil); err == nil {
		t.Fatal("invalid environment accepted")
	}

	if _, err := Resolve(t.Context(), &Config{}); err != nil {
		t.Fatalf("explicit config did not override invalid environment: %v", err)
	}
}

func TestDisabled(t *testing.T) {
	cleanEnvironment(t)

	session, err := Start(t.Context(), nil)
	if err != nil || session.Addr() != "" {
		t.Fatalf("disabled = %v, %v", session, err)
	}

	if err := session.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestRejectUnsafeAddressesAndRates(t *testing.T) {
	for _, address := range []string{":6060", "0.0.0.0:6060", "[::]:6060", "192.0.2.1:6060", "example.com:6060", "6060"} {
		t.Run(address, func(t *testing.T) {
			if _, err := Start(t.Context(), &Config{Address: address}); !errors.Is(err, errAddress) {
				t.Fatalf("Start(%q) = %v", address, err)
			}
		})
	}

	for _, config := range []Config{{BlockProfileRate: -1}, {MutexProfileFraction: -1}} {
		if _, err := Start(t.Context(), &config); !errors.Is(err, errSampling) {
			t.Fatalf("negative rate = %v", err)
		}
	}
}

func TestEndpointsAndSharedLifetime(t *testing.T) {
	config := Config{Address: "localhost:0"}

	first, err := Start(t.Context(), &config)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = first.Close() })

	second, err := Start(t.Context(), &config)
	if err != nil || second.Addr() != first.Addr() {
		t.Fatalf("shared listener = %v, %v", second, err)
	}

	t.Cleanup(func() { _ = second.Close() })

	if _, err := Start(t.Context(), &Config{Address: "127.0.0.1:1"}); !errors.Is(err, errConflict) {
		t.Fatalf("conflicting config = %v", err)
	}

	if err := first.Close(); err != nil {
		t.Fatal(err)
	}

	client := &http.Client{Timeout: 5 * time.Second}

	for _, path := range []string{
		"", "heap", "allocs", "goroutine", "block", "mutex", "cmdline", "symbol", "profile?seconds=1", "trace?seconds=0.01",
	} {
		t.Run(path, func(t *testing.T) {
			response, err := client.Get("http://" + second.Addr() + "/debug/pprof/" + path)
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()

			body, err := io.ReadAll(response.Body)
			if err != nil || response.StatusCode != http.StatusOK || len(body) == 0 {
				t.Fatalf("response = %d, %d bytes, %v", response.StatusCode, len(body), err)
			}
		})
	}

	response, err := client.Post("http://"+second.Addr()+"/debug/pprof/heap", "text/plain", strings.NewReader(""))
	if err != nil {
		t.Fatal(err)
	}

	response.Body.Close()

	if response.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("POST status = %d", response.StatusCode)
	}

	address := second.Addr()
	if err := second.Close(); err != nil {
		t.Fatal(err)
	}

	listener, err := net.Listen("tcp", address)
	if err != nil {
		t.Fatalf("listener not released: %v", err)
	}

	listener.Close()
}

func TestCloseCancelsActiveCapture(t *testing.T) {
	session, err := Start(t.Context(), &Config{Address: "127.0.0.1:0"})
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	response, err := http.Get("http://" + session.Addr() + "/debug/pprof/trace?seconds=60")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()

	started := time.Now()

	if err := session.Close(); err != nil {
		t.Fatal(err)
	}

	if elapsed := time.Since(started); elapsed >= shutdownTimeout {
		t.Fatalf("active capture delayed shutdown by %s", elapsed)
	}

	if _, err := io.Copy(io.Discard, response.Body); err != nil {
		t.Fatalf("capture did not flush: %v", err)
	}
}

func TestConcurrentSessions(t *testing.T) {
	config := Config{Address: "127.0.0.1:0"}

	owner, err := Start(t.Context(), &config)
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Close()

	var workers sync.WaitGroup
	for range 10 {
		workers.Go(func() {
			session, err := Start(t.Context(), &config)
			if err != nil {
				t.Error(err)

				return
			}

			if session.Addr() != owner.Addr() {
				t.Error("different listener")
			}

			if err := session.Close(); err != nil {
				t.Error(err)
			}

			if err := session.Close(); err != nil {
				t.Error(err)
			}
		})
	}

	workers.Wait()
}

func TestBindFailure(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	if _, err := Start(t.Context(), &Config{Address: listener.Addr().String()}); err == nil {
		t.Fatal("occupied address accepted")
	}
}

func TestMutexSamplingRestored(t *testing.T) {
	previous := runtime.SetMutexProfileFraction(7)
	defer runtime.SetMutexProfileFraction(previous)

	session, err := Start(t.Context(), &Config{Address: "127.0.0.1:0", MutexProfileFraction: 1, BlockProfileRate: 1})
	if err != nil {
		t.Fatal(err)
	}

	if current := runtime.SetMutexProfileFraction(-1); current != 1 {
		t.Errorf("active mutex fraction = %d", current)
	}

	if err := session.Close(); err != nil {
		t.Fatal(err)
	}

	if current := runtime.SetMutexProfileFraction(-1); current != 7 {
		t.Errorf("restored mutex fraction = %d", current)
	}
}

func TestCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	if _, err := Start(ctx, &Config{Address: "127.0.0.1:0"}); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled start = %v", err)
	}
}
