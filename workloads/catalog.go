package workloads

import (
	"fmt"
	"sort"
	"strings"
)

// PresetInfo describes one embedded preset and its SQL dialect/variant stems
// and docs.
type PresetInfo struct {
	Name string   `json:"name"`          // e.g. "tpcc"
	SQL  []string `json:"sql,omitempty"` // dialect/variant stems, e.g. "pg"
	Docs []string `json:"docs,omitempty"`
}

// Catalog walks the embedded presets and classifies their files.
func Catalog() ([]PresetInfo, error) {
	presets := AvailablePresets()
	sort.Strings(presets)

	out := make([]PresetInfo, 0, len(presets))

	for _, name := range presets {
		entries, err := Content.ReadDir(name)
		if err != nil {
			return nil, fmt.Errorf("%w: %s", ErrUnknownPreset, name)
		}

		info := PresetInfo{Name: name}

		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}

			switch fileName := entry.Name(); {
			case strings.HasSuffix(fileName, ".sql"):
				info.SQL = append(info.SQL, strings.TrimSuffix(fileName, ".sql"))
			case strings.HasSuffix(fileName, ".md"):
				info.Docs = append(info.Docs, fileName)
			}
		}

		out = append(out, info)
	}

	return out, nil
}
