package commands

import (
	"fmt"
	"os"
	"os/signal"
	"slices"
	"sync"
	"time"

	"go.k6.io/k6/cmd/state"
)

func inteceptInteruptSignals(gs *state.GlobalState) {
	var caught sync.Map // map[chan<- os.Signal]chan struct{}

	gs.SignalNotify = func(sigChan chan<- os.Signal, signals ...os.Signal) {
		if !slices.Contains(signals, os.Interrupt) {
			signal.Notify(sigChan, signals...) // do as is

			return
		}

		// pass non-interrupt signals directly
		rest := slices.DeleteFunc(slices.Clone(signals),
			func(s os.Signal) bool { return s == os.Interrupt })
		if len(rest) > 0 {
			signal.Notify(sigChan, rest...)
		}

		// check if this channel is already handled
		if _, loaded := caught.Load(sigChan); loaded {
			return
		}

		stopper := make(chan struct{}, 1)
		caught.Store(sigChan, stopper)

		go doubleConfirmationSigInt(sigChan, stopper)
	}

	gs.SignalStop = func(sigChan chan<- os.Signal) {
		if val, yes := caught.LoadAndDelete(sigChan); yes {
			if stopper, ok := val.(chan struct{}); ok {
				stopper <- struct{}{} // doubleConfirmation will stop its channel by itself

				return
			}

			panic("caught sync.Map stored type corrupted")
		}

		signal.Stop(sigChan)
	}
}

func doubleConfirmationSigInt(sigChan chan<- os.Signal, stopper chan struct{}) {
	const (
		nonBlockingBufferSize = 2
		confirmationWindow    = 5 * time.Second
	)

	sigWaiter := make(chan os.Signal, nonBlockingBufferSize)
	signal.Notify(sigWaiter, os.Interrupt)

	var confirmTimer *time.Timer

loop:
	for {
		select {
		case sig := <-sigWaiter:
			if confirmTimer != nil { // have timer -> second signal within 5s
				fmt.Fprintf(os.Stdout, "\nReceived second interrupt, stopping...\n")
				signal.Stop(sigWaiter)
				signal.Notify(sigChan, os.Interrupt) // restore direct signal delivery to k6

				sigChan <- sig // forward the confirming signal

				break loop
			}

			// first signal -> set timer, ask user to confirm
			fmt.Fprintf(os.Stdout, "\nInterrupt received. Press Ctrl+C again within 5s to stop.\n")

			confirmTimer = time.AfterFunc(confirmationWindow, func() {
				confirmTimer = nil

				fmt.Fprintf(os.Stdout, "\nConfirmation window expired. Test continues.\n")
			})
		case <-stopper: // release goroutine
			signal.Stop(sigWaiter)

			break loop
		}
	}
}
