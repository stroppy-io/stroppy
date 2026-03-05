package runner

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	js "github.com/grafana/sobek"
	"github.com/stretchr/testify/require"

	"github.com/stroppy-io/stroppy/internal/common"
)

// Sobek internal reference tests — understanding how the Go↔JS bridge behaves.

func Test_multipleReturns(t *testing.T) {
	vm := createVM()
	vm.Set("my_func", func(a, b int) (c int, d int, str string, err error) {
		if a < b {
			return a + b, a * b, "yes", nil
		} else {
			return a, b, "no", errors.New("a > b")
		}
	})
	val, err := vm.RunString("my_func(5, 10)")
	t.Logf("|%T|%T|", val, err)
	t.Logf("|%v|%v|", val, err)

	val, err = vm.RunString("my_func(10, 5)")
	t.Logf("|%T|%T|", val, err)
	t.Logf("|%v|%v|", val, err)
}

func Test_anyError(t *testing.T) {
	vm := createVM()
	vm.Set("my_func", func(a, b int) any {
		if a < b {
			return a + b
		} else {
			return errors.New("a > b")
		}
	})
	val, err := vm.RunString("my_func(5, 10)")
	t.Logf("|%T|%v|%T|", val, val.ExportType(), err)
	t.Logf("|%v|%v|", val, err)

	val, err = vm.RunString("my_func(10, 5)")
	t.Logf("|%T|%v|%T|", val, val.ExportType(), err)
	t.Logf("|%v|%v|", val, err)
}

func Test_defaultFunctionExport(t *testing.T) {
	vm := createVM()
	vm.RunString(` export default function () {} `)
	global := vm.GlobalObject()
	t.Log(global.GetOwnPropertyNames())

	fn := vm.Get("default")
	t.Log(fn)
	fnFunc, ok := js.AssertFunction(fn)
	t.Log(fnFunc, ok)
}

func Test_defaultFunctionTranspilation(t *testing.T) {
	vm := createVM()
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "test_file_name.ts")
	require.NoError(t, os.WriteFile(filePath, []byte(` export default function (): void {} `), common.FileMode))

	jsCode, err := TranspileTypeScript(filePath)
	require.NoError(t, err)
	t.Log(jsCode)

	_, err = vm.RunString(jsCode)
	require.NoError(t, err)

	global := vm.GlobalObject()
	t.Log(global.GetOwnPropertyNames())

	fn := vm.Get("test_file_name_default")
	t.Log(fn)
	fnFunc, ok := js.AssertFunction(fn)
	t.Log(fnFunc, ok)
}

// Test that driverStub exposes QueryResult fields (stats, rows) correctly
// through sobek's UncapFieldNameMapper.
func Test_driverStubRunQuery(t *testing.T) {
	vm := createVM()

	stub := &driverStub{}
	require.NoError(t, vm.Set("driver", stub))

	val, err := vm.RunString(`
		const result = driver.runQuery("SELECT 1", {});
		JSON.stringify({
			hasResult: result !== undefined && result !== null,
			hasRows:   result.rows !== undefined,
			hasStats:  result.stats !== undefined,
		});
	`)
	require.NoError(t, err)
	require.Equal(t, `{"hasResult":true,"hasRows":true,"hasStats":true}`, val.String())
}

// Test that rowsStub methods are callable from JS.
func Test_rowsStubMethods(t *testing.T) {
	vm := createVM()

	stub := &driverStub{}
	require.NoError(t, vm.Set("driver", stub))

	val, err := vm.RunString(`
		const r = driver.runQuery("SELECT 1", {});
		const rows = r.rows;
		const nextResult = rows.next();
		rows.close();
		const allRows = rows.readAll(0);
		JSON.stringify({
			next: nextResult,
			readAll: allRows,
		});
	`)
	require.NoError(t, err)
	require.JSONEq(t, `{"next":false,"readAll":[]}`, val.String())
}

// Test that driverStub.InsertValuesBin returns stats with elapsed field.
func Test_driverStubInsertValues(t *testing.T) {
	vm := createVM()

	stub := &driverStub{}
	require.NoError(t, vm.Set("driver", stub))

	val, err := vm.RunString(`
		const stats = driver.insertValuesBin(new Uint8Array(0), 0);
		stats.elapsed.milliseconds() >= 0;
	`)
	require.NoError(t, err)
	require.Equal(t, "true", val.String())
}
