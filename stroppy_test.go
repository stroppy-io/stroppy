package stroppy_test

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	stroppy "github.com/stroppy-io/stroppy/v6"
	"github.com/stroppy-io/stroppy/v6/pkg/bench"
	"github.com/stroppy-io/stroppy/v6/pkg/config"
)

type namedWorkload struct{ name string }

func (workload *namedWorkload) Name() string                        { return workload.name }
func (*namedWorkload) Define(*bench.Def) error                      { return nil }
func (*namedWorkload) Setup(context.Context, *bench.Bench) error    { return nil }
func (*namedWorkload) Iterate(context.Context, *bench.Bench) error  { return nil }
func (*namedWorkload) Teardown(context.Context, *bench.Bench) error { return nil }

func newWorkload() bench.Workload { return &namedWorkload{name: "external/example"} }

func TestNewRejectsReservedWorkloadName(t *testing.T) {
	_, err := stroppy.New(func() bench.Workload { return &namedWorkload{name: "probe"} })
	if err == nil || !strings.Contains(err.Error(), "reserved workload name") {
		t.Fatalf("New() error = %v", err)
	}
}

func TestApplicationRun(t *testing.T) {
	app, err := stroppy.New(newWorkload)
	if err != nil {
		t.Fatal(err)
	}

	metrics := &bench.MetricsConfig{Quiet: true}

	report, err := app.Run(t.Context(), &stroppy.RunRequest{
		Drivers: map[int]*config.DriverConfig{0: {DriverType: config.DriverTypeNoop}},
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

	app, err := stroppy.New(newWorkload)
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
