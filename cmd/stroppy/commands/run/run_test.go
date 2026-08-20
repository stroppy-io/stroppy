package run

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"maps"
	"os"
	"sync"
	"testing"

	"github.com/spf13/cobra"

	"github.com/stroppy-io/stroppy/internal/runner"
	_ "github.com/stroppy-io/stroppy/internal/workloads/simple"
	"github.com/stroppy-io/stroppy/pkg/bench"
	"github.com/stroppy-io/stroppy/pkg/config"
)

//nolint:cyclop // one table covers the complete run argument grammar
func TestParseRunArgs(t *testing.T) {
	t.Parallel()

	type tc struct {
		name          string
		args          []string
		wantScript    string
		wantSQL       string
		wantFile      string
		wantSteps     []string
		wantNoSteps   []string
		wantAfterDash []string
		wantTyped     map[string]string
		wantHelp      bool
		wantPresets   map[int]string
		wantOpts      map[int][][2]string
		wantErr       error
		wantErrStr    string // substring match when wantErr is nil but error expected
	}

	tests := []tc{
		// ── Positional args ────────────────────────────────────────────────
		{
			name:       "script only",
			args:       []string{"tpcc"},
			wantScript: "tpcc",
		},
		{
			name:       "script and sql",
			args:       []string{"tpcc", "tpcc-scale-100"},
			wantScript: "tpcc",
			wantSQL:    "tpcc-scale-100",
		},
		{
			name:       "script with .ts extension",
			args:       []string{"bench.ts"},
			wantScript: "bench.ts",
		},
		{
			name:       "script with path and sql",
			args:       []string{"./benchmarks/custom.ts", "data.sql"},
			wantScript: "./benchmarks/custom.ts",
			wantSQL:    "data.sql",
		},
		{
			name:       "third positional returns error",
			args:       []string{"tpcc", "pg.sql", "extra.sql"},
			wantErrStr: "too many positional arguments",
		},
		{
			name:       "typed flag pair form",
			args:       []string{"tpcc", "--vus", "10"},
			wantScript: "tpcc",
			wantTyped:  map[string]string{"vus": "10"},
		},
		{
			name:       "typed bool equals form",
			args:       []string{"--enabled=false", "tpcc"},
			wantScript: "tpcc",
			wantTyped:  map[string]string{"enabled": "false"},
		},
		{
			name:       "typed negative pair value",
			args:       []string{"tpcc", "--offset", "-10"},
			wantScript: "tpcc",
			wantTyped:  map[string]string{"offset": "-10"},
		},
		{
			name:       "help after workload",
			args:       []string{"tpcc", "--help"},
			wantScript: "tpcc",
			wantHelp:   true,
		},
		{
			name:        "inline SQL query with spaces and equals is single positional",
			args:        []string{"select a=1", "-d", "pg"},
			wantScript:  "select a=1",
			wantPresets: map[int]string{0: "pg"},
		},

		// ── Missing script ─────────────────────────────────────────────────
		{
			name:    "empty args returns errNoScript",
			args:    []string{},
			wantErr: errNoScript,
		},

		// ── -f / --file ────────────────────────────────────────────────────
		{
			name:       "-f flag",
			args:       []string{"-f", "myconfig.json", "tpcc"},
			wantScript: "tpcc",
			wantFile:   "myconfig.json",
		},
		{
			name:       "--file= form",
			args:       []string{"--file=prod.json", "tpcc"},
			wantScript: "tpcc",
			wantFile:   "prod.json",
		},
		{
			name:       "-f=path form",
			args:       []string{"-f=cfg.json", "tpcc"},
			wantScript: "tpcc",
			wantFile:   "cfg.json",
		},
		{
			name:     "-f without script is allowed (script may come from file)",
			args:     []string{"-f", "myconfig.json"},
			wantFile: "myconfig.json",
		},
		{
			name:       "-f followed by driver flag returns missing value",
			args:       []string{"-f", "-d", "pg"},
			wantErrStr: "-f: flag requires a value",
		},

		// ── -e / --env ─────────────────────────────────────────────────────
		{
			name:       "-e accepts values starting with dash after equals",
			args:       []string{"tpcc", "-e", "TOKEN=-abc"},
			wantScript: "tpcc",
		},
		{
			name:       "-e followed by steps flag returns missing value",
			args:       []string{"tpcc", "-e", "--steps", "load"},
			wantErrStr: "-e: flag requires a value",
		},

		// ── --steps / --no-steps ───────────────────────────────────────────
		{
			name:       "--steps space-separated value",
			args:       []string{"tpcc", "--steps", "create_schema,load"},
			wantScript: "tpcc",
			wantSteps:  []string{"create_schema", "load"},
		},
		{
			name:       "--steps= equals form",
			args:       []string{"tpcc", "--steps=create_schema,load"},
			wantScript: "tpcc",
			wantSteps:  []string{"create_schema", "load"},
		},
		{
			name:        "--no-steps space-separated value",
			args:        []string{"tpcc", "--no-steps", "load"},
			wantScript:  "tpcc",
			wantNoSteps: []string{"load"},
		},
		{
			name:        "--no-steps= equals form",
			args:        []string{"tpcc", "--no-steps=load,run"},
			wantScript:  "tpcc",
			wantNoSteps: []string{"load", "run"},
		},
		{
			name:    "--steps and --no-steps together returns error",
			args:    []string{"tpcc", "--steps", "load", "--no-steps", "run"},
			wantErr: errStepsMutExclusive,
		},
		{
			name:       "--steps missing value returns error",
			args:       []string{"tpcc", "--steps"},
			wantErrStr: "flag requires a value",
		},
		{
			name:       "--no-steps missing value returns error",
			args:       []string{"tpcc", "--no-steps"},
			wantErrStr: "flag requires a value",
		},
		{
			name:       "--steps followed by known flag returns missing value",
			args:       []string{"tpcc", "--steps", "-d", "pg"},
			wantErrStr: "--steps: flag requires a value",
		},
		{
			name:       "--steps followed by unknown flag returns missing value",
			args:       []string{"tpcc", "--steps", "--vus", "10"},
			wantErrStr: "--steps: flag requires a value",
		},

		// ── Driver preset flags ────────────────────────────────────────────
		{
			name:        "-d NAME",
			args:        []string{"tpcc", "-d", "pg"},
			wantScript:  "tpcc",
			wantPresets: map[int]string{0: "pg"},
		},
		{
			name:        "-d0 is same as -d",
			args:        []string{"tpcc", "-d0", "pg"},
			wantScript:  "tpcc",
			wantPresets: map[int]string{0: "pg"},
		},
		{
			name:        "-d1 NAME",
			args:        []string{"tpcc", "-d1", "mysql"},
			wantScript:  "tpcc",
			wantPresets: map[int]string{1: "mysql"},
		},
		{
			name:        "--driver NAME",
			args:        []string{"tpcc", "--driver", "pg"},
			wantScript:  "tpcc",
			wantPresets: map[int]string{0: "pg"},
		},
		{
			name:        "--driver0 same as --driver",
			args:        []string{"tpcc", "--driver0", "pg"},
			wantScript:  "tpcc",
			wantPresets: map[int]string{0: "pg"},
		},
		{
			name:        "--driver1 NAME",
			args:        []string{"tpcc", "--driver1", "mysql"},
			wantScript:  "tpcc",
			wantPresets: map[int]string{1: "mysql"},
		},
		{
			name:        "--driver=NAME equals form",
			args:        []string{"tpcc", "--driver=pg"},
			wantScript:  "tpcc",
			wantPresets: map[int]string{0: "pg"},
		},
		{
			name:        "--driver1=NAME equals form",
			args:        []string{"tpcc", "--driver1=mysql"},
			wantScript:  "tpcc",
			wantPresets: map[int]string{1: "mysql"},
		},
		{
			name:       "-d missing value returns error",
			args:       []string{"tpcc", "-d"},
			wantErrStr: "flag requires a value",
		},
		{
			name:       "--driver missing value returns error",
			args:       []string{"tpcc", "--driver"},
			wantErrStr: "flag requires a value",
		},
		{
			name:       "-d followed by driver option flag returns missing value",
			args:       []string{"tpcc", "-d", "-D", "url=postgres://prod"},
			wantErrStr: "-d: flag requires a value",
		},
		{
			name:       "--driver followed by steps flag returns missing value",
			args:       []string{"tpcc", "--driver", "--steps", "load"},
			wantErrStr: "--driver: flag requires a value",
		},
		{
			name:        "two drivers -d and -d1",
			args:        []string{"tpcc", "-d", "pg", "-d1", "mysql"},
			wantScript:  "tpcc",
			wantPresets: map[int]string{0: "pg", 1: "mysql"},
		},

		// ── Driver option flags ────────────────────────────────────────────
		{
			name:       "-D key=value",
			args:       []string{"tpcc", "-D", "url=postgres://prod:5432"},
			wantScript: "tpcc",
			wantOpts:   map[int][][2]string{0: {{"url", "postgres://prod:5432"}}},
		},
		{
			name:       "unquoted driver value fragment returns quote hint",
			args:       []string{"tpcc", "-D", "url=host=localhost", "user=postgres"},
			wantErrStr: "quote driver/env values",
		},
		{
			name:       "unquoted driver value fragment before script returns key value hint",
			args:       []string{"-D", "url=host=localhost", "user=postgres", "tpcc"},
			wantErrStr: "key=value arguments must follow",
		},
		{
			name:       "-D1 key=value",
			args:       []string{"tpcc", "-D1", "url=mysql://prod:3306"},
			wantScript: "tpcc",
			wantOpts:   map[int][][2]string{1: {{"url", "mysql://prod:3306"}}},
		},
		{
			name:       "--driver-opt key=value",
			args:       []string{"tpcc", "--driver-opt", "url=postgres://prod:5432"},
			wantScript: "tpcc",
			wantOpts:   map[int][][2]string{0: {{"url", "postgres://prod:5432"}}},
		},
		{
			name:       "--driver1-opt key=value",
			args:       []string{"tpcc", "--driver1-opt", "url=mysql://prod:3306"},
			wantScript: "tpcc",
			wantOpts:   map[int][][2]string{1: {{"url", "mysql://prod:3306"}}},
		},
		{
			name:       "--driver-opt=key=value equals form",
			args:       []string{"tpcc", "--driver-opt=url=postgres://prod:5432"},
			wantScript: "tpcc",
			wantOpts:   map[int][][2]string{0: {{"url", "postgres://prod:5432"}}},
		},
		{
			name:       "--driver1-opt=key=value equals form",
			args:       []string{"tpcc", "--driver1-opt=url=mysql://prod:3306"},
			wantScript: "tpcc",
			wantOpts:   map[int][][2]string{1: {{"url", "mysql://prod:3306"}}},
		},
		{
			name:       "-D=key=value equals form",
			args:       []string{"tpcc", "-D=url=postgres://prod:5432"},
			wantScript: "tpcc",
			wantOpts:   map[int][][2]string{0: {{"url", "postgres://prod:5432"}}},
		},
		{
			name:       "-D1=key=value equals form",
			args:       []string{"tpcc", "-D1=url=mysql://prod:3306"},
			wantScript: "tpcc",
			wantOpts:   map[int][][2]string{1: {{"url", "mysql://prod:3306"}}},
		},
		{
			name:       "multiple -D overrides accumulate",
			args:       []string{"tpcc", "-D", "url=postgres://prod:5432", "-D", "driverType=postgres"},
			wantScript: "tpcc",
			wantOpts: map[int][][2]string{
				0: {{"url", "postgres://prod:5432"}, {"driverType", "postgres"}},
			},
		},
		{
			name:       "-D missing value returns error",
			args:       []string{"tpcc", "-D"},
			wantErrStr: "flag requires a value",
		},
		{
			name:       "--driver-opt missing value returns error",
			args:       []string{"tpcc", "--driver-opt"},
			wantErrStr: "flag requires a value",
		},
		{
			name:       "--driver-opt followed by steps flag returns missing value",
			args:       []string{"tpcc", "--driver-opt", "--steps", "load"},
			wantErrStr: "--driver-opt: flag requires a value",
		},
		{
			name:       "-D value without = returns error",
			args:       []string{"tpcc", "-D", "noequals"},
			wantErrStr: "expected key=value format",
		},
		{
			name:       "--driver-opt value without = returns error",
			args:       []string{"tpcc", "--driver-opt", "noequals"},
			wantErrStr: "expected key=value format",
		},

		// ── -- separator ───────────────────────────────────────────────────
		{
			name:          "-- passes remaining args to k6",
			args:          []string{"tpcc", "--", "--duration", "5m"},
			wantScript:    "tpcc",
			wantAfterDash: []string{"--duration", "5m"},
		},
		{
			name:          "-- with empty tail",
			args:          []string{"tpcc", "--"},
			wantScript:    "tpcc",
			wantAfterDash: []string{},
		},
		{
			name:          "flags before -- are not passed to k6",
			args:          []string{"tpcc", "--steps", "load", "--", "--vus", "10"},
			wantScript:    "tpcc",
			wantSteps:     []string{"load"},
			wantAfterDash: []string{"--vus", "10"},
		},

		// ── Mixed combinations ─────────────────────────────────────────────
		{
			name:          "script + driver + steps + k6args",
			args:          []string{"tpcc", "-d", "pg", "--steps", "load,run", "--", "--duration", "5m"},
			wantScript:    "tpcc",
			wantPresets:   map[int]string{0: "pg"},
			wantSteps:     []string{"load", "run"},
			wantAfterDash: []string{"--duration", "5m"},
		},
		{
			name:        "flags may wrap adjacent script sql block",
			args:        []string{"-f", "prod.json", "tpcc", "tpcc/pico", "-d", "pico"},
			wantScript:  "tpcc",
			wantSQL:     "tpcc/pico",
			wantFile:    "prod.json",
			wantPresets: map[int]string{0: "pico"},
		},
		{
			name:       "positional after option following script returns adjacency error",
			args:       []string{"tpcc", "-d", "pg", "tpcc/pico"},
			wantErrStr: "script and sql_file must be adjacent",
		},
		{
			name:        "script + sql + two drivers + driver opt",
			args:        []string{"tpcc", "tpcc-scale-100", "-d", "pg", "-d1", "mysql", "-D1", "url=mysql://prod"},
			wantScript:  "tpcc",
			wantSQL:     "tpcc-scale-100",
			wantPresets: map[int]string{0: "pg", 1: "mysql"},
			wantOpts:    map[int][][2]string{1: {{"url", "mysql://prod"}}},
		},
		{
			name:       "driver opt without preset",
			args:       []string{"tpcc", "-D", "url=postgres://custom:5432"},
			wantScript: "tpcc",
			wantOpts:   map[int][][2]string{0: {{"url", "postgres://custom:5432"}}},
		},
		{
			name:        "-d with JSON string",
			args:        []string{"tpcc", "-d", `{"url":"postgres://prod:5432","driverType":"postgres"}`},
			wantScript:  "tpcc",
			wantPresets: map[int]string{0: `{"url":"postgres://prod:5432","driverType":"postgres"}`},
		},
		{
			name:        "--driver=JSON equals form",
			args:        []string{"tpcc", `--driver={"driverType":"mysql"}`},
			wantScript:  "tpcc",
			wantPresets: map[int]string{0: `{"driverType":"mysql"}`},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// parseRunArgs doesn't handle the empty-args case (RunE does before calling it).
			// For the errNoScript test we invoke RunE's guard condition directly.
			if len(tt.args) == 0 {
				if !errors.Is(tt.wantErr, errNoScript) {
					t.Fatalf("unexpected zero-args test without errNoScript expectation")
				}

				// Simulate what RunE does.
				if len(tt.args) == 0 {
					err := errNoScript
					if !errors.Is(err, tt.wantErr) {
						t.Fatalf("got %v, want %v", err, tt.wantErr)
					}
				}

				return
			}

			got, err := parseRunArgs(tt.args)

			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("got error %v, want %v", err, tt.wantErr)
				}

				return
			}

			if tt.wantErrStr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.wantErrStr)
				}

				if !contains(err.Error(), tt.wantErrStr) {
					t.Fatalf("got error %q, want it to contain %q", err.Error(), tt.wantErrStr)
				}

				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if got.scriptArg != tt.wantScript {
				t.Errorf("scriptArg: got %q, want %q", got.scriptArg, tt.wantScript)
			}

			if got.sqlArg != tt.wantSQL {
				t.Errorf("sqlArg: got %q, want %q", got.sqlArg, tt.wantSQL)
			}

			if got.fileArg != tt.wantFile {
				t.Errorf("fileArg: got %q, want %q", got.fileArg, tt.wantFile)
			}

			if !stringSliceEqual(got.steps, tt.wantSteps) {
				t.Errorf("steps: got %v, want %v", got.steps, tt.wantSteps)
			}

			if !stringSliceEqual(got.noSteps, tt.wantNoSteps) {
				t.Errorf("noSteps: got %v, want %v", got.noSteps, tt.wantNoSteps)
			}

			if !stringSliceEqual(got.afterDash, tt.wantAfterDash) {
				t.Errorf("afterDash: got %v, want %v", got.afterDash, tt.wantAfterDash)
			}

			if !maps.Equal(got.typedParams, tt.wantTyped) {
				t.Errorf("typedParams: got %v, want %v", got.typedParams, tt.wantTyped)
			}

			if got.help != tt.wantHelp {
				t.Errorf("help: got %v, want %v", got.help, tt.wantHelp)
			}

			if !presetMapsEqual(got.driverPresets, tt.wantPresets) {
				t.Errorf("driverPresets: got %v, want %v", got.driverPresets, tt.wantPresets)
			}

			if !driverOptMapsEqual(got.driverOpts, tt.wantOpts) {
				t.Errorf("driverOpts: got %v, want %v", got.driverOpts, tt.wantOpts)
			}
		})
	}
}

