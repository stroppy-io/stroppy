//go:build integration

package integration

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stroppy-io/stroppy/v6"
	"github.com/stroppy-io/stroppy/v6/examples/authoring"
	"github.com/stroppy-io/stroppy/v6/pkg/bench"
	"github.com/stroppy-io/stroppy/v6/pkg/gen"
	"github.com/stroppy-io/stroppy/v6/pkg/report"
)

func TestAuthorNativeCopyPanicPropagatesAfterCleanup(t *testing.T) {
	pool := NewPG(t)
	connectionURL, err := url.Parse(envOr(envPGAllURL, defaultPGAllURL))
	if err != nil {
		t.Fatal(err)
	}
	identity := fmt.Sprintf("stroppy-panic-%d", time.Now().UnixNano())
	query := connectionURL.Query()
	query.Set("application_name", identity)
	connectionURL.RawQuery = query.Encode()
	var cleaned atomic.Bool
	source := gen.FromRows(1, func(uint64) (struct{ ID int64 }, error) { panic("native row panic") })
	test := bench.Test{Name: "author/native-panic", Define: func(d *bench.Def) error {
		d.Execution.Step("create", func(ctx context.Context, b *bench.Bench) error {
			return b.Exec(ctx, "CREATE TEMP TABLE stroppy_author_panic (id BIGINT)", nil)
		})
		defer d.Execution.Step("cleanup", func(ctx context.Context, b *bench.Bench) error {
			err := b.Exec(ctx, "DROP TABLE stroppy_author_panic", nil)
			cleaned.Store(err == nil)
			return err
		}, bench.Always(time.Second))
		d.Execution.Step("load", func(ctx context.Context, b *bench.Bench) error {
			_, err := b.Insert(ctx, "stroppy_author_panic", source)
			return err
		})
		return d.Execution.Err()
	}}
	application, err := stroppy.New(test)
	if err != nil {
		t.Fatal(err)
	}
	connections := int32(1)
	request := &stroppy.RunRequest{
		Drivers: map[string]bench.DriverConfig{"default": {
			Kind: bench.DriverPostgres, URL: connectionURL.String(),
			Postgres: &bench.PostgresConfig{MaxConns: &connections},
		}},
		Metrics: &bench.MetricsConfig{Quiet: true, SummaryWriter: io.Discard},
	}
	var value any
	func() {
		defer func() { value = recover() }()
		_, _ = application.Run(t.Context(), request)
	}()
	if value != "native row panic" {
		t.Fatalf("panic = %v", value)
	}
	if !cleaned.Load() {
		t.Fatal("cleanup did not complete after panic")
	}
	var remaining int
	if err := pool.QueryRow(t.Context(), "SELECT count(*) FROM pg_stat_activity WHERE application_name = $1", identity).Scan(&remaining); err != nil {
		t.Fatal(err)
	}
	if remaining != 0 {
		t.Fatalf("driver connections remain after panic: %d", remaining)
	}
}

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
