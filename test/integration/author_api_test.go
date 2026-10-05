//go:build integration

package integration

import (
	"io"
	"testing"

	"github.com/stroppy-io/stroppy/v6"
	"github.com/stroppy-io/stroppy/v6/examples/authoring"
	"github.com/stroppy-io/stroppy/v6/pkg/bench"
	"github.com/stroppy-io/stroppy/v6/pkg/report"
)

func TestAuthorAccountsOnSQLDrivers(t *testing.T) {
	for _, database := range []struct {
		name string
		kind bench.DriverTypeName
		url  string
	}{
		{"postgres", bench.DriverPostgres, envOr(envPGAllURL, defaultPGAllURL)},
		{"mysql", bench.DriverMySQL, envOr(envMySQLAllURL, defaultMySQLAllURL)},
	} {
		t.Run(database.name, func(t *testing.T) {
			application, err := stroppy.New(authoring.Accounts)
			if err != nil {
				t.Fatal(err)
			}
			result, err := application.Run(t.Context(), &stroppy.RunRequest{
				Drivers: map[string]bench.DriverConfig{"default": {Kind: database.kind, URL: database.url}},
				Params: bench.ParamInputs{CLI: map[string]string{
					"rows": "100", "load-workers": "2", "iterations": "20", "vus": "2",
				}},
				Metrics: &bench.MetricsConfig{Quiet: true, SummaryWriter: io.Discard},
			})
			if err != nil {
				t.Fatal(err)
			}
			if result.Status != report.StatusCompleted {
				t.Fatalf("status = %s", result.Status)
			}
			if *result.Metrics["account_transfers"].Total != 20 {
				t.Fatal("transfer count")
			}
			if *result.Metrics["insert_rows_total"].Total != 100 {
				t.Fatal("load count")
			}
		})
	}
}
