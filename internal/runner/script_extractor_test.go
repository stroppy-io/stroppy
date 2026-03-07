package runner

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/evanw/esbuild/pkg/api"
	"github.com/stretchr/testify/require"

	"github.com/stroppy-io/stroppy/internal/common"
	"github.com/stroppy-io/stroppy/internal/static"
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
	require.NoError(t, vm.Set("Step", stepSpy(vm, &steps)))
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
	accessedProps := []SQLSection{}

	require.NoError(t, vm.Set("parse_sql_with_groups", parseSectionsSpy(&accessedProps)))

	_, err := vm.RunString(`
const groups = parse_sql_with_groups("", null);
groups("some group directly");
const group_name = "my dynamic name"
groups(group_name);
`)
	require.NoError(t, err)
	require.Equal(
		t,
		[]SQLSection{
			{Name: "some group directly"},
			{Name: "my dynamic name"},
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
	// These tests used the old defineConfig/NewDriverByConfigBin API.
	// With the new DriverX.create().setup() pattern, driver config
	// is no longer extracted at probe time via GlobalConfig.
}

func TestExtractConfigFromJS_BinaryConfig(t *testing.T) {
	t.SkipNow()
}

func TestExtractConfigFromJS_NoConfig(t *testing.T) {
	t.SkipNow()
}

func TestExtractConfigFromJS_InvalidConfig(t *testing.T) {
	t.SkipNow()
}

func TestExtractConfigFromJS_WithOpenMock(t *testing.T) {
	t.SkipNow()
}

func TestExtractConfigFromScript_ExecuteSQL(t *testing.T) {
	t.SkipNow()
}