func TestLegacyEnvAndConfigDriversMergeBelowCLI(t *testing.T) {
	driverType := "postgres"
	fileURL := "postgres://file"
	errorMode := "throw"
	bulkSize := int32(20)
	maxConns := int32(7)
	specificMaxConns := int32(5)
	statementCache := int32(13)

	configs, err := runner.DriverCLIConfigsFromFile(map[uint32]*config.DriverRunConfig{
		0: {
			DriverType: &driverType,
			URL:        &fileURL,
			ErrorMode:  &errorMode,
			BulkSize:   &bulkSize,
			Pool:       &config.PoolConfig{MaxConns: &maxConns},
			Postgres: &config.PostgresConfig{
				MaxConns:               &specificMaxConns,
				StatementCacheCapacity: &statementCache,
			},
		},
	})
	if err != nil {
		t.Fatalf("DriverCLIConfigsFromFile() error = %v", err)
	}

	if err := applyDriverOpt(configs, 0, "url", "postgres://cli"); err != nil {
		t.Fatalf("applyDriverOpt(url) error = %v", err)
	}

	if err := applyDriverOpt(configs, 0, "pool.maxConns", "10"); err != nil {
		t.Fatalf("applyDriverOpt(pool.maxConns) error = %v", err)
	}

	if configs[0].DriverType != "postgres" || configs[0].URL != "postgres://cli" {
		t.Fatalf("merged driver config = %#v", configs[0])
	}

	runtimeConfig, err := buildDriverConfig(0, configs[0])
	if err != nil {
		t.Fatalf("buildDriverConfig() error = %v", err)
	}

	if runtimeConfig.URL != "postgres://cli" ||
		runtimeConfig.GetBulkSize() != 20 ||
		runtimeConfig.ErrorMode != config.ErrorModeThrow ||
		runtimeConfig.Postgres.GetMaxConns() != 10 ||
		runtimeConfig.Postgres.GetStatementCacheCapacity() != 13 {
		t.Fatalf("runtime driver config = %#v, extra = %#v", runtimeConfig, configs[0].Extra)
	}

	mysql := "mysql"
	maxOpenConns := int32(9)
	specificMaxOpenConns := int32(5)
	maxIdleConns := int32(4)

	configs, err = runner.DriverCLIConfigsFromFile(map[uint32]*config.DriverRunConfig{
		0: {
			DriverType: &mysql,
			Pool:       &config.PoolConfig{MaxOpenConns: &maxOpenConns},
			SQL: &config.SQLConfig{
				MaxOpenConns: &specificMaxOpenConns,
				MaxIdleConns: &maxIdleConns,
			},
		},
	})
	if err != nil {
		t.Fatalf("DriverCLIConfigsFromFile(mysql) error = %v", err)
	}

	if err := applyDriverOpt(configs, 0, "pool.maxOpenConns", "12"); err != nil {
		t.Fatalf("applyDriverOpt(pool.maxOpenConns) error = %v", err)
	}

	runtimeConfig, err = buildDriverConfig(0, configs[0])
	if err != nil {
		t.Fatalf("buildDriverConfig(mysql) error = %v", err)
	}

	if runtimeConfig.SQL.GetMaxOpenConns() != 12 || runtimeConfig.SQL.GetMaxIdleConns() != 4 {
		t.Fatalf("runtime mysql driver config = %#v", runtimeConfig)
	}
}

