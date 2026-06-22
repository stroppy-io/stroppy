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

	// Entrypoints declare `export const options`; helpers do not.
	runnable := func(preset, script string) bool {
		for _, s := range byName[preset].Scripts {
			if s.Name == script {
				return s.Runnable
			}
		}

		t.Fatalf("script %s not found in preset %s", script, preset)

		return false
	}

	cases := []struct {
		preset, script string
		want           bool
	}{
		{"simple", "simple.ts", true},
		{"tpcc", "procs.ts", true},
		{"tpcc", "tx.ts", true},
		{"tpcc", "tpcc_helpers.ts", false},
		{"tpch", "tx.ts", true},
		{"tpch", "tpch_helpers.ts", false},
		{"tpch", "tpch_validate.ts", false},
	}

	for _, c := range cases {
		if got := runnable(c.preset, c.script); got != c.want {
			t.Errorf("%s/%s runnable = %v, want %v", c.preset, c.script, got, c.want)
		}
	}

	// tpcc ships SQL dialects and a README.
	if len(byName["tpcc"].SQL) == 0 {
		t.Error("tpcc: expected SQL dialects, got none")
	}

	if len(byName["tpcc"].Docs) == 0 {
		t.Error("tpcc: expected docs, got none")
	}
}
