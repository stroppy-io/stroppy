package main

import (
	"context"
	"fmt"
	"os"

	"github.com/stroppy-io/stroppy/v6"
	"github.com/stroppy-io/stroppy/v6/examples/authoring"
	"github.com/stroppy-io/stroppy/v6/pkg/bench"
)

func main() {
	catalog, err := bench.NewCatalog(authoring.Query, authoring.Accounts, authoring.Mirror)
	if err == nil {
		var application *stroppy.Application

		application, err = stroppy.NewCatalog(catalog)
		if err == nil {
			err = application.Execute(context.Background(), os.Args[1:], os.Stdout, os.Stderr)
		}
	}

	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