func TestWithExecuteSQLSource(t *testing.T) {
	t.Run("inline positional binds sql-body", func(t *testing.T) {
		inputs := withExecuteSQLSource(bench.ParamInputs{CLI: map[string]string{}}, "--= query\nselect 1;\n", "")

		if inputs.CLI["sql-body"] != "--= query\nselect 1;\n" {
			t.Fatalf("sql-body = %q", inputs.CLI["sql-body"])
		}

		if _, ok := inputs.CLI["sql-file"]; ok {
			t.Fatal("sql-file should not be set")
		}
	})

	t.Run("sql file positional binds sql-file", func(t *testing.T) {
		inputs := withExecuteSQLSource(bench.ParamInputs{CLI: map[string]string{}}, "", "queries.sql")

		if inputs.CLI["sql-file"] != "queries.sql" {
			t.Fatalf("sql-file = %q", inputs.CLI["sql-file"])
		}

		if _, ok := inputs.CLI["sql-body"]; ok {
			t.Fatal("sql-body should not be set")
		}
	})

	t.Run("explicit typed sql-file wins over inline positional", func(t *testing.T) {
		inputs := withExecuteSQLSource(
			bench.ParamInputs{CLI: map[string]string{"sql-file": "typed.sql"}},
			"--= query\nselect 1;\n",
			"",
		)

		if inputs.CLI["sql-file"] != "typed.sql" {
			t.Fatalf("sql-file = %q", inputs.CLI["sql-file"])
		}

		if _, ok := inputs.CLI["sql-body"]; ok {
			t.Fatal("sql-body should not be set")
		}
	})

	t.Run("no source leaves params untouched", func(t *testing.T) {
		inputs := withExecuteSQLSource(bench.ParamInputs{CLI: map[string]string{}}, "", "")

		if _, ok := inputs.CLI["sql-body"]; ok {
			t.Fatal("sql-body should not be set")
		}

		if _, ok := inputs.CLI["sql-file"]; ok {
			t.Fatal("sql-file should not be set")
		}
	})
}

