package testkit_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/stroppy-io/stroppy/v6/pkg/bench"
	"github.com/stroppy-io/stroppy/v6/pkg/bench/testkit"
	"github.com/stroppy-io/stroppy/v6/pkg/record"
)

func TestSuppliedParametersDoNotRequireValidDefaults(t *testing.T) {
	test := bench.Test{Name: "tenant-test", Define: func(d *bench.Def) error {
		tenant, _ := d.Param.String("tenant", "", "Tenant to test.")
		if tenant == "" {
			return errors.New("tenant is required")
		}

		d.Execution.Step("work", func(ctx context.Context, b *bench.Bench) error {
			return b.Exec(ctx, "SELECT :tenant", map[string]any{"tenant": tenant})
		})

		return d.Execution.Err()
	}}
	options := bench.RunOptions{Params: bench.ParamInputs{CLI: map[string]string{"tenant": "example"}}}
	_, err := testkit.Run(t.Context(), test, options)
	require.NoError(t, err)

	recorder := &record.Recorder{}
	_, err = testkit.Record(t.Context(), test, recorder, options)
	require.NoError(t, err)
	require.Len(t, recorder.Operations(), 1)
}

func TestAllBackendDependentDeclarationsRemainDatabaseFree(t *testing.T) {
	for _, kind := range []bench.DriverTypeName{bench.DriverNoop, bench.DriverRecording} {
		t.Run(string(kind), func(t *testing.T) {
			declared := func(d *bench.Def) bench.DriverRef {
				previous := d.Drivers.Declare("default", bench.DriverConfig{Kind: bench.DriverPostgres})
				for _, name := range []string{"primary", "secondary", "tertiary"} {
					if previous.Kind() != kind {
						break
					}

					previous = d.Drivers.Declare(name, bench.DriverConfig{
						Kind: bench.DriverPostgres, URL: "postgres://localhost:1/no-network",
					})
				}

				return previous
			}
			test := bench.Test{Name: "backend-branches", Define: func(d *bench.Def) error {
				ref := declared(d)
				d.Execution.Step("work", func(ctx context.Context, b *bench.Bench) error {
					require.Equal(t, "tertiary", ref.Name())
					require.Equal(t, kind, b.DriverTypeName())

					return b.Exec(ctx, "SELECT 1", nil)
				}, bench.Use(ref))

				return d.Execution.Err()
			}}

			var err error

			if kind == bench.DriverRecording {
				recorder := &record.Recorder{}
				_, err = testkit.Record(t.Context(), test, recorder, bench.RunOptions{})
				require.Len(t, recorder.Operations(), 1)
				require.Equal(t, "tertiary", recorder.Operations()[0].Scope.Database)
			} else {
				_, err = testkit.Run(t.Context(), test, bench.RunOptions{})
			}

			require.NoError(t, err)
		})
	}
}

func TestExecutionOnlyDeclarationUsesForcedBackend(t *testing.T) {
	test := bench.Test{Name: "dynamic-database", Define: func(d *bench.Def) error {
		result := d.Execution.Step("first", func(context.Context, *bench.Bench) error { return nil })
		if result.Status == bench.Completed {
			ref := d.Drivers.Declare("runtime", bench.DriverConfig{Kind: bench.DriverPostgres})
			d.Execution.Step("second", func(ctx context.Context, b *bench.Bench) error {
				require.Equal(t, bench.DriverRecording, b.DriverTypeName())

				return b.Exec(ctx, "SELECT 1", nil)
			}, bench.Use(ref))
		}

		return d.Execution.Err()
	}}
	recorder := &record.Recorder{}
	_, err := testkit.Record(t.Context(), test, recorder, bench.RunOptions{})
	require.NoError(t, err)
	require.Len(t, recorder.Operations(), 1)
	require.Equal(t, "runtime", recorder.Operations()[0].Scope.Database)
}
