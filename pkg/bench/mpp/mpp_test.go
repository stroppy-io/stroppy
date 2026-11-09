package mpp

import (
	"context"
	"errors"
	"testing"

	"github.com/stroppy-io/stroppy/v6/pkg/bench"
	"github.com/stroppy-io/stroppy/v6/pkg/bench/testkit"
	"github.com/stroppy-io/stroppy/v6/pkg/record"
)

func TestSelectRefusesIncoherentRequests(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		req  Request
		want error
	}{
		{"gp with a schema file", Request{Dialect: Dialect, Driver: bench.DriverPostgres, OwnFiles: true}, ErrFiles},
		{"gp with unlogged tables", Request{Dialect: Dialect, Driver: bench.DriverPostgres, Unlogged: true}, ErrUnlog},
		{"gp on mysql", Request{Dialect: Dialect, Driver: bench.DriverMySQL}, ErrDriver},
		{"gp on yugabyte", Request{Dialect: Dialect, Driver: bench.DriverYugabyte}, ErrDriver},
		{"gp on pg", Request{Dialect: Dialect, Driver: bench.DriverPostgres}, nil},
		{"gp dry run on noop", Request{Dialect: Dialect, Driver: bench.DriverNoop}, nil},
		{"gp on recording", Request{Dialect: Dialect, Driver: bench.DriverRecording}, nil},
		{"auto on pg", Request{Driver: bench.DriverPostgres}, nil},
		{"pg dialect with unlogged tables", Request{Dialect: "pg", Driver: bench.DriverPostgres, Unlogged: true}, nil},
		{"auto on mysql", Request{Driver: bench.DriverMySQL}, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			sel, err := Select(tc.req)
			if !errors.Is(err, tc.want) {
				t.Fatalf("Select(%+v) error = %v, want %v", tc.req, err, tc.want)
			}

			if err == nil && sel.GP() != (tc.req.Dialect == Dialect) {
				t.Errorf("GP() = %v for dialect %q", sel.GP(), tc.req.Dialect)
			}
		})
	}
}

// TestStepChecksTheServerOnceForEveryStep is the point of the package: a run may select
// any steps, so the check rides on all of them, and it must neither be skipped when
// the first step is not the first statement nor repeated for each step.
func TestStepChecksTheServerOnceForEveryStep(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name      string
		req       Request
		serverGP  bool
		wantErr   error
		wantProbe int
	}{
		{"gp on an MPP server", Request{Dialect: Dialect, Driver: bench.DriverPostgres}, true, nil, 1},
		{"gp on plain PostgreSQL", Request{Dialect: Dialect, Driver: bench.DriverPostgres}, false, ErrNotGP, 1},
		{"auto on an MPP server", Request{Driver: bench.DriverPostgres}, true, ErrNeedGP, 1},
		{"pg dialect on an MPP server", Request{Dialect: "pg", Driver: bench.DriverPostgres}, true, ErrNeedGP, 1},
		{"auto on plain PostgreSQL", Request{Driver: bench.DriverPostgres}, false, nil, 1},
		{"own files are the author's call", Request{Driver: bench.DriverPostgres, OwnFiles: true}, true, nil, 0},
		{"no probe for other drivers", Request{Driver: bench.DriverMySQL}, false, nil, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var ran int

			test := bench.Test{Name: "mpp-check", Define: func(d *bench.Def) error {
				d.Drivers.Declare("default", bench.DriverConfig{Kind: bench.DriverPostgres})

				sel, err := Select(tc.req)
				if err != nil {
					return err
				}

				count := func(context.Context, *bench.Bench) error {
					ran++

					return nil
				}
				d.Execution.Step("first", sel.Step(count))
				d.Execution.Step("second", sel.Step(count))

				return d.Execution.Err()
			}}

			recorder := &record.Recorder{}
			recorder.Reply(probeSQL, record.Response{Columns: []string{"gp"}, Rows: [][]any{{tc.serverGP}}})

			_, err := testkit.Record(t.Context(), test, recorder, bench.RunOptions{})
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("run error = %v, want %v", err, tc.wantErr)
			}

			if tc.wantErr != nil && ran != 0 {
				t.Errorf("%d steps ran against the wrong dialect", ran)
			}

			if tc.wantErr == nil && ran != 2 {
				t.Errorf("%d steps ran, want 2", ran)
			}

			var probes int

			for _, op := range recorder.Operations() {
				if op.SQL == probeSQL {
					probes++
				}
			}

			if probes != tc.wantProbe {
				t.Errorf("probe issued %d times, want %d", probes, tc.wantProbe)
			}
		})
	}
}

func BenchmarkSelect(b *testing.B) {
	req := Request{Dialect: Dialect, Driver: bench.DriverPostgres}

	for b.Loop() {
		if _, err := Select(req); err != nil {
			b.Fatal(err)
		}
	}
}