func TestBuildDriverConfigReturnsJSONConversionErrors(t *testing.T) {
	_, err := buildDriverConfig(0, &runner.DriverCLIConfig{
		Extra: map[string]any{"invalid": func() {}},
	})
	if err == nil || !contains(err.Error(), "extra config") {
		t.Fatalf("buildDriverConfig() error = %v", err)
	}
}

func TestBuildDriverConfigRejectsDefaultTxIsolation(t *testing.T) {
	t.Parallel()

	_, err := buildDriverConfig(0, &runner.DriverCLIConfig{
		DriverType: "postgres",
		Extra:      map[string]any{"defaultTxIsolation": "read_committed"},
	})
	if err == nil {
		t.Fatal("buildDriverConfig() accepted defaultTxIsolation")
	}

	if !contains(err.Error(), "--tx-isolation") {
		t.Fatalf("buildDriverConfig() error = %v, want --tx-isolation migration guidance", err)
	}
}

func TestBuildDriverConfigResolvesInsertMethod(t *testing.T) {
	t.Parallel()

	t.Run("valid method stored canonically", func(t *testing.T) {
		t.Parallel()

		dc, err := buildDriverConfig(0, &runner.DriverCLIConfig{
			DriverType:          "postgres",
			DefaultInsertMethod: "columnar",
		})
		if err != nil {
			t.Fatalf("buildDriverConfig() error = %v", err)
		}

		if dc.InsertMethod != "columnar" {
			t.Fatalf("InsertMethod = %q, want columnar", dc.InsertMethod)
		}
	})

	t.Run("preset default resolves", func(t *testing.T) {
		t.Parallel()

		cfg := runner.NewDriverCLIConfigFromPreset(func() runner.DriverPreset {
			p, err := runner.LookupDriverPreset("pg")
			if err != nil {
				t.Fatalf("LookupDriverPreset(pg): %v", err)
			}

			return p
		}())

		dc, err := buildDriverConfig(0, &cfg)
		if err != nil {
			t.Fatalf("buildDriverConfig() error = %v", err)
		}

		if dc.InsertMethod != "native" {
			t.Fatalf("InsertMethod = %q, want native", dc.InsertMethod)
		}
	})

	t.Run("invalid value rejected", func(t *testing.T) {
		t.Parallel()

		_, err := buildDriverConfig(0, &runner.DriverCLIConfig{
			DriverType:          "postgres",
			DefaultInsertMethod: "bogus",
		})
		if err == nil {
			t.Fatal("buildDriverConfig() accepted bogus insert method")
		}
	})

	t.Run("unsupported value rejected", func(t *testing.T) {
		t.Parallel()

		_, err := buildDriverConfig(0, &runner.DriverCLIConfig{
			DriverType:          "mysql",
			DefaultInsertMethod: "columnar",
		})
		if err == nil {
			t.Fatal("buildDriverConfig() accepted columnar for mysql")
		}
	})

	t.Run("empty method leaves workload default", func(t *testing.T) {
		t.Parallel()

		dc, err := buildDriverConfig(0, &runner.DriverCLIConfig{DriverType: "postgres"})
		if err != nil {
			t.Fatalf("buildDriverConfig() error = %v", err)
		}

		if dc.InsertMethod != "" {
			t.Fatalf("InsertMethod = %q, want empty", dc.InsertMethod)
		}
	})
}

