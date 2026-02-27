package runner

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/evanw/esbuild/pkg/api"
	"github.com/grafana/sobek"
	"github.com/stretchr/testify/require"

	"github.com/stroppy-io/stroppy/internal/common"
	"github.com/stroppy-io/stroppy/internal/static"
	stroppy "github.com/stroppy-io/stroppy/pkg/common/proto/stroppy"
	"github.com/stroppy-io/stroppy/workloads"
)

func Test_spyProxyObject(t *testing.T) {
	vm := createVM()
	accessedProps := []string{}
	proxy := spyProxyObject(vm, vm.NewObject(), &accessedProps)

	require.NoError(t, vm.Set("__ENV", proxy))

	_, err := vm.RunString(`
__ENV.some;
__ENV.other;
__ENV.__some_secret || "secret";
`)
	require.NoError(t, err)
	require.Equal(t, []string{"some", "other", "__some_secret"}, accessedProps)
}

func Test_stepSpy(t *testing.T) {
	vm := createVM()
	steps := []string{}
	require.NoError(t, vm.Set("Step", stepSpy(&steps)))
	v, err := vm.RunString(`
Step("other step", undefined);
Step("my great step", ()=>{ return "wow" });
`)
	require.NoError(t, err)
	require.Equal(t, "wow", v.ToString().String())
	require.Equal(t, []string{"other step", "my great step"}, steps)
}

func Test_parseGroupsSpy(t *testing.T) {
	vm := createVM()
	accessedProps := []string{}

	require.NoError(t, vm.Set("parse_sql_with_groups", parseGroupsSpy(vm, &accessedProps)))

	_, err := vm.RunString(`
const groups = parse_sql_with_groups("", null);
groups["some group directly"];
const group_name = "my dynamic name"
groups[group_name];
groups.my_simple_name_as_property;
`)
	require.NoError(t, err)
	require.Equal(
		t,
		[]string{
			"some group directly",
			"my dynamic name",
			"my_simple_name_as_property",
		},
		accessedProps,
	)
}

// bundleScriptForTest bundles a TypeScript script with all dependencies from internal/static.
// It creates a temp directory, copies static files, and uses esbuild to bundle everything.
func bundleScriptForTest(t *testing.T, scriptPath string) string {
	t.Helper()

	// Create temp directory
	tempDir, err := os.MkdirTemp("", "stroppy-test-")
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = os.RemoveAll(tempDir)
	})

	// Copy static files to temp directory
	allFiles := append(static.StaticFiles, static.DevStaticFiles...)
	err = static.CopyStaticFilesToPath(tempDir, common.FileMode, allFiles...)
	require.NoError(t, err)

	// Copy the script to temp directory
	scriptName := filepath.Base(scriptPath)
	scriptData, err := os.ReadFile(scriptPath)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(tempDir, scriptName), scriptData, common.FileMode)
	require.NoError(t, err)

	// Copy SQL files if they exist in the script's directory
	scriptDir := filepath.Dir(scriptPath)

	sqlFiles, err := filepath.Glob(filepath.Join(scriptDir, "*.sql"))
	if err == nil {
		for _, sqlFile := range sqlFiles {
			sqlName := filepath.Base(sqlFile)

			sqlData, err := os.ReadFile(sqlFile)
			if err == nil {
				_ = os.WriteFile(filepath.Join(tempDir, sqlName), sqlData, common.FileMode)
			}
		}
	}

	// Use esbuild to bundle the script
	entryAbs := filepath.Join(tempDir, scriptName)
	result := api.Build(api.BuildOptions{
		EntryPoints:       []string{entryAbs},
		Bundle:            true,
		Platform:          api.PlatformNode,
		Format:            api.FormatESModule,
		Target:            api.ES2017,
		Sourcemap:         api.SourceMapInline,
		Write:             false,
		LogLevel:          api.LogLevelWarning,
		AbsWorkingDir:     tempDir,
		External:          []string{"k6/x/*", "k6/*"},
		MainFields:        []string{"module", "main"},
		ResolveExtensions: []string{".ts", ".tsx", ".js", ".mjs", ".json"},
		Loader: map[string]api.Loader{
			".ts":   api.LoaderTS,
			".tsx":  api.LoaderTSX,
			".js":   api.LoaderJS,
			".mjs":  api.LoaderJS,
			".json": api.LoaderJSON,
		},
	})

	require.Empty(t, result.Errors, "esbuild should not have errors")
	require.NotEmpty(t, result.OutputFiles, "esbuild should produce output")

	// Mock k6/x/encoding import
	jsCode := string(result.OutputFiles[0].Contents)
	re := regexp.MustCompile(`import\s+(\w+)\s+from\s+["']k6/x/encoding["'];?`)
	jsCode = re.ReplaceAllString(
		jsCode,
		`const $1 = { TextEncoder: globalThis.TextEncoder, TextDecoder: globalThis.TextDecoder };`,
	)

	return jsCode
}

