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
	startRetries  = 10
)

// ErrServerNotReady reports that the server neither printed its listening
// line nor accepted connections within the readiness window.
var ErrServerNotReady = errors.New("pgnoop: server did not become ready")

// Server is a pg-noop process bound to a loopback port for one baseline run.
type Server struct {
	cmd  *exec.Cmd
	host string
	port int

	readySignal chan int
	done        chan error
}

// Addr returns the host:port the server listens on.
func (s *Server) Addr() string { return net.JoinHostPort(s.host, strconv.Itoa(s.port)) }

// Port returns the server's listen port.
func (s *Server) Port() int { return s.port }

// Start launches the server binary on loopback and waits for its listening
// announcement. Port zero selects an ephemeral port with bounded bind retries.
func Start(ctx context.Context, binary string, port int) (*Server, error) {
	if port != 0 {
		return startOnPort(ctx, binary, port)
	}

	var lastErr error

	for range startRetries {
		candidate, err := freePort(ctx)
		if err != nil {
			return nil, err
		}

		server, err := startOnPort(ctx, binary, candidate)
		if err == nil {
			return server, nil
		}

		lastErr = err

		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
	}

	return nil, fmt.Errorf("pgnoop: start after %d bind attempts: %w", startRetries, lastErr)
}

func startOnPort(ctx context.Context, binary string, port int) (*Server, error) {
	s := &Server{
		host:        defaultHost,
		port:        port,
		readySignal: make(chan int, 1),
		done:        make(chan error, 1),
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

	// The server announces "pgnoop listening on <addr>" after binding. Some
	// builds buffer stdout, so waitReady also checks that the accepting socket
	// remains paired with a live child process.
	go announceListening(stdout, s.readySignal)
	go func() { _, _ = io.Copy(io.Discard, stderr) }()
	go func() {
		s.done <- s.cmd.Wait()

		close(s.done)
	}()

	if err := s.waitReady(ctx); err != nil {
		_ = s.Stop()

		return nil, err
	}

	return s, nil
}

func freePort(ctx context.Context) (int, error) {
	listener, err := (&net.ListenConfig{}).Listen(ctx, "tcp", net.JoinHostPort(defaultHost, "0"))
	if err != nil {
		return 0, fmt.Errorf("pgnoop: find free port: %w", err)
	}
	defer func() { _ = listener.Close() }()

	addr, ok := listener.Addr().(*net.TCPAddr)
	if !ok {
		return 0, ErrServerNotReady
	}

	return addr.Port, nil
}

// waitReady waits for the listening announcement or a stable accepting socket
// while the child process remains alive.
func (s *Server) waitReady(ctx context.Context) error {
	timer := time.NewTimer(readyTimeout)
	defer timer.Stop()

	ticker := time.NewTicker(readyPollTick)
	defer ticker.Stop()

	dialer := &net.Dialer{Timeout: readyPollTick}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err() //nolint:wrapcheck // caller reports the cancellation directly
		case port := <-s.readySignal:
			if port == s.port {
				return nil
			}
		case err := <-s.done:
			return serverExitError(s, err)
		case <-ticker.C:
			conn, err := dialer.DialContext(ctx, "tcp", s.Addr())
			if err != nil {
				continue
			}

			_ = conn.Close()

			// A process that lost the bind race exits immediately. Require the
			// child to remain alive before accepting socket readiness.
			select {
			case err := <-s.done:
				return serverExitError(s, err)
			case <-ctx.Done():
				return ctx.Err() //nolint:wrapcheck // caller reports the cancellation directly
			case <-time.After(readyPollTick):
				return nil
			}
		case <-timer.C:
			return fmt.Errorf("%w at %s", ErrServerNotReady, s.Addr())
		}
	}
}

func serverExitError(server *Server, err error) error {
	if err != nil {
		return fmt.Errorf("%w at %s: %w", ErrServerNotReady, server.Addr(), err)
	}

	return fmt.Errorf("%w at %s: process exited", ErrServerNotReady, server.Addr())
}

// Stop terminates the server: SIGTERM first, SIGKILL after a grace period.
// The signal-derived exit status is the normal path and not an error.
func (s *Server) Stop() error {
	if s.cmd == nil || s.cmd.Process == nil || s.done == nil {
		return nil
	}

	select {
	case <-s.done:
		return nil
	default:
	}

	if err := s.cmd.Process.Signal(syscall.SIGTERM); err != nil {
		select {
		case <-s.done:
		case <-time.After(stopGrace):
		}

		return nil //nolint:nilerr // process exit is the desired end state
	}

	select {
	case <-s.done:
	case <-time.After(stopGrace):
		_ = s.cmd.Process.Kill()

		<-s.done
	}

	return nil
}

func announceListening(output io.Reader, ready chan<- int) {
	scanner := bufio.NewScanner(output)

	for scanner.Scan() {
		port, ok := listeningPort(scanner.Text())
		if !ok {
			continue
		}

		select {
		case ready <- port:
		default:
		}
	}
}

func listeningPort(line string) (int, bool) {
	if !strings.Contains(line, "listening") {
		return 0, false
	}

	fields := strings.Fields(line)
	for idx := len(fields) - 1; idx >= 0; idx-- {
		_, rawPort, err := net.SplitHostPort(strings.Trim(fields[idx], ",.;"))
		if err != nil {
			continue
		}

		port, err := strconv.Atoi(rawPort)
		if err == nil && port > 0 {
			return port, true
		}
	}

	return 0, false
}