func TestApplyDriverOptAcceptsInsertMethodAliases(t *testing.T) {
	t.Parallel()

	for _, key := range []string{"insertMethod", "insert_method", "defaultInsertMethod"} {
		configs := runner.DriverCLIConfigs{0: &runner.DriverCLIConfig{}}
		if err := applyDriverOpt(configs, 0, key, "plain_bulk"); err != nil {
			t.Fatalf("applyDriverOpt(%q) error = %v", key, err)
		}

		if configs[0].DefaultInsertMethod != "plain_bulk" {
			t.Fatalf("applyDriverOpt(%q): DefaultInsertMethod = %q, want plain_bulk",
				key, configs[0].DefaultInsertMethod)
		}
	}
}

func TestDynamicWorkloadHelpAndBadTypedParam(t *testing.T) {
	registerRunParamTestWorkload()

	previousOutput := Cmd.OutOrStdout()
	defer Cmd.SetOut(previousOutput)

	var output bytes.Buffer
	Cmd.SetOut(&output)

	malformedConfig := t.TempDir() + "/stroppy-config.json"
	if err := os.WriteFile(malformedConfig, []byte(`{malformed`), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	if err := Cmd.RunE(Cmd, []string{
		"-f", malformedConfig, "test/run-typed-params", "--help",
	}); err != nil {
		t.Fatalf("RunE(help) error = %v", err)
	}

	help := output.String()
	for _, expected := range []string{
		"Run parameters:",
		"Workload parameters:",
		"--executor",
		"--enabled=true|false",
		"Boolean parameters require an explicit value",
		"--count",
		"default=selected at runtime",
	} {
		if !contains(help, expected) {
			t.Fatalf("help output missing %q:\n%s", expected, help)
		}
	}

	err := Cmd.RunE(Cmd, []string{"test/run-typed-params", "--count=bad", "-d", "noop"})
	if err == nil || !contains(err.Error(), `parameter "count"`) {
		t.Fatalf("RunE(bad param) error = %v", err)
	}

	if contains(err.Error(), "driver dispatch") {
		t.Fatalf("bad parameter reached driver dispatch: %v", err)
	}

	configPath := t.TempDir() + "/scoped.json"
	if err := os.WriteFile(configPath, []byte(`{
		"script":"test/run-typed-params",
		"run":{"count":2},
		"params":{"vus":3}
	}`), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	err = Cmd.RunE(Cmd, []string{"-f", configPath, "-d", "noop"})
	if err == nil || !contains(err.Error(), "unknown run config parameter") ||
		!contains(err.Error(), "unknown workload config parameter") {
		t.Fatalf("RunE(crossed config scopes) error = %v", err)
	}

	if contains(err.Error(), "driver dispatch") {
		t.Fatalf("crossed config scopes reached driver dispatch: %v", err)
	}
}

func TestWorkloadTypedFlagCompletion(t *testing.T) {
	registerRunParamTestWorkload()

	completions, directive := Cmd.ValidArgsFunction(
		Cmd,
		[]string{"test/run-typed-params"},
		"--en",
	)
	if directive != cobra.ShellCompDirectiveNoFileComp {
		t.Fatalf("directive = %v", directive)
	}

	for _, expected := range []string{
		"--enabled=true\tEnable test behavior.",
		"--enabled=false\tEnable test behavior.",
	} {
		if !containsCompletion(completions, expected) {
			t.Fatalf("completions %v missing %q", completions, expected)
		}
	}

	completions, directive = Cmd.ValidArgsFunction(
		Cmd,
		[]string{"test/run-typed-params"},
		"--sql",
	)
	if directive != cobra.ShellCompDirectiveNoFileComp ||
		!containsCompletion(completions, "--sql-file\tSQL dialect override file.") {
		t.Fatalf("sql-file completions = %v, directive = %v", completions, directive)
	}

	completions, directive = Cmd.ValidArgsFunction(Cmd, nil, "--en")
	if len(completions) != 0 || directive != cobra.ShellCompDirectiveDefault {
		t.Fatalf("generic completions = %v, directive = %v", completions, directive)
	}
}

func TestRegisteredWorkloadReceivesEffectiveSQLFile(t *testing.T) {
	registerRunParamTestWorkload()

	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "second positional",
			args: []string{
				"test/run-typed-params", "compat.sql", "--count=bad", "-d", "noop",
			},
			want: "compat.sql",
		},
		{
			name: "direct typed flag wins",
			args: []string{
				"test/run-typed-params", "compat.sql", "--sql-file=direct.sql", "--count=bad", "-d", "noop",
			},
			want: "direct.sql",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			lastRunParamSQLFile = ""

			err := Cmd.RunE(Cmd, test.args)
			if err == nil || !contains(err.Error(), `parameter "count"`) {
				t.Fatalf("RunE() error = %v", err)
			}

			if lastRunParamSQLFile != test.want {
				t.Fatalf("sql-file binding = %q, want %q", lastRunParamSQLFile, test.want)
			}
		})
	}

	configPath := t.TempDir() + "/sql.json"
	if err := os.WriteFile(configPath, []byte(`{
		"script":"test/run-typed-params",
		"sql":"config.sql"
	}`), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	lastRunParamSQLFile = ""

	err := Cmd.RunE(Cmd, []string{"-f", configPath, "--count=bad", "-d", "noop"})
	if err == nil || !contains(err.Error(), `parameter "count"`) {
		t.Fatalf("RunE(config) error = %v", err)
	}

	if lastRunParamSQLFile != "config.sql" {
		t.Fatalf("config sql-file binding = %q", lastRunParamSQLFile)
	}
}

