package stroppy_test

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	stroppy "github.com/stroppy-io/stroppy/v6"
	"github.com/stroppy-io/stroppy/v6/pkg/bench"
)

func newWorkload() bench.Test {
	return bench.Test{Name: "external/example", Define: func(d *bench.Def) error {
		settings := bench.RunParameters(&d.Param, bench.RunDefaults{})
		d.Execution.Step("workload", func(context.Context, *bench.Bench) error { return nil }, settings.Policy())

		return d.Execution.Err()
	}}
}

func TestNewRejectsReservedWorkloadName(t *testing.T) {
	_, err := stroppy.New(bench.Test{Name: "probe", Define: func(*bench.Def) error { return nil }})
	if err == nil || !strings.Contains(err.Error(), "reserved workload name") {
		t.Fatalf("New() error = %v", err)
	}
}

func TestApplicationRun(t *testing.T) {
	app, err := stroppy.New(newWorkload())
	if err != nil {
		t.Fatal(err)
	}

	metrics := &bench.MetricsConfig{Quiet: true}

	report, err := app.Run(t.Context(), &stroppy.RunRequest{
		Drivers: map[string]bench.DriverConfig{"": {Kind: bench.DriverNoop}},
		Params:  bench.ParamInputs{CLI: map[string]string{"iterations": "2"}},
		Metrics: metrics,
	})
	if err != nil {
		t.Fatal(err)
	}

	if report.Workload != "external/example" || report.Driver != "noop" {
		t.Fatalf("report identity = %s/%s", report.Workload, report.Driver)
	}

	if metrics.ServiceVersion != "" {
		t.Fatalf("caller metrics service version mutated to %q", metrics.ServiceVersion)
	}
}

func TestApplicationExecute(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	app, err := stroppy.New(newWorkload())
	if err != nil {
		t.Fatal(err)
	}

	for _, test := range []struct {
		name string
		args []string
		want string
	}{
		{"help", []string{"--help"}, "external/example"},
		{"probe", []string{"probe", "-o", "json"}, `"external/example"`},
		{"version", []string{"version", "--json"}, `"stroppy"`},
	} {
		t.Run(test.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer

			if err := app.Execute(t.Context(), test.args, &stdout, &stderr); err != nil {
				t.Fatalf("Execute() error = %v, stderr = %q", err, stderr.String())
			}

			if !strings.Contains(stdout.String(), test.want) {
				t.Fatalf("stdout = %q, want %q", stdout.String(), test.want)
			}
		})
	}

	var stdout, stderr bytes.Buffer

	err = app.Execute(t.Context(), []string{
		"-d", "noop", "--iterations", "2", "--report-format", "json",
	}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("default run error = %v, stderr = %q", err, stderr.String())
	}

	var report map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatalf("decode report: %v\n%s", err, stdout.String())
	}

	if report["workload"] != "external/example" {
		t.Fatalf("workload = %v", report["workload"])
	}
}
