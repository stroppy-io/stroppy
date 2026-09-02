package pgnoop

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const (
	defaultHost = "127.0.0.1"

	readyTimeout  = 15 * time.Second
	readyPollTick = 50 * time.Millisecond
	stopGrace     = 3 * time.Second
)

// ErrServerNotReady reports that the server neither printed its listening
// line nor accepted connections within the readiness window.
var ErrServerNotReady = errors.New("pgnoop: server did not become ready")

// Server is a pg-noop process bound to a loopback port for one baseline run.
type Server struct {
	cmd  *exec.Cmd
	host string
	port int

	readySignal chan struct{}
}

// Addr returns the host:port the server listens on.
func (s *Server) Addr() string { return net.JoinHostPort(s.host, strconv.Itoa(s.port)) }

// Port returns the server's listen port.
func (s *Server) Port() int { return s.port }

// Start launches the server binary on host:port and waits until it accepts
// connections or prints its listening line.
func Start(ctx context.Context, binary string, port int) (*Server, error) {
	s := &Server{
		host:        defaultHost,
		port:        port,
		readySignal: make(chan struct{}),
	}

	args := []string{"--host", s.host, "--port", strconv.Itoa(port)}
	s.cmd = exec.CommandContext(ctx, binary, args...) //nolint:gosec // G204: verified path

	stdout, err := s.cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("pgnoop: stdout pipe: %w", err)
	}

	stderr, err := s.cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("pgnoop: stderr pipe: %w", err)
	}

	if err := s.cmd.Start(); err != nil {
		return nil, fmt.Errorf("pgnoop: start server: %w", err)
	}

	// The server announces "pgnoop listening on <addr>"; treat the line as
	// the ready signal and drain both pipes so the process never blocks.
	go announceListening(stdout, s.readySignal)
	go func() { _, _ = io.Copy(io.Discard, stderr) }()

	if err := s.waitReady(ctx); err != nil {
		_ = s.Stop()

		return nil, err
	}

	return s, nil
}

// waitReady polls until the server accepts connections, prints its listening
// line, the context is canceled, or the readiness window expires.
func (s *Server) waitReady(ctx context.Context) error {
	deadline := time.Now().Add(readyTimeout)
	dialer := &net.Dialer{Timeout: readyPollTick}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err() //nolint:wrapcheck // caller reports the cancellation directly
		case <-s.readySignal:
			return nil
		default:
		}

		conn, err := dialer.DialContext(ctx, "tcp", s.Addr())
		if err == nil {
			_ = conn.Close()

			return nil
		}

		if time.Now().After(deadline) {
			return fmt.Errorf("%w at %s", ErrServerNotReady, s.Addr())
		}

		select {
		case <-time.After(readyPollTick):
		case <-ctx.Done():
			return ctx.Err() //nolint:wrapcheck // caller reports the cancellation directly
		}
	}
}

// Stop terminates the server: SIGTERM first, SIGKILL after a grace period.
// The signal-derived exit status is the normal path and not an error.
func (s *Server) Stop() error {
	if s.cmd == nil || s.cmd.Process == nil {
		return nil
	}

	if err := s.cmd.Process.Signal(syscall.SIGTERM); err != nil {
		// Already gone.
		_ = s.cmd.Wait()

		return nil //nolint:nilerr // process exit is the desired end state
	}

	done := make(chan struct{})

	go func() {
		defer close(done)

		_ = s.cmd.Wait()
	}()

	select {
	case <-done:
	case <-time.After(stopGrace):
		_ = s.cmd.Process.Kill()

		<-done
	}

	return nil
}

func announceListening(output io.Reader, ready chan struct{}) {
	scanner := bufio.NewScanner(output)

	for scanner.Scan() {
		if strings.Contains(scanner.Text(), "listening") {
			select {
			case <-ready:
			default:
				close(ready)
			}
		}
	}
}
