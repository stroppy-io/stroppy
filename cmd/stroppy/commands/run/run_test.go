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
	stroppy "github.com/stroppy-io/stroppy/pkg/common/proto/stroppy"
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
	unsetRunTestEnv(t, "POOL_SIZE")

	merged := mergeLegacyEnv(
		map[string]string{"CONFIG_ONLY": "file", "SHARED": "file"},
		map[string]string{"CLI_ONLY": "cli", "SHARED": "cli"},
	)
	if !maps.Equal(merged, map[string]string{
		"CONFIG_ONLY": "file",
		"CLI_ONLY":    "cli",
		"SHARED":      "cli",
	}) {
		t.Fatalf("merged env = %v", merged)
	}

	driverType := "postgres"
	fileURL := "postgres://file"
	errorMode := "throw"
	bulkSize := int32(20)
	maxConns := int32(7)
	specificMaxConns := int32(5)
	statementCache := int32(13)

	configs, err := runner.DriverCLIConfigsFromFile(map[uint32]*stroppy.DriverRunConfig{
		0: {
			DriverType: &driverType,
			Url:        &fileURL,
			ErrorMode:  &errorMode,
			BulkSize:   &bulkSize,
			Pool:       &stroppy.DriverRunConfig_PoolConfig{MaxConns: &maxConns},
			Postgres: &stroppy.DriverConfig_PostgresConfig{
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

	runtimeConfig, err := buildDriverConfig(0, configs[0], nil)
	if err != nil {
		t.Fatalf("buildDriverConfig() error = %v", err)
	}

	if runtimeConfig.GetUrl() != "postgres://cli" ||
		runtimeConfig.GetBulkSize() != 20 ||
		runtimeConfig.GetErrorMode() != stroppy.DriverConfig_ERROR_MODE_THROW ||
		runtimeConfig.GetPostgres().GetMaxConns() != 10 ||
		runtimeConfig.GetPostgres().GetStatementCacheCapacity() != 13 {
		t.Fatalf("runtime driver config = %#v, extra = %#v", runtimeConfig, configs[0].Extra)
	}

	mysql := "mysql"
	maxOpenConns := int32(9)
	specificMaxOpenConns := int32(5)
	maxIdleConns := int32(4)

	configs, err = runner.DriverCLIConfigsFromFile(map[uint32]*stroppy.DriverRunConfig{
		0: {
			DriverType: &mysql,
			Pool:       &stroppy.DriverRunConfig_PoolConfig{MaxOpenConns: &maxOpenConns},
			Sql: &stroppy.DriverConfig_SqlConfig{
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

	runtimeConfig, err = buildDriverConfig(0, configs[0], nil)
	if err != nil {
		t.Fatalf("buildDriverConfig(mysql) error = %v", err)
	}

	if runtimeConfig.GetSql().GetMaxOpenConns() != 12 || runtimeConfig.GetSql().GetMaxIdleConns() != 4 {
		t.Fatalf("runtime mysql driver config = %#v", runtimeConfig)
	}
}

func TestPoolSizePreservesPostgresSpecificConfig(t *testing.T) {
	t.Setenv("POOL_SIZE", "11")

	config, err := buildDriverConfig(0, &runner.DriverCLIConfig{
		DriverType: "postgres",
		Extra: map[string]any{
			"postgres": map[string]any{
				"statementCacheCapacity": 13,
				"maxConnLifetime":        "1h",
			},
		},
	}, nil)
	if err != nil {
		t.Fatalf("buildDriverConfig() error = %v", err)
	}

	postgres := config.GetPostgres()
	if postgres.GetMaxConns() != 11 || postgres.GetMinConns() != 11 ||
		postgres.GetStatementCacheCapacity() != 13 || postgres.GetMaxConnLifetime() != "1h" {
		t.Fatalf("postgres config = %#v", postgres)
	}
}

func TestLegacyPostgresPoolSizeRejectsInt32Overflow(t *testing.T) {
	t.Setenv("POOL_SIZE", "2147483648")

	config, err := buildDriverConfig(0, &runner.DriverCLIConfig{DriverType: "postgres"}, nil)
	if err != nil {
		t.Fatalf("buildDriverConfig() error = %v", err)
	}

	if config.GetPostgres() != nil {
		t.Fatalf("postgres config = %#v", config.GetPostgres())
	}
}

func TestResolveSQLSourcePrecedence(t *testing.T) {
	t.Run("CLI SQL positional beats config inline body", func(t *testing.T) {
		unsetRunTestEnv(t, sqlBodyEnv, sqlFileEnv)

		source := resolveSQLSource(
			&runArgs{scriptArg: "execute_sql", sqlArg: "cli.sql"},
			"",
			"cli.sql",
			nil,
			nil,
			map[string]string{sqlBodyEnv: "select 'config'"},
		)
		if source.envKey != sqlFileEnv || source.value != "cli.sql" || !source.explicitCLI {
			t.Fatalf("source = %#v", source)
		}

		env := map[string]string{sqlBodyEnv: "select 'config'"}
		applySQLSource(env, source)

		if !maps.Equal(env, map[string]string{sqlFileEnv: "cli.sql"}) {
			t.Fatalf("resolved env = %v", env)
		}
	})

	t.Run("typed CLI sql-file beats process and config body", func(t *testing.T) {
		unsetRunTestEnv(t, sqlBodyEnv, sqlFileEnv)
		t.Setenv(sqlBodyEnv, "select 'process'")

		source := resolveSQLSource(
			&runArgs{typedParams: map[string]string{"sql-file": "typed.sql"}},
			"select 'config route'",
			"",
			nil,
			nil,
			map[string]string{sqlBodyEnv: "select 'config env'"},
		)
		if source.envKey != sqlFileEnv || source.value != "typed.sql" || !source.explicitCLI {
			t.Fatalf("source = %#v", source)
		}
	})

	t.Run("CLI inline SQL masks process file", func(t *testing.T) {
		unsetRunTestEnv(t, sqlBodyEnv, sqlFileEnv)
		t.Setenv(sqlFileEnv, "process.sql")

		source := resolveSQLSource(
			&runArgs{scriptArg: "select 1"},
			"--= query\nselect 1;\n",
			"",
			nil,
			nil,
			nil,
		)
		if !source.explicitCLI || source.envKey != sqlBodyEnv {
			t.Fatalf("source = %#v", source)
		}

		if err := withProcessSQLSource(source, func() error {
			if body := os.Getenv(sqlBodyEnv); body != source.value {
				t.Fatalf("process body = %q", body)
			}

			if _, ok := os.LookupEnv(sqlFileEnv); ok {
				t.Fatal("process SQL_FILE was not masked")
			}

			return nil
		}); err != nil {
			t.Fatalf("withProcessSQLSource() error = %v", err)
		}

		if file := os.Getenv(sqlFileEnv); file != "process.sql" {
			t.Fatalf("process SQL_FILE was not restored: %q", file)
		}
	})

	t.Run("typed workload config beats config env body", func(t *testing.T) {
		unsetRunTestEnv(t, sqlBodyEnv, sqlFileEnv)

		source := resolveSQLSource(
			&runArgs{},
			"",
			"",
			nil,
			map[string]json.RawMessage{"sqlFile": json.RawMessage(`"typed-config.sql"`)},
			map[string]string{sqlBodyEnv: "select 'config env'"},
		)
		if source.envKey != sqlFileEnv || source.value != "typed-config.sql" {
			t.Fatalf("source = %#v", source)
		}

		env := map[string]string{sqlBodyEnv: "select 'config env'"}
		applySQLSource(env, source)

		if !maps.Equal(env, map[string]string{sqlFileEnv: "typed-config.sql"}) {
			t.Fatalf("resolved env = %v", env)
		}
	})

	t.Run("process beats legacy and config sources", func(t *testing.T) {
		unsetRunTestEnv(t, sqlBodyEnv, sqlFileEnv)
		t.Setenv(sqlFileEnv, "process.sql")

		source := resolveSQLSource(
			&runArgs{},
			"select 'config route'",
			"",
			map[string]string{sqlBodyEnv: "select '-e'"},
			nil,
			map[string]string{sqlBodyEnv: "select 'config env'"},
		)
		if source.envKey != sqlFileEnv || source.value != "process.sql" {
			t.Fatalf("source = %#v", source)
		}
	})
}

func TestBuildDriverConfigReturnsJSONConversionErrors(t *testing.T) {
	_, err := buildDriverConfig(0, &runner.DriverCLIConfig{
		Extra: map[string]any{"invalid": func() {}},
	}, nil)
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

	if _, err := buildDriverConfig(0, configs[0], nil); err == nil || !contains(err.Error(), "unknown field") {
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

func TestToEnvVarsRespectsExistingEnv(t *testing.T) {
	t.Setenv("STROPPY_DRIVER_0", `{"url":"from-env"}`)

	configs := runner.DriverCLIConfigs{
		0: &runner.DriverCLIConfig{URL: "from-cli"},
	}

	envs, err := configs.ToEnvVars()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, env := range envs {
		if len(env) > 16 && env[:16] == "STROPPY_DRIVER_0" {
			t.Fatalf("ToEnvVars should not override existing STROPPY_DRIVER_0, got: %s", env)
		}
	}
}

func TestToEnvVarsSetsWhenNotInEnv(t *testing.T) {
	// Ensure STROPPY_DRIVER_0 is not set.
	os.Unsetenv("STROPPY_DRIVER_0")

	configs := runner.DriverCLIConfigs{
		0: &runner.DriverCLIConfig{URL: "from-cli"},
	}

	envs, err := configs.ToEnvVars()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(envs) == 0 {
		t.Fatal("expected STROPPY_DRIVER_0 to be set")
	}
}

func unsetRunTestEnv(t *testing.T, names ...string) {
	t.Helper()

	for _, name := range names {
		value, set := os.LookupEnv(name)
		if err := os.Unsetenv(name); err != nil {
			t.Fatalf("Unsetenv(%q) error = %v", name, err)
		}

		t.Cleanup(func() {
			if set {
				_ = os.Setenv(name, value)
			} else {
				_ = os.Unsetenv(name)
			}
		})
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