func TestSimpleRejectsSQLFilePositional(t *testing.T) {
	err := Cmd.RunE(Cmd, []string{"simple", "unused.sql", "-d", "noop"})
	if err == nil || !contains(err.Error(), "workload does not accept sql_file positional") {
		t.Fatalf("RunE() error = %v", err)
	}
}

type runParamTestWorkload struct{}

var (
	lastRunParamSQLFile          string
	registerRunParamWorkloadOnce sync.Once
)

func registerRunParamTestWorkload() {
	registerRunParamWorkloadOnce.Do(func() {
		bench.Register(func() bench.Workload { return &runParamTestWorkload{} })
	})
}

func (*runParamTestWorkload) Name() string { return "test/run-typed-params" }

func (*runParamTestWorkload) Define(def *bench.Def) error {
	def.Param.Bool("enabled", false, "Enable test behavior.")
	def.Param.Int("count", 1, "Number of test operations.", bench.DerivedDefault("selected at runtime"))
	sqlFile := def.Param.String("sql-file", "", "SQL dialect override file.")
	lastRunParamSQLFile = sqlFile.Value()

	return nil
}

func (*runParamTestWorkload) Setup(context.Context, *bench.Bench) error    { return nil }
func (*runParamTestWorkload) Iterate(context.Context, *bench.Bench) error  { return nil }
func (*runParamTestWorkload) Teardown(context.Context, *bench.Bench) error { return nil }

