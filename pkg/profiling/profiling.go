// Package profiling provides opt-in, process-wide Go diagnostics for Stroppy applications.
package profiling

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"runtime"
	"strconv"
	"sync"
	"time"
)

const shutdownTimeout = 5 * time.Second

var (
	errAddress  = errors.New("pprof requires a loopback host:port address")
	errSampling = errors.New("pprof sampling rates must not be negative")
	errConflict = errors.New("pprof already running with different options")
	process     struct {
		sync.Mutex
		active *state
	}
)

// Config controls process-wide diagnostics. An empty Address disables profiling.
// Sampling changes benchmark overhead; zero leaves the existing runtime setting untouched.
type Config struct {
	Address              string
	BlockProfileRate     int
	MutexProfileFraction int
}

type configKey struct{}

// WithConfig supplies profiling options to nested Stroppy runs without changing the environment.
func WithConfig(ctx context.Context, config Config) context.Context {
	return context.WithValue(ctx, configKey{}, config)
}

// Resolve gives explicit options precedence over context options and process environment.
func Resolve(ctx context.Context, config *Config) (Config, error) {
	if config != nil {
		return *config, nil
	}

	if ctx != nil {
		if selected, ok := ctx.Value(configKey{}).(Config); ok {
			return selected, nil
		}
	}

	selected := Config{Address: os.Getenv("STROPPY_PPROF")}
	for name, destination := range map[string]*int{
		"STROPPY_PPROF_BLOCK_RATE":     &selected.BlockProfileRate,
		"STROPPY_PPROF_MUTEX_FRACTION": &selected.MutexProfileFraction,
	} {
		if value := os.Getenv(name); value != "" {
			parsed, err := strconv.Atoi(value)
			if err != nil {
				return Config{}, fmt.Errorf("%s: %w", name, err)
			}

			*destination = parsed
		}
	}

	return selected, nil
}

type state struct {
	config                Config
	server                *http.Server
	listener              net.Listener
	cancel                context.CancelFunc
	done                  chan struct{}
	references            int
	previousMutexFraction int
}

// Session owns one reference to a shared process-wide profiling listener.
// Concurrent runs with identical options share it until the last session closes.
type Session struct {
	state *state
	once  sync.Once
	err   error
}

// Start enables diagnostics for a caller-owned scope. Nil options use context or environment.
// Only loopback listeners are accepted because profiles can expose credentials and application data.
func Start(ctx context.Context, config *Config) (*Session, error) {
	selected, err := Resolve(ctx, config)
	if err != nil {
		return nil, err
	}

	if selected.BlockProfileRate < 0 || selected.MutexProfileFraction < 0 {
		return nil, errSampling
	}

	if selected.Address == "" {
		return &Session{}, nil
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	host, port, err := net.SplitHostPort(selected.Address)
	if err != nil {
		return nil, fmt.Errorf("%w: %q", errAddress, selected.Address)
	}

	if host == "localhost" {
		host = "127.0.0.1"
	}

	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return nil, fmt.Errorf("%w: %q", errAddress, selected.Address)
	}

	selected.Address = net.JoinHostPort(host, port)

	process.Lock()
	defer process.Unlock()

	if process.active != nil {
		if process.active.config != selected {
			return nil, errConflict
		}

		process.active.references++

		return &Session{state: process.active}, nil
	}

	listener, err := (&net.ListenConfig{}).Listen(ctx, "tcp", selected.Address)
	if err != nil {
		return nil, fmt.Errorf("start pprof: %w", err)
	}

	serverContext, cancel := context.WithCancel(context.WithoutCancel(ctx))

	active := &state{
		config: selected, listener: listener, cancel: cancel,
		done: make(chan struct{}), references: 1,
		server: &http.Server{
			Handler: profileMux(), ReadHeaderTimeout: shutdownTimeout,
			ReadTimeout: shutdownTimeout, IdleTimeout: shutdownTimeout,
			BaseContext: func(net.Listener) context.Context { return serverContext },
		},
	}
	if selected.BlockProfileRate > 0 {
		runtime.SetBlockProfileRate(selected.BlockProfileRate)
	}

	if selected.MutexProfileFraction > 0 {
		active.previousMutexFraction = runtime.SetMutexProfileFraction(selected.MutexProfileFraction)
	}

	process.active = active
	go func() {
		defer close(active.done)

		_ = active.server.Serve(listener)
	}()

	return &Session{state: active}, nil
}

// Addr reports the actual listener address, including an OS-selected port when configured with :0.
func (s *Session) Addr() string {
	if s == nil || s.state == nil {
		return ""
	}

	return s.state.listener.Addr().String()
}

// Close stops the listener after its last owner exits and cancels active profile requests.
// Explicit block sampling is disabled; explicit mutex sampling restores its previous fraction.
func (s *Session) Close() error {
	if s == nil || s.state == nil {
		return nil
	}

	s.once.Do(func() {
		process.Lock()
		defer process.Unlock()

		active := s.state

		active.references--
		if active.references != 0 {
			return
		}

		active.cancel()

		ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()

		s.err = active.server.Shutdown(ctx)
		if s.err != nil {
			s.err = errors.Join(s.err, active.server.Close())
		}

		<-active.done

		if active.config.BlockProfileRate > 0 {
			runtime.SetBlockProfileRate(0)
		}

		if active.config.MutexProfileFraction > 0 {
			runtime.SetMutexProfileFraction(active.previousMutexFraction)
		}

		process.active = nil
	})

	return s.err
}
