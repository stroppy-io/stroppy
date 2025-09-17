package shutdown

import (
	"os"
	"os/signal"
	"syscall"
)

func NewQuitSignal(sig ...os.Signal) chan os.Signal {
	quit := make(chan os.Signal, 1)

	if len(sig) == 0 {
		sig = append(sig, syscall.SIGINT, syscall.SIGTERM)
	}

	signal.Notify(quit, sig...)

	return quit
}