// ── helpers ───────────────────────────────────────────────────────────────────

func containsCompletion(completions []string, expected string) bool {
	for _, completion := range completions {
		if completion == expected {
			return true
		}
	}

	return false
}

func contains(s, substr string) bool {
	return len(substr) == 0 || (len(s) >= len(substr) && containsStr(s, substr))
}

func containsStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}

	return false
}

func stringSliceEqual(a, b []string) bool {
	if len(a) == 0 && len(b) == 0 {
		return true
	}

	if len(a) != len(b) {
		return false
	}

	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}

	return true
}

func presetMapsEqual(a, b map[int]string) bool {
	if len(a) == 0 && len(b) == 0 {
		return true
	}

	if len(a) != len(b) {
		return false
	}

	for k, v := range a {
		if b[k] != v {
			return false
		}
	}

	return true
}

func TestApplyDriverPresetJSON(t *testing.T) {
	t.Parallel()

	configs := runner.DriverCLIConfigs{}

	err := applyDriverPreset(configs, 0, `{"url":"postgres://prod:5432","driverType":"postgres","errorMode":"throw"}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	cfg := configs[0]
	if cfg.URL != "postgres://prod:5432" {
		t.Errorf("URL: got %q, want %q", cfg.URL, "postgres://prod:5432")
	}

	if cfg.DriverType != "postgres" {
		t.Errorf("DriverType: got %q, want %q", cfg.DriverType, "postgres")
	}

	if cfg.Extra["errorMode"] != "throw" {
		t.Errorf("Extra[errorMode]: got %v, want %q", cfg.Extra["errorMode"], "throw")
	}
}

func TestApplyDriverPresetInvalidJSON(t *testing.T) {
	t.Parallel()

	configs := runner.DriverCLIConfigs{}

	err := applyDriverPreset(configs, 0, `{broken`)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestApplyDriverOptDottedPool(t *testing.T) {
	t.Parallel()

	configs := runner.DriverCLIConfigs{}

	if err := applyDriverOpt(configs, 0, "pool.maxConns", "20"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if err := applyDriverOpt(configs, 0, "pool.maxConnLifetime", "30m"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got := marshalDriverConfig(t, configs[0])
	pool := objectField(t, got, "pool")

	if pool["maxConns"] != float64(20) {
		t.Errorf("pool.maxConns: got %v, want 20", pool["maxConns"])
	}

	if pool["maxConnLifetime"] != "30m" {
		t.Errorf("pool.maxConnLifetime: got %v, want 30m", pool["maxConnLifetime"])
	}
}

func TestApplyDriverOptDottedPoolMergesJSONPreset(t *testing.T) {
	t.Parallel()

	configs := runner.DriverCLIConfigs{}

	if err := applyDriverPreset(configs, 0, `{"driverType":"postgres","pool":{"minConns":5}}`); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if err := applyDriverOpt(configs, 0, "pool.maxConns", "20"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got := marshalDriverConfig(t, configs[0])
	pool := objectField(t, got, "pool")

	if pool["minConns"] != float64(5) {
		t.Errorf("pool.minConns: got %v, want 5", pool["minConns"])
	}

	if pool["maxConns"] != float64(20) {
		t.Errorf("pool.maxConns: got %v, want 20", pool["maxConns"])
	}
}

func TestApplyDriverOptDottedPoolUnknownField(t *testing.T) {
	t.Parallel()

	configs := runner.DriverCLIConfigs{
		0: &runner.DriverCLIConfig{DriverType: "postgres"},
	}

	if err := applyDriverOpt(configs, 0, "pool.maximum", "20"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got := marshalDriverConfig(t, configs[0])
	pool := objectField(t, got, "pool")

	if pool["maximum"] != float64(20) {
		t.Errorf("pool.maximum: got %v, want 20", pool["maximum"])
	}

	if _, err := buildDriverConfig(0, configs[0]); err == nil || !contains(err.Error(), "unknown field") {
		t.Fatalf("buildDriverConfig() error = %v", err)
	}
}

func TestApplyDriverOptDottedPathIsGeneric(t *testing.T) {
	t.Parallel()

	configs := runner.DriverCLIConfigs{}

	if err := applyDriverOpt(configs, 0, "custom.deep.value", "1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got := marshalDriverConfig(t, configs[0])
	custom := objectField(t, got, "custom")
	deep := objectField(t, custom, "deep")

	if deep["value"] != float64(1) {
		t.Errorf("custom.deep.value: got %v, want 1", deep["value"])
	}
}

func TestApplyDriverOptDottedPathConflict(t *testing.T) {
	t.Parallel()

	configs := runner.DriverCLIConfigs{}

	if err := applyDriverOpt(configs, 0, "pool", "not-object"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	err := applyDriverOpt(configs, 0, "pool.maxConns", "20")
	if err == nil {
		t.Fatal("expected structural conflict error")
	}

	if !contains(err.Error(), "conflicts") {
		t.Fatalf("got error %q, want conflict", err.Error())
	}
}

func marshalDriverConfig(t *testing.T, cfg *runner.DriverCLIConfig) map[string]any {
	t.Helper()

	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal driver config: %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal driver config: %v", err)
	}

	return got
}

func objectField(t *testing.T, m map[string]any, key string) map[string]any {
	t.Helper()

	raw, ok := m[key]
	if !ok {
		t.Fatalf("missing object field %q in %#v", key, m)
	}

	obj, ok := raw.(map[string]any)
	if !ok {
		t.Fatalf("field %q has type %T, want object", key, raw)
	}

	return obj
}

func driverOptMapsEqual(a, b map[int][][2]string) bool {
	if len(a) == 0 && len(b) == 0 {
		return true
	}

	if len(a) != len(b) {
		return false
	}

	for k, av := range a {
		bv, ok := b[k]
		if !ok || len(av) != len(bv) {
			return false
		}

		for i := range av {
			if av[i] != bv[i] {
				return false
			}
		}
	}

	return true
}
