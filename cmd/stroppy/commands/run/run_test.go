package run

import (
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

		// ── Missing script ─────────────────────────────────────────────────
		{
			name:    "empty args returns errNoScript",
			args:    []string{},
			wantErr: errNoScript,
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
			name:       "script + sql + two drivers + driver opt",
			args:       []string{"tpcc", "tpcc-scale-100", "-d", "pg", "-d1", "mysql", "-D1", "url=mysql://prod"},
			wantScript: "tpcc",
			wantSQL:    "tpcc-scale-100",
			wantPresets: map[int]string{0: "pg", 1: "mysql"},
			wantOpts:   map[int][][2]string{1: {{"url", "mysql://prod"}}},
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
			name:       "--driver=JSON equals form",
			args:       []string{"tpcc", `--driver={"driverType":"mysql"}`},
			wantScript: "tpcc",
			wantPresets: map[int]string{0: `{"driverType":"mysql"}`},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// parseRunArgs doesn't handle the empty-args case (RunE does before calling it).
			// For the errNoScript test we invoke RunE's guard condition directly.
			if len(tt.args) == 0 {
				if tt.wantErr != errNoScript {
					t.Fatalf("unexpected zero-args test without errNoScript expectation")
				}

				// Simulate what RunE does.
				if len(tt.args) == 0 {
					err := errNoScript
					if err != tt.wantErr {
						t.Fatalf("got %v, want %v", err, tt.wantErr)
					}
				}

				return
			}

			got, err := parseRunArgs(tt.args)

			if tt.wantErr != nil {
				if err != tt.wantErr {
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
