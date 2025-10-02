package shutdown

import "os"

type StopFn func()

// Stop is a method of the StopFn type that executes the underlying function.
func (s StopFn) Stop() {
	s()
}

type StopInterface interface {
	Stop()
}

type Stopper struct {
	stops []StopFn
}

// New creates a new instance of the Stopper struct.
//
// It returns a pointer to the newly created Stopper struct.
func New() *Stopper {
	return &Stopper{}
}

func (s *Stopper) Register(toStop ...StopInterface) {
	for _, stop := range toStop {
		if stop != nil {
			s.stops = append(s.stops, stop.Stop)
		}
	}
}

// Wait waits for a signal on the given channel and then stops the Stopper.
//
// Parameters:
// - ch: a channel of struct{} type to receive the signal from.
//
// Return type: None.
func (s *Stopper) Wait(ch chan struct{}) {
	<-ch
	s.Stop()
}

// WaitSignal waits for a signal on the given channel and then stops the Stopper.
//
// Parameters:
// - ch: a channel of os.Signal type to receive the signal from.
//
// Return type: None.
func (s *Stopper) WaitSignal(ch chan os.Signal) {
	<-ch
	s.Stop()
}

// Stop stops all registered stop functions.
//
// It iterates over the stops slice and calls the Stop method on each stop function.
// This function does not take any parameters.
// It does not return anything.
func (s *Stopper) Stop() {
	for _, stop := range s.stops {
		stop.Stop()
	}
}
