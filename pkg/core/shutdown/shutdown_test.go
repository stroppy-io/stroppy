package shutdown

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

type mockStopFn struct {
	called bool
}

func (m *mockStopFn) Stop() {
	m.called = true
}

func TestStopFn_Stop(t *testing.T) {
	t.Parallel()

	var called bool

	stopFn := StopFn(func() { called = true })
	stopFn.Stop()

	require.True(t, called)
}

func TestStopper_Register(t *testing.T) {
	t.Parallel()

	stopper := New()
	mock := &mockStopFn{}
	stopper.Register(mock)

	require.Len(t, stopper.stops, 1)
}

func TestStopper_Wait(t *testing.T) {
	t.Parallel()

	stopper := New()
	mock := &mockStopFn{}
	stopper.Register(mock)

	channel := make(chan struct{})
	go func() {
		close(channel)
	}()

	stopper.Wait(channel)

	require.True(t, mock.called)
}

func TestStopper_WaitSignal(t *testing.T) {
	t.Parallel()

	stopper := New()
	mock := &mockStopFn{}
	stopper.Register(mock)

	signals := make(chan os.Signal, 1)
	go func() {
		signals <- os.Interrupt
	}()

	stopper.WaitSignal(signals)

	require.True(t, mock.called)
}

func TestStopper_Stop(t *testing.T) {
	t.Parallel()

	stopper := New()
	mock := &mockStopFn{}
	stopper.Register(mock)

	stopper.Stop()

	require.True(t, mock.called)
}
