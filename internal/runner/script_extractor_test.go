package runner

import (
	"os"
	"path/filepath"
	"testing"

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

func Test_spyProxyObjectReturnsExistingValues(t *testing.T) {
	vm := createVM()
	accessedProps := []string{}
	target := vm.NewObject()
	require.NoError(t, target.Set("STROPPY_DRIVER_0", `{"driverType":"ydb"}`))
	proxy := spyProxyObject(vm, target, &accessedProps)

	require.NoError(t, vm.Set("__ENV", proxy))

	v, err := vm.RunString(`__ENV.STROPPY_DRIVER_0`)
	require.NoError(t, err)
	require.JSONEq(t, `{"driverType":"ydb"}`, v.String())
	require.Equal(t, []string{"STROPPY_DRIVER_0"}, accessedProps)
}

func TestProbeScriptWithEnvAppliesDriverEnvToDeclaredSetup(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, static.CopyAllStaticFilesToPath(dir, common.FileMode))

	scriptPath := filepath.Join(dir, "driver_probe.ts")
	require.NoError(t, os.WriteFile(scriptPath, []byte(`
import { DriverX, declareDriverSetup } from "./helpers.ts";

export const options = {};

const driverConfig = declareDriverSetup(0, {
  url: "postgres://postgres:postgres@localhost:5432",
  driverType: "postgres",
  defaultInsertMethod: "plain_bulk",
  pool: { maxConns: 1, minConns: 1 },
});

DriverX.create().setup(driverConfig);

export default function() {}
`), common.FileMode))

	probe, err := ProbeScriptWithEnv(scriptPath, map[string]string{
		"STROPPY_DRIVER_0": `{
			"driverType": "ydb",
			"url": "grpc://localhost:2136/local",
			"defaultInsertMethod": "native",
			"defaultTxIsolation": "repeatable_read",
			"pool": { "maxOpenConns": 7, "maxIdleConns": 7 }
		}`,
	})
	require.NoError(t, err)
	require.Len(t, probe.DriverSetups, 1)

	setup := probe.DriverSetups[0].Defaults
	require.Equal(t, "ydb", setup["driverType"])
	require.Equal(t, "grpc://localhost:2136/local", setup["url"])
	require.Equal(t, "native", setup["defaultInsertMethod"])
	require.Equal(t, "repeatable_read", setup["defaultTxIsolation"])

	pool, ok := setup["pool"].(map[string]any)
	require.True(t, ok)
	require.EqualValues(t, 7, pool["maxOpenConns"])
	require.EqualValues(t, 7, pool["maxIdleConns"])
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
