package main

import (
	"os"
	"os/signal"
	"syscall"

	"github.com/stroppy-io/stroppy-cloud-panel/cmd/application"
	"github.com/stroppy-io/stroppy/pkg/core/shutdown"
)

func makeQuitSignal() chan os.Signal {
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	return quit
}

func main() {
	app, err := application.New()
	if err != nil {
		panic(err)
	}

	if err := app.Run(); err != nil {
		panic(err)
	}

	shutdown.WaitSignal(makeQuitSignal())
}