func TestExtractConfigFromJS_SimpleConfig(t *testing.T) {
	t.SkipNow()

	jsCode := `const config = {
	driver: {
		url: "postgres://localhost:5432",
		driverType: 1
	}
};
defineConfig(config);`

	config, err := ProbeJSTest(sobek.New(), jsCode)
	require.NoError(t, err)
	require.NotNil(t, config)
	require.NotNil(t, config.GlobalConfig)
	require.NotNil(t, config.GlobalConfig.Driver)
	require.Equal(t, "postgres://localhost:5432", config.GlobalConfig.Driver.Url)
	require.Equal(
		t,
		stroppy.DriverConfig_DRIVER_TYPE_POSTGRES,
		config.GlobalConfig.Driver.DriverType,
	)
}

func TestExtractConfigFromJS_BinaryConfig(t *testing.T) {
	t.SkipNow()
	// Test with a config object (binary protobuf handling will be tested
	// in the comprehensive test with real execute_sql.ts which uses toBinary())
	jsCode := `
		const config = {
			driver: {
				url: "postgres://test:5432",
				driverType: 1
			}
		};
		defineConfig(config);
	`

	config, err := ProbeJSTest(sobek.New(), jsCode)
	require.NoError(t, err)
	require.NotNil(t, config)
	require.NotNil(t, config.GlobalConfig)
	require.Equal(t, "postgres://test:5432", config.GlobalConfig.Driver.Url)
}

func TestExtractConfigFromJS_NoConfig(t *testing.T) {
	t.SkipNow()

	jsCode := `
		// Script that doesn't call defineConfig
		const x = 42;
	`

	config, err := ProbeJSTest(sobek.New(), jsCode)
	require.Error(t, err)
	require.Nil(t, config)
	require.Equal(t, ErrNoConfigProvided, err)
}

func TestExtractConfigFromJS_InvalidConfig(t *testing.T) {
	t.SkipNow()

	jsCode := `
		// Script with invalid config
		defineConfig({ invalid: "config" });
	`

	// This should still work but the config might be empty or partially filled
	config, err := ProbeJSTest(sobek.New(), jsCode)
	// The extractor might succeed but with empty config, or it might fail
	// Let's check what actually happens
	if err != nil {
		require.Equal(t, ErrNoConfigProvided, err)
	} else {
		require.NotNil(t, config)
	}
}

func TestExtractConfigFromJS_WithOpenMock(t *testing.T) {
	t.SkipNow()

	jsCode := `
		if (typeof open !== "undefined") {
			const content = open("test.sql");
			// Use content somehow
		}
		const config = {
			driver: {
				url: "postgres://localhost:5432",
				driverType: 1
			}
		};
		defineConfig(config);
	`

	_ = func(filename string) string {
		if filename == "test.sql" {
			return "CREATE TABLE test (id INTEGER);"
		}

		return ""
	}

	config, err := ProbeJSTest(sobek.New(), jsCode)

	require.NoError(t, err)
	require.NotNil(t, config)
	require.NotNil(t, config.GlobalConfig)
}

func TestExtractConfigFromScript_ExecuteSQL(t *testing.T) {
	t.SkipNow()
	// Get the path to execute_sql.ts
	// We need to find it in the examples directory
	examplesDir := "examples"
	scriptPath := filepath.Join(examplesDir, "execute_sql.ts")

	// Check if file exists, if not try to read from embedded FS
	if _, err := os.Stat(scriptPath); os.IsNotExist(err) {
		// Try to read from embedded examples
		scriptData, err := workloads.Content.ReadFile("execute_sql/execute_sql.ts")
		require.NoError(t, err)

		// Create temp file
		tempDir, err := os.MkdirTemp("", "stroppy-test-")
		require.NoError(t, err)
		t.Cleanup(func() {
			_ = os.RemoveAll(tempDir)
		})

		scriptPath = filepath.Join(tempDir, "execute_sql.ts")
		err = os.WriteFile(scriptPath, scriptData, common.FileMode)
		require.NoError(t, err)

		// Also copy SQL file
		sqlData, err := workloads.Content.ReadFile("execute_sql/tpcb_mini.sql")
		if err == nil {
			sqlPath := filepath.Join(tempDir, "tpcb_mini.sql")
			_ = os.WriteFile(sqlPath, sqlData, common.FileMode)
		}
	}

	// Bundle the script with all dependencies
	bundledJS := bundleScriptForTest(t, scriptPath)

	_ = bundledJS

	// Create open mock that returns SQL content
	sqlContent, err := workloads.Content.ReadFile("execute_sql/tpcb_mini.sql")
	require.NoError(t, err)

	openMock := func(filename string) string {
		if filename == "tpcb_mini.sql" {
			return string(sqlContent)
		}

		return ""
	}
	_ = openMock

	// Extract config from bundled code
	config, err := ProbeJSTest(sobek.New(), bundledJS)
	require.NoError(t, err, "should extract config from execute_sql.ts")
	require.NotNil(t, config)
	require.NotNil(t, config.GlobalConfig)
	require.NotNil(t, config.GlobalConfig.Driver)
	require.Equal(
		t,
		stroppy.DriverConfig_DRIVER_TYPE_POSTGRES,
		config.GlobalConfig.Driver.DriverType,
	)
	require.NotEmpty(t, config.GlobalConfig.Driver.Url)
}
