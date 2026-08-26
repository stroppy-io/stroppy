package workloads_test

import (
	"testing"

	"github.com/stroppy-io/stroppy/workloads"
	_ "github.com/stroppy-io/stroppy/workloads/all"
)

func TestCatalog(t *testing.T) {
	catalog, err := workloads.Catalog()
	if err != nil {
		t.Fatalf("Catalog() error: %v", err)
	}

	expected := []workloads.Preset{
		workloads.PresetTPCB,
		workloads.PresetTPCC,
		workloads.PresetTPCDS,
		workloads.PresetTPCH,
	}
	if len(catalog) != len(expected) {
		t.Fatalf("got %d presets, want %d", len(catalog), len(expected))
	}

	byName := make(map[string]workloads.PresetInfo, len(catalog))
	for _, preset := range catalog {
		byName[preset.Name] = preset
	}

	for _, preset := range expected {
		if _, ok := byName[string(preset)]; !ok {
			t.Errorf("missing preset %q", preset)
		}
	}

	tpcc := byName[string(workloads.PresetTPCC)]
	if len(tpcc.SQL) == 0 {
		t.Error("tpcc: expected SQL dialects, got none")
	}

	if len(tpcc.Docs) == 0 {
		t.Error("tpcc: expected docs, got none")
	}
}
