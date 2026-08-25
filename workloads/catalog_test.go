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

	if len(catalog) != len(workloads.AvailablePresets()) {
		t.Fatalf("got %d presets, want %d", len(catalog), len(workloads.AvailablePresets()))
	}

	byName := make(map[string]workloads.PresetInfo, len(catalog))
	for _, preset := range catalog {
		byName[preset.Name] = preset
	}

	if len(byName["tpcc"].SQL) == 0 {
		t.Error("tpcc: expected SQL dialects, got none")
	}

	if len(byName["tpcc"].Docs) == 0 {
		t.Error("tpcc: expected docs, got none")
	}
}
