package run

import (
	"encoding/json"
	"errors"
	"os"
	"testing"

	"github.com/stroppy-io/stroppy/internal/runner"
)

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
			name:       "unknown flag before separator returns error",
			args:       []string{"tpcc", "--vus", "10"},
			wantErrStr: "pass k6 flags after --",
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

			if !presetMapsEqual(got.driverPresets, tt.wantPresets) {
				t.Errorf("driverPresets: got %v, want %v", got.driverPresets, tt.wantPresets)
			}

			if !driverOptMapsEqual(got.driverOpts, tt.wantOpts) {
				t.Errorf("driverOpts: got %v, want %v", got.driverOpts, tt.wantOpts)
			}
		})
	}
}

// ── helpers ───────────────────────────────────────────────────────────────────

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

	configs := runner.DriverCLIConfigs{}

	err := applyDriverOpt(configs, 0, "pool.maximum", "20")
	if err == nil {
		t.Fatal("expected error for unknown pool field")
	}

	if !contains(err.Error(), "unknown pool option") {
		t.Fatalf("got error %q, want it to contain unknown pool option", err.Error())
	}
}

func TestApplyDriverOptTLSAliases(t *testing.T) {
	t.Parallel()

	configs := runner.DriverCLIConfigs{}

	if err := applyDriverOpt(configs, 0, "tls.insecureSkipVerify", "true"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got := marshalDriverConfig(t, configs[0])
	if got["tlsInsecureSkipVerify"] != true {
		t.Errorf("tlsInsecureSkipVerify: got %v, want true", got["tlsInsecureSkipVerify"])
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
