package shutdown

import "os"

var globalShutdown = New() //nolint:gochecknoglobals // used for main app

// Global returns the global shutdown manager.
//
// It returns a pointer to a Stopper object.
func Global() *Stopper {
	return globalShutdown
}

// Register registers the provided stoppers.
//
// Variadic parameter of type StopInterface.
func Register(toStop ...StopInterface) {
	globalShutdown.Register(toStop...)
}

// RegisterFn registers the provided functions as stoppers.
//
// The variadic parameter `toStop` is a slice of functions that will be registered as stoppers.
// Each function in `toStop` will be converted to a `StopFn` and registered with the global shutdown manager.
func RegisterFn(toStop ...func()) {
	for _, stop := range toStop {
		globalShutdown.Register(StopFn(stop))
	}
}

// Wait waits for a signal on the given channel before stopping the global shutdown manager.
//
// Parameters:
// - ch: The channel to wait on for a signal.
func Wait(ch chan struct{}) {
	globalShutdown.Wait(ch)
}

// WaitSignal waits for a signal on the given channel before stopping the global shutdown manager.
//
// Parameters:
// - ch: The channel to wait on for a signal.
func WaitSignal(ch chan os.Signal) {
	globalShutdown.WaitSignal(ch)
}

// Stop stops the global shutdown manager.
//
// This function does not take any parameters.
// It does not return any values.
func Stop() {
	globalShutdown.Stop()
}
