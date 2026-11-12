package postgres

import (
	"context"
	"os"
	"testing"

	"go.uber.org/zap"

	"github.com/stroppy-io/stroppy/v6/pkg/config"
	"github.com/stroppy-io/stroppy/v6/pkg/driver"
	"github.com/stroppy-io/stroppy/v6/pkg/driver/sqldriver"
)

func BenchmarkSuccessFiveArgBinding(b *testing.B) {
	const query = "SELECT :a, :b, :c, :d, :e"

	args := map[string]any{"a": 1, "b": 2, "c": 3, "d": 4, "e": 5}
	if _, _, err := sqldriver.ProcessArgs(PgxDialect{}, query, args); err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()

	for b.Loop() {
		if _, _, err := sqldriver.ProcessArgs(PgxDialect{}, query, args); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkSuccessFiveArgBindingReusable(b *testing.B) {
	const query = "SELECT :a, :b, :c, :d, :e"

	args := map[string]any{"a": 1, "b": 2, "c": 3, "d": 4, "e": 5}

	buffer := make([]any, 0, 5)
	if _, _, err := sqldriver.ProcessArgsInto(PgxDialect{}, query, args, buffer); err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()

	for b.Loop() {
		if _, _, err := sqldriver.ProcessArgsInto(PgxDialect{}, query, args, buffer); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkSuccessRoutinePostgres(b *testing.B) {
	url := os.Getenv("STROPPY_PG_BENCH_URL")
	if url == "" {
		b.Skip("STROPPY_PG_BENCH_URL is required")
	}

	d, err := NewDriver(b.Context(), driver.Options{
		Config: &config.DriverConfig{DriverType: config.DriverTypePostgres, URL: url}, Logger: zap.NewNop(),
	})
	if err != nil {
		b.Fatal(err)
	}

	b.Cleanup(func() { _ = d.Teardown(context.Background()) })

	const query = "SELECT :a::int, :b::int, :c::int, :d::int, :e::int"

	args := map[string]any{"a": 1, "b": 2, "c": 3, "d": 4, "e": 5}
	if _, err := d.RunRoutine(b.Context(), config.TxIsolationLevelUnspecified, query, args); err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()

	for b.Loop() {
		if _, err := d.RunRoutine(b.Context(), config.TxIsolationLevelUnspecified, query, args); err != nil {
			b.Fatal(err)
		}
	}
}
