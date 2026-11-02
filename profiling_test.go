package stroppy_test

import (
	"bytes"
	"context"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"

	stroppy "github.com/stroppy-io/stroppy/v6"
	"github.com/stroppy-io/stroppy/v6/pkg/bench"
	"github.com/stroppy-io/stroppy/v6/pkg/profiling"
)

func TestApplicationProfiling(t *testing.T) {
	for _, mode := range []string{"environment", "request", "bench", "standalone"} {
		t.Run(mode, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())

			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}

			address := listener.Addr().String()
			listener.Close()

			config := &profiling.Config{Address: address}
			if mode == "environment" {
				t.Setenv("STROPPY_PPROF", address)
			}

			observed := 0
			test := bench.Test{Name: "external/profile", Define: func(d *bench.Def) error {
				d.Execution.Step("workload", func(ctx context.Context, _ *bench.Bench) error {
					request, err := http.NewRequestWithContext(ctx, http.MethodGet,
						"http://"+address+"/debug/pprof/heap", nil)
					if err != nil {
						return err
					}

					response, err := http.DefaultClient.Do(request)
					if err != nil {
						return err
					}
					defer response.Body.Close()

					if response.StatusCode != http.StatusOK {
						t.Errorf("heap status = %d", response.StatusCode)
					}

					observed++

					return nil
				})

				return d.Execution.Err()
			}}

			app, err := stroppy.New(test)
			if err != nil {
				t.Fatal(err)
			}

			var stdout, stderr bytes.Buffer

			switch mode {
			case "standalone":
				err = app.Execute(t.Context(), []string{
					"-d", "noop", "--pprof", address, "--no-report",
				}, &stdout, &stderr)
				if !strings.Contains(stderr.String(), "pprof listening on http://"+address) {
					t.Errorf("missing listener diagnostic: %s", &stderr)
				}
			case "bench":
				_, err = bench.RunTest(t.Context(), test, bench.RunOptions{
					Profiling: config, Metrics: &bench.MetricsConfig{Quiet: true, SummaryWriter: io.Discard},
				})
			case "request", "environment":
				request := &stroppy.RunRequest{
					NoReport: true, Metrics: &bench.MetricsConfig{Quiet: true, SummaryWriter: io.Discard},
				}
				if mode == "request" {
					request.Profiling = config
				}

				_, err = app.Run(t.Context(), request)
			}

			if err != nil {
				t.Fatalf("run = %v, stderr = %s", err, &stderr)
			}

			if observed != 1 {
				t.Fatalf("executed %d actions", observed)
			}

			listener, err = net.Listen("tcp", address)
			if err != nil {
				t.Fatalf("profiling listener leaked: %v", err)
			}

			listener.Close()
		})
	}
}
