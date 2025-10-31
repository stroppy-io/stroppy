package probes //nolint:testpackage //no need

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// Mock Asyncer to simulate asynchronous behavior.
type MockAsyncer struct {
	wg       sync.WaitGroup
	goCalled int
}

func newMockAsyncer() *MockAsyncer {
	return &MockAsyncer{wg: sync.WaitGroup{}}
}

func (m *MockAsyncer) Go(f func()) {
	m.goCalled++
	m.wg.Add(1)

	go func() {
		f()
		m.wg.Done()
	}()
}

func (m *MockAsyncer) Wait() {
	m.wg.Wait()
}

func TestBoolProbe(t *testing.T) {
	t.Parallel()

	probe := NewBoolProbe()

	// HealthCheck should return FailureProbe initially
	status := probe.HealthCheck(context.Background())
	require.Equal(t, FailureProbe, status)

	// Set to healthy and check again
	probe.Set(true)
	status = probe.HealthCheck(context.Background())
	require.Equal(t, SuccessProbe, status)

	// Set to unhealthy and check again
	probe.Set(false)
	status = probe.HealthCheck(context.Background())
	require.Equal(t, FailureProbe, status)
}

func TestAsyncListProbe_AllSuccess(t *testing.T) {
	t.Parallel()

	mockAsyncer := newMockAsyncer()
	probe1 := NewBoolProbe()
	probe2 := NewBoolProbe()

	probe1.Set(true)
	probe2.Set(true)

	asyncListProbe := NewAsyncListProbe(NewAsyncerFactory(mockAsyncer), probe1, probe2)

	// HealthCheck should return SuccessProbe when all probes succeed
	status := asyncListProbe.HealthCheck(context.Background())
	require.Equal(t, SuccessProbe, status)

	// Ensure async calls were made
	require.Equal(t, 2, mockAsyncer.goCalled)
}

func TestAsyncListProbe_AllFail(t *testing.T) {
	t.Parallel()

	mockAsyncer := newMockAsyncer()
	probe1 := NewBoolProbe()
	probe2 := NewBoolProbe()

	probe1.Set(false)
	probe2.Set(false)

	asyncListProbe := NewAsyncListProbe(NewAsyncerFactory(mockAsyncer), probe1, probe2)

	// HealthCheck should return FailureProbe when all probes fail
	status := asyncListProbe.HealthCheck(context.Background())
	require.Equal(t, FailureProbe, status)

	// Ensure async calls were made
	require.Equal(t, 2, mockAsyncer.goCalled)
}

func TestAsyncListProbe_MixedResults(t *testing.T) {
	t.Parallel()

	mockAsyncer := newMockAsyncer()
	probe1 := NewBoolProbe()
	probe2 := NewBoolProbe()

	probe1.Set(true)  // Success
	probe2.Set(false) // Failure

	asyncListProbe := NewAsyncListProbe(NewAsyncerFactory(mockAsyncer), probe1, probe2)

	// HealthCheck should return SuccessProbe if any probe succeeds
	status := asyncListProbe.HealthCheck(context.Background())
	require.Equal(t, FailureProbe, status)

	// Ensure async calls were made
	require.Equal(t, 2, mockAsyncer.goCalled)
}

func TestAsyncListProbe_Timeout(t *testing.T) {
	t.Parallel()

	mockAsyncer := newMockAsyncer()
	timeoutProbe := NewBoolProbe()
	timeoutProbe.Set(false) // Simulating a probe failure

	asyncListProbe := NewAsyncListProbe(NewAsyncerFactory(mockAsyncer), timeoutProbe)

	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancel()

	// HealthCheck should return FailureProbe if the probe times out or fails
	status := asyncListProbe.HealthCheck(ctx)
	require.Equal(t, FailureProbe, status)

	// Ensure async calls were made
	require.Equal(t, 1, mockAsyncer.goCalled)
}
