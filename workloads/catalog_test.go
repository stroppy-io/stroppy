package workloads

import "testing"

func TestCatalog(t *testing.T) {
	catalog, err := Catalog()
	if err != nil {
		t.Fatalf("Catalog() error: %v", err)
	}

	if len(catalog) != len(AvailablePresets()) {
		t.Fatalf("got %d presets, want %d", len(catalog), len(AvailablePresets()))
	}

	byName := make(map[string]PresetInfo, len(catalog))
	for _, p := range catalog {
		byName[p.Name] = p
	}

	if len(byName["tpcc"].SQL) == 0 {
		t.Error("tpcc: expected SQL dialects, got none")
	}

	if len(byName["tpcc"].Docs) == 0 {
		t.Error("tpcc: expected docs, got none")
	}
}
