package probes //nolint:testpackage //no need

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// MockProbe implements the Probe interface for testing purposes.
type MockProbe struct {
	status ProbeStatus
	delay  time.Duration
}

func (m *MockProbe) HealthCheck(_ context.Context) ProbeStatus {
	time.Sleep(m.delay) // Simulate delay if any

	return m.status
}

func TestWaitProbe_Success(t *testing.T) {
	t.Parallel()

	probe := &MockProbe{status: SuccessProbe}
	status := WaitProbe(context.Background(), probe, DefaultProbeTimeout)
	require.Equal(t, SuccessProbe, status)
}

func TestWaitProbe_Failure(t *testing.T) {
	t.Parallel()

	probe := &MockProbe{status: FailureProbe}
	status := WaitProbe(context.Background(), probe, DefaultProbeTimeout)
	require.Equal(t, FailureProbe, status)
}

func TestWaitProbe_Timeout(t *testing.T) {
	t.Parallel()

	probe := &MockProbe{status: SuccessProbe, delay: 50 * time.Millisecond}
	status := WaitProbe(context.Background(), probe, 10*time.Millisecond)

	require.Equal(t, TimeoutProbe, status)
}

func TestNewProbeHandler_Healthy(t *testing.T) {
	t.Parallel()

	probe := &MockProbe{status: SuccessProbe}
	req, err := http.NewRequest(http.MethodGet, "/healthz/liveness", http.NoBody)

	require.NoError(t, err)

	recorder := httptest.NewRecorder()
	handler := NewProbeHandler("Liveness", probe)

	handler.ServeHTTP(recorder, req)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, "Healthy", recorder.Body.String())
}

func TestNewProbeHandler_Unhealthy(t *testing.T) {
	t.Parallel()

	probe := &MockProbe{status: FailureProbe}

	req, err := http.NewRequest(http.MethodGet, "/healthz/liveness", http.NoBody)
	require.NoError(t, err)

	recorder := httptest.NewRecorder()
	handler := NewProbeHandler("Liveness", probe)

	handler.ServeHTTP(recorder, req)

	require.Equal(t, http.StatusServiceUnavailable, recorder.Code)
	require.Equal(t, "Unhealthy Liveness", recorder.Body.String())
}

func TestServeProbes_HealthyLivenessProbe(t *testing.T) {
	t.Parallel()

	probe := &MockProbe{status: SuccessProbe}
	livenessProbe := NewLivenessProbe(probe)

	// Use httptest.Server to simulate the HTTP server
	server := httptest.NewServer(livenessProbe.Handler)
	defer server.Close()

	resp, err := http.Get(server.URL)
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestServeProbes_UnhealthyReadinessProbe(t *testing.T) {
	t.Parallel()

	probe := &MockProbe{status: FailureProbe}
	readinessProbe := NewReadinessProbe(probe)

	// Use httptest.Server to simulate the HTTP server
	server := httptest.NewServer(readinessProbe.Handler)
	defer server.Close()

	resp, err := http.Get(server.URL)
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)
}

func TestServeProbes_StartupProbe(t *testing.T) {
	t.Parallel()

	probe := &MockProbe{status: SuccessProbe}
	startupProbe := NewStartupProbe(probe)

	// Use httptest.Server to simulate the HTTP server
	server := httptest.NewServer(startupProbe.Handler)
	defer server.Close()

	resp, err := http.Get(server.URL)
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, http.StatusOK, resp.StatusCode)
}
