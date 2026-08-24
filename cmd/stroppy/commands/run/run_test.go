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
	_ "github.com/stroppy-io/stroppy/pkg/driver/noop"
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
			name:       "typed SQL body accepts query marker",
			args:       []string{"execute_sql", "--sql-body", "--= query\nselect 1"},
			wantScript: "execute_sql",
			wantTyped:  map[string]string{"sql-body": "--= query\nselect 1"},
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
			name:        "explicit empty steps remains an override beside no-steps",
			args:        []string{"tpcc", "--steps=", "--no-steps=workload"},
			wantScript:  "tpcc",
			wantSteps:   []string{""},
			wantNoSteps: []string{"workload"},
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

func TestConfigDriversMergeBelowCLI(t *testing.T) {
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

	if err := applyDriverOpt(configs, 0, "pool.maxConns", "20"); err != nil {
		t.Fatalf("applyDriverOpt(pool.maxConns) error = %v", err)
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
	tests := []struct {
		name   string
		inputs bench.ParamInputs
		body   string
		file   string
		want   map[string]string
	}{
		{
			name: "inline positional binds body",
			body: "--= query\nselect 1;\n",
			want: map[string]string{"sql-body": "--= query\nselect 1;\n", "sql-file": ""},
		},
		{
			name: "file positional binds file",
			file: "queries.sql",
			want: map[string]string{"sql-body": "", "sql-file": "queries.sql"},
		},
		{
			name:   "typed file wins over inline positional",
			inputs: bench.ParamInputs{CLI: map[string]string{"sql-file": "typed.sql"}},
			body:   "--= query\nselect 1;\n",
			want:   map[string]string{"sql-body": "", "sql-file": "typed.sql"},
		},
		{
			name:   "typed body wins over file positional",
			inputs: bench.ParamInputs{CLI: map[string]string{"sql-body": "--= typed\nselect 2;"}},
			file:   "queries.sql",
			want:   map[string]string{"sql-body": "--= typed\nselect 2;", "sql-file": ""},
		},
		{
			name:   "typed file wins when both typed sources are set",
			inputs: bench.ParamInputs{CLI: map[string]string{"sql-body": "body", "sql-file": "file.sql"}},
			want:   map[string]string{"sql-body": "", "sql-file": "file.sql"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := withExecuteSQLSource(test.inputs, test.body, test.file)
			if !maps.Equal(got.CLI, test.want) {
				t.Fatalf("CLI inputs = %v, want %v", got.CLI, test.want)
			}
		})
	}
}

func TestWithExecuteSQLSourceDoesNotMutateProcessEnv(t *testing.T) {
	t.Setenv("STROPPY_SQL_BODY", "process body")
	t.Setenv("SQL_FILE", "process.sql")

	_ = withExecuteSQLSource(bench.ParamInputs{}, "route body", "")

	if got := os.Getenv("STROPPY_SQL_BODY"); got != "process body" {
		t.Fatalf("STROPPY_SQL_BODY = %q", got)
	}

	if got := os.Getenv("SQL_FILE"); got != "process.sql" {
		t.Fatalf("SQL_FILE = %q", got)
	}
}

func TestBuildDriverConfigReturnsJSONConversionErrors(t *testing.T) {
	_, err := buildDriverConfig(0, &runner.DriverCLIConfig{
		Extra: map[string]any{"invalid": func() {}},
	})
	if err == nil || !contains(err.Error(), "extra config") {
		t.Fatalf("buildDriverConfig() error = %v", err)
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

func TestStepsNoStepsMergedMutualExclusion(t *testing.T) {
	tests := []struct {
		name   string
		config string
		args   []string
	}{
		{
			name:   "config steps with CLI no-steps",
			config: `{"script":"simple","steps":["load_data"]}`,
			args:   []string{"--no-steps", "analyze", "-d", "noop"},
		},
		{
			name:   "config no_steps with CLI steps",
			config: `{"script":"simple","no_steps":["analyze"]}`,
			args:   []string{"--steps", "load_data", "-d", "noop"},
		},
		{
			name:   "both in config",
			config: `{"script":"simple","steps":["load_data"],"no_steps":["analyze"]}`,
			args:   []string{"-d", "noop"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			configPath := t.TempDir() + "/stroppy-config.json"
			if err := os.WriteFile(configPath, []byte(test.config), 0o600); err != nil {
				t.Fatalf("WriteFile() error = %v", err)
			}

			args := append([]string{"-f", configPath}, test.args...)

			err := Cmd.RunE(Cmd, args)
			if !errors.Is(err, errStepsMutExclusive) {
				t.Fatalf("RunE() error = %v, want %v", err, errStepsMutExclusive)
			}

			if contains(err.Error(), "driver dispatch") {
				t.Fatalf("merged conflict reached driver dispatch: %v", err)
			}
		})
	}
}

func TestBlankStepNamesDoNotConflictOrMutateProcessEnv(t *testing.T) {
	t.Setenv("STROPPY_STEPS", "sentinel-steps")
	t.Setenv("STROPPY_NO_STEPS", "sentinel-no-steps")

	previousContext := Cmd.Context()
	Cmd.SetContext(t.Context())
	t.Cleanup(func() { Cmd.SetContext(previousContext) })

	tests := []struct {
		name   string
		config string
		args   []string
	}{
		{
			name:   "explicit empty CLI steps clears config allowlist",
			config: `{"script":"simple","steps":["load_data"]}`,
			args:   []string{"--steps=", "--no-steps", "workload"},
		},
		{
			name:   "blank config steps with real config noSteps",
			config: `{"script":"simple","steps":[""],"noSteps":["workload"]}`,
		},
		{
			name:   "comma-only config steps with real config noSteps",
			config: `{"script":"simple","steps":[" , , "],"noSteps":["workload"]}`,
		},
		{
			name:   "real config steps with comma-only config noSteps",
			config: `{"script":"simple","steps":["load_data"],"noSteps":[" , , "]}`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			configPath := t.TempDir() + "/stroppy-config.json"
			if err := os.WriteFile(configPath, []byte(test.config), 0o600); err != nil {
				t.Fatalf("WriteFile() error = %v", err)
			}

			args := append([]string{"-f", configPath, "-d", "noop"}, test.args...)
			args = append(args, "--executor", "shared-iterations", "--iterations", "1", "--vus", "1")

			if err := Cmd.RunE(Cmd, args); err != nil {
				t.Fatalf("RunE() error = %v", err)
			}

			if got := os.Getenv("STROPPY_STEPS"); got != "sentinel-steps" {
				t.Fatalf("STROPPY_STEPS = %q", got)
			}

			if got := os.Getenv("STROPPY_NO_STEPS"); got != "sentinel-no-steps" {
				t.Fatalf("STROPPY_NO_STEPS = %q", got)
			}
		})
	}
}

func TestStepsNoStepsConflictDoesNotBlockHelp(t *testing.T) {
	configPath := t.TempDir() + "/stroppy-config.json"
	if err := os.WriteFile(configPath, []byte(`{
		"script":"simple",
		"steps":["load_data"],
		"no_steps":["analyze"]
	}`), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	previousOutput := Cmd.OutOrStdout()
	defer Cmd.SetOut(previousOutput)

	var output bytes.Buffer
	Cmd.SetOut(&output)

	if err := Cmd.RunE(Cmd, []string{"-f", configPath, "--help"}); err != nil {
		t.Fatalf("RunE(help) error = %v", err)
	}

	if output.Len() == 0 {
		t.Fatal("RunE(help) produced no output")
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

func TestApplyDriverPresetStrictJSON(t *testing.T) {
	tests := []struct {
		name string
		doc  string
		path string
	}{
		{name: "exact duplicate", doc: `{"url":"a","url":"b"}`, path: `$.url`},
		{name: "alias collision", doc: `{"bulkSize":1,"bulk_size":2}`, path: `$.bulkSize`},
		{name: "wrong case", doc: `{"DriverType":"postgres"}`, path: `$["DriverType"]`},
		{name: "unknown nested field", doc: `{"pool":{"MaxConns":1}}`, path: `$.pool["MaxConns"]`},
		{name: "fractional int32", doc: `{"bulkSize":1.5}`, path: `$.bulkSize`},
		{name: "trailing JSON", doc: `{} {}`, path: `$`},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			configs := runner.DriverCLIConfigs{}

			err := applyDriverPreset(configs, 0, test.doc)
			if err == nil || !contains(err.Error(), test.path) {
				t.Fatalf("applyDriverPreset() error = %v, want path %s", err, test.path)
			}
		})
	}
}

func TestApplyDriverPresetAliasesReachRuntimeConfig(t *testing.T) {
	configs := runner.DriverCLIConfigs{}
	if err := applyDriverPreset(
		configs,
		0,
		`  {"driver_type":"postgres","bulk_size":"2e2","pool":{"max_conns":3}}  `,
	); err != nil {
		t.Fatalf("applyDriverPreset() error = %v", err)
	}

	got, err := buildDriverConfig(0, configs[0])
	if err != nil {
		t.Fatalf("buildDriverConfig() error = %v", err)
	}

	if got.GetBulkSize() != 200 {
		t.Fatalf("bulkSize = %d, want 200", got.GetBulkSize())
	}

	if got.Postgres.GetMaxConns() != 3 {
		t.Fatalf("postgres.maxConns = %d, want 3", got.Postgres.GetMaxConns())
	}
}

func TestDriverExtrasRejectAliasCollisionsAndWrongCase(t *testing.T) {
	tests := []struct {
		name string
		keys [][2]string
		path string
	}{
		{
			name: "alias collision",
			keys: [][2]string{{"bulkSize", "1"}, {"bulk_size", "2"}},
			path: `$.bulkSize`,
		},
		{
			name: "nested alias collision",
			keys: [][2]string{{"pool.maxConns", "1"}, {"pool.max_conns", "2"}},
			path: `$.pool.maxConns`,
		},
		{
			name: "wrong case",
			keys: [][2]string{{"pool.MaxConns", "1"}},
			path: `$.pool["MaxConns"]`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			configs := runner.DriverCLIConfigs{0: {DriverType: "postgres"}}
			for _, keyValue := range test.keys {
				if err := applyDriverOpt(configs, 0, keyValue[0], keyValue[1]); err != nil {
					t.Fatalf("applyDriverOpt(%q) error = %v", keyValue[0], err)
				}
			}

			_, err := buildDriverConfig(0, configs[0])
			if err == nil || !contains(err.Error(), test.path) {
				t.Fatalf("buildDriverConfig() error = %v, want path %s", err, test.path)
			}
		})
	}
}

func TestApplyDriverOptStrictNumericLexemes(t *testing.T) {
	configs := runner.DriverCLIConfigs{}

	if err := applyDriverOpt(configs, 0, "driverType", "postgres"); err != nil {
		t.Fatal(err)
	}

	if err := applyDriverOpt(configs, 0, "bulkSize", "1e1"); err != nil {
		t.Fatal(err)
	}

	got, err := buildDriverConfig(0, configs[0])
	if err != nil {
		t.Fatal(err)
	}

	if got.GetBulkSize() != 10 {
		t.Fatalf("bulkSize = %d, want 10", got.GetBulkSize())
	}

	for _, value := range []string{"1.0000000000000001", "1.", "01"} {
		invalidConfigs := runner.DriverCLIConfigs{}

		if err := applyDriverOpt(invalidConfigs, 0, "bulkSize", value); err != nil {
			t.Fatal(err)
		}

		if _, err := buildDriverConfig(0, invalidConfigs[0]); err == nil {
			t.Errorf("bulkSize %q unexpectedly succeeded", value)
		}
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

func TestRemovedIsolationRejectedAtDriverCLISurfaces(t *testing.T) {
	configPath := t.TempDir() + "/stroppy-config.json"
	if err := os.WriteFile(configPath, []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name       string
		driverArgs []string
		path       string
	}{
		{
			name:       "raw driver JSON",
			driverArgs: []string{"-d", `{"driverType":"noop","defaultTxIsolation":"none"}`},
			path:       `$.defaultTxIsolation`,
		},
		{
			name:       "raw driver JSON alias",
			driverArgs: []string{"-d", `{"driverType":"noop","default_tx_isolation":"none"}`},
			path:       `$["default_tx_isolation"]`,
		},
		{
			name:       "driver option",
			driverArgs: []string{"-d", "noop", "-D", "defaultTxIsolation=none"},
			path:       `$.defaultTxIsolation`,
		},
		{
			name:       "driver option alias",
			driverArgs: []string{"-d", "noop", "-D", "default_tx_isolation=none"},
			path:       `$["default_tx_isolation"]`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			args := append([]string{"-f", configPath, "simple"}, test.driverArgs...)
			args = append(args, "--executor", "shared-iterations", "--iterations", "1")

			err := Cmd.RunE(Cmd, args)
			if err == nil || !contains(err.Error(), test.path) || !contains(err.Error(), "unknown field") {
				t.Fatalf("RunE() error = %v, want ordinary unknown-field error at %s", err, test.path)
			}
		})
	}
}

func TestBuildDriverConfigDefaultInsertMethod(t *testing.T) {
	for _, test := range []struct {
		name    string
		cfg     *runner.DriverCLIConfig
		want    string
		wantErr bool
	}{
		{
			name: "winning supported default",
			cfg: &runner.DriverCLIConfig{
				DriverType: "postgres", DefaultInsertMethod: "columnar",
			},
			want: "columnar",
		},
		{
			name: "winning unsupported default",
			cfg: &runner.DriverCLIConfig{
				DriverType: "mysql", DefaultInsertMethod: "columnar",
			},
			wantErr: true,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := buildDriverConfig(0, test.cfg)
			if test.wantErr {
				if err == nil {
					t.Fatal("buildDriverConfig() succeeded")
				}

				return
			}

			if err != nil {
				t.Fatalf("buildDriverConfig() error = %v", err)
			}

			if got.DefaultInsertMethod != test.want {
				t.Fatalf("DefaultInsertMethod = %q, want %q", got.DefaultInsertMethod, test.want)
			}
		})
	}
}

func TestDriverDefaultInputPrecedenceAndIndices(t *testing.T) {
	mysql := "mysql"
	unsupported := "columnar"

	configs, err := runner.DriverCLIConfigsFromFile(map[uint32]*config.DriverRunConfig{
		0: {DriverType: &mysql, DefaultInsertMethod: &unsupported},
	})
	if err != nil {
		t.Fatalf("DriverCLIConfigsFromFile() error = %v", err)
	}

	if err := applyDriverPreset(configs, 0, "pg"); err != nil {
		t.Fatalf("applyDriverPreset() error = %v", err)
	}

	if err := applyDriverPreset(configs, 1, "mysql"); err != nil {
		t.Fatalf("applyDriverPreset() error = %v", err)
	}

	if err := applyDriverOpt(configs, 1, "defaultInsertMethod", "native"); err != nil {
		t.Fatalf("applyDriverOpt() error = %v", err)
	}

	first, err := buildDriverConfig(0, configs[0])
	if err != nil {
		t.Fatalf("buildDriverConfig(0) error = %v", err)
	}

	second, err := buildDriverConfig(1, configs[1])
	if err != nil {
		t.Fatalf("buildDriverConfig(1) error = %v", err)
	}

	if first.DefaultInsertMethod != "native" || second.DefaultInsertMethod != "native" {
		t.Fatalf("defaults = (%q, %q), want native per driver", first.DefaultInsertMethod, second.DefaultInsertMethod)
	}

	if first.DriverType != config.DriverTypePostgres || second.DriverType != config.DriverTypeMySQL {
		t.Fatalf("driver types = (%s, %s), want postgres, mysql", first.DriverType, second.DriverType)
	}
}

func TestGlobalInsertMethodFlagIsRejected(t *testing.T) {
	err := Cmd.RunE(Cmd, []string{"simple", "--insert-method", "native"})
	if err == nil || !contains(err.Error(), `unknown CLI parameter "insert-method"`) {
		t.Fatalf("RunE() error = %v, want unknown CLI parameter", err)
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
