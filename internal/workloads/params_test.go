package workloads

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/stroppy-io/stroppy/pkg/bench"
)

func TestBuiltInWorkloadParameterSchemas(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		want []bench.ParamSchema
	}{
		{"execute_sql", []bench.ParamSchema{
			param("sql-file", bench.ParamTypeString, "", "SQL_FILE", "sqlFile"),
		}},
		{"simple", []bench.ParamSchema{}},
		{"tpcb/tx", []bench.ParamSchema{
			param("load-workers", bench.ParamTypeInt, 1, "LOAD_WORKERS", "loadWorkers"),
			param("retry-attempts", bench.ParamTypeInt, 3, "RETRY_ATTEMPTS", "retryAttempts"),
			param("scale-factor", bench.ParamTypeInt, 1, "SCALE_FACTOR", "scaleFactor"),
			param("sql-file", bench.ParamTypeString, "", "SQL_FILE", "sqlFile"),
			param("tx-isolation", bench.ParamTypeString, "", "TX_ISOLATION", "txIsolation"),
		}},
		{"tpcc/procs", tpccParamSchema()},
		{"tpcc/tx", tpccParamSchema()},
		{"tpcds", []bench.ParamSchema{
			param("load-workers", bench.ParamTypeInt, 0, "LOAD_WORKERS", "loadWorkers"),
			param("pg-unlogged", bench.ParamTypeBool, false, "PG_UNLOGGED", "pgUnlogged"),
			param("query-seed", bench.ParamTypeInt, 19620718, "QUERY_SEED", "querySeed"),
			param("query-stream", bench.ParamTypeInt, 0, "QUERY_STREAM", "queryStream"),
			param("scale-factor", bench.ParamTypeFloat64, float64(1), "SCALE_FACTOR", "scaleFactor"),
			param("schema-file", bench.ParamTypeString, "", "SCHEMA_FILE", "schemaFile"),
			param("sql-file", bench.ParamTypeString, "", "SQL_FILE", "sqlFile"),
			param("streams", bench.ParamTypeInt, 1, "STREAMS", "streams"),
			param("validate-force", bench.ParamTypeBool, false, "VALIDATE_FORCE", "validateForce"),
			param("ydb-store-mode", bench.ParamTypeString, "column", "YDB_STORE_MODE", "ydbStoreMode"),
		}},
		{"tpch/tx", []bench.ParamSchema{
			param("load-workers", bench.ParamTypeInt, 0, "LOAD_WORKERS", "loadWorkers"),
			param("pg-unlogged", bench.ParamTypeBool, false, "PG_UNLOGGED", "pgUnlogged"),
			param("scale-factor", bench.ParamTypeFloat64, float64(1), "SCALE_FACTOR", "scaleFactor"),
			param("sql-file", bench.ParamTypeString, "", "SQL_FILE", "sqlFile"),
			param("ydb-store-mode", bench.ParamTypeString, "column", "YDB_STORE_MODE", "ydbStoreMode"),
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			description, err := bench.Describe(tt.name)
			if err != nil {
				t.Fatal(err)
			}

			got := workloadParams(description.Params)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("parameter schema mismatch\n got: %#v\nwant: %#v", got, tt.want)
			}
		})
	}
}

func TestBuiltInWorkloadsDoNotReadPublicEnvDirectly(t *testing.T) {
	t.Parallel()

	fset := token.NewFileSet()

	err := filepath.WalkDir(".", func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		if entry.IsDir() || strings.HasSuffix(path, "_test.go") || filepath.Ext(path) != ".go" {
			return nil
		}

		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return err
		}

		ast.Inspect(file, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}

			selector, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || len(call.Args) == 0 || !isEnvRead(selector.Sel.Name) {
				return true
			}

			pkg, ok := selector.X.(*ast.Ident)
			if !ok || pkg.Name != "bench" {
				return true
			}

			literal, ok := call.Args[0].(*ast.BasicLit)
			if !ok {
				return true
			}

			name, err := strconv.Unquote(literal.Value)
			if err == nil && name != "STROPPY_SQL_BODY" {
				t.Errorf("%s reads public workload knob %s through bench.%s", path, name, selector.Sel.Name)
			}

			return true
		})

		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func isEnvRead(name string) bool {
	return name == "Env" || name == "EnvInt" || name == "EnvFloat"
}

func tpccParamSchema() []bench.ParamSchema {
	return []bench.ParamSchema{
		derivedParam(
			"load-items",
			bench.ParamTypeBool,
			"LOAD_ITEMS",
			"loadItems",
			"true when warehouse-start is 1; false otherwise",
		),
		param("load-workers", bench.ParamTypeInt, 1, "LOAD_WORKERS", "loadWorkers"),
		param("pacing", bench.ParamTypeBool, false, "PACING", "pacing"),
		param("pg-unlogged", bench.ParamTypeBool, false, "PG_UNLOGGED", "pgUnlogged"),
		param("retry-attempts", bench.ParamTypeInt, 3, "RETRY_ATTEMPTS", "retryAttempts"),
		param("scale-factor", bench.ParamTypeInt, 1, "SCALE_FACTOR", "scaleFactor", "WAREHOUSES"),
		param("sql-file", bench.ParamTypeString, "", "SQL_FILE", "sqlFile"),
		param("tx-isolation", bench.ParamTypeString, "", "TX_ISOLATION", "txIsolation"),
		param("warehouse-start", bench.ParamTypeInt, 1, "WAREHOUSE_START", "warehouseStart"),
	}
}

func param(
	name string,
	typ bench.ParamType,
	defaultValue any,
	env, config string,
	aliases ...string,
) bench.ParamSchema {
	return bench.ParamSchema{
		Name: name, Flag: "--" + name, Scope: bench.ParamScopeWorkload,
		Type: typ, Default: defaultValue, Env: env,
		LegacyEnvAliases: aliases, Config: config,
	}
}

func derivedParam(
	name string,
	typ bench.ParamType,
	env, config, defaultDescription string,
) bench.ParamSchema {
	schema := param(name, typ, nil, env, config)
	schema.DefaultDescription = defaultDescription

	return schema
}

func workloadParams(params []bench.ParamSchema) []bench.ParamSchema {
	workload := make([]bench.ParamSchema, 0, len(params))
	for _, schema := range params {
		if schema.Scope != bench.ParamScopeWorkload {
			continue
		}

		schema.Description = ""
		workload = append(workload, schema)
	}

	return workload
}
