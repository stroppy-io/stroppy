package probe

import (
	"bytes"
	"encoding/json"
	"slices"
	"strings"
	"testing"

	_ "github.com/stroppy-io/stroppy/internal/workloads"
	"github.com/stroppy-io/stroppy/pkg/bench"
)

func TestJSONCatalogIncludesWorkloadSchemas(t *testing.T) {
	first := renderJSONCatalog(t)
	second := renderJSONCatalog(t)

	if !bytes.Equal(first, second) {
		t.Fatal("JSON output is not stable")
	}

	catalog := decodeCatalog(t, first)
	assertSortedWorkloads(t, catalog.Workloads)
	assertRepresentativeParams(t, catalog.Workloads)
}

func TestHumanCatalogIncludesGroupedWorkloads(t *testing.T) {
	var output bytes.Buffer
	if err := printCatalog(&output, humanFormat); err != nil {
		t.Fatalf("printCatalog() error = %v", err)
	}

	text := output.String()
	for _, want := range []string{
		"PRESETS (embedded workloads)",
		"WORKLOADS (typed parameters)",
		"  tpcc/tx\n",
		"    run:      --duration, --executor, --iterations, --query-timeout, --vus",
		"    workload: --load-items",
		"stroppy run <workload> --help",
		"DRIVERS (supported insert methods)",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("human output does not contain %q\n%s", want, text)
		}
	}
}

func renderJSONCatalog(t *testing.T) []byte {
	t.Helper()

	var output bytes.Buffer
	if err := printCatalog(&output, jsonFormat); err != nil {
		t.Fatalf("printCatalog() error = %v", err)
	}

	return output.Bytes()
}

func decodeCatalog(t *testing.T, data []byte) catalogOutput {
	t.Helper()

	var top map[string]json.RawMessage
	if err := json.Unmarshal(data, &top); err != nil {
		t.Fatalf("decode top-level JSON: %v", err)
	}

	for _, key := range []string{"presets", "drivers", "workloads"} {
		if _, ok := top[key]; !ok {
			t.Errorf("top-level key %q is missing", key)
		}
	}

	var catalog catalogOutput
	if err := json.Unmarshal(data, &catalog); err != nil {
		t.Fatalf("decode catalog JSON: %v", err)
	}

	return catalog
}

func assertSortedWorkloads(t *testing.T, workloads []workloadEntry) {
	t.Helper()

	names := make([]string, 0, len(workloads))
	for _, workload := range workloads {
		names = append(names, workload.Name)
	}

	if !slices.IsSorted(names) {
		t.Fatalf("workloads are not sorted: %v", names)
	}
}

func assertRepresentativeParams(t *testing.T, workloads []workloadEntry) {
	t.Helper()

	tpcc := findWorkload(t, workloads, "tpcc/tx")
	scaleFactor := findParam(t, tpcc.Params, "scale-factor")

	if scaleFactor.Flag != "--scale-factor" ||
		scaleFactor.Scope != bench.ParamScopeWorkload ||
		scaleFactor.Type != bench.ParamTypeInt ||
		scaleFactor.Description != "Number of warehouses." ||
		scaleFactor.Default != float64(1) ||
		scaleFactor.Env != "SCALE_FACTOR" ||
		!slices.Equal(scaleFactor.LegacyAliases, []string{"WAREHOUSES"}) ||
		scaleFactor.Config != "scaleFactor" {
		t.Fatalf("tpcc scale-factor schema = %#v", scaleFactor)
	}

	executor := findParam(t, tpcc.Params, "executor")
	if executor.Scope != bench.ParamScopeRun || executor.Flag != "--executor" || executor.Config != "executor" {
		t.Fatalf("executor schema = %#v", executor)
	}

	duration := findParam(t, tpcc.Params, "duration")
	if duration.Type != bench.ParamTypeDuration || duration.Default != "0s" {
		t.Fatalf("duration schema = %#v", duration)
	}

	loadItems := findParam(t, tpcc.Params, "load-items")
	if loadItems.Default != nil ||
		loadItems.DefaultDescription != "true when warehouse-start is 1; false otherwise" {
		t.Fatalf("load-items schema = %#v", loadItems)
	}
}

func findWorkload(t *testing.T, workloads []workloadEntry, name string) workloadEntry {
	t.Helper()

	for _, workload := range workloads {
		if workload.Name == name {
			return workload
		}
	}

	t.Fatalf("workload %q not found", name)

	return workloadEntry{}
}

func findParam(t *testing.T, params []paramEntry, name string) paramEntry {
	t.Helper()

	for idx := range params {
		if params[idx].Name == name {
			return params[idx]
		}
	}

	t.Fatalf("parameter %q not found", name)

	return paramEntry{}
}
