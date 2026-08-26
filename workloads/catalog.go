package workloads

import (
	"fmt"
	"io/fs"
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
		files, err := presetFiles(Preset(name))
		if err != nil {
			return nil, err
		}

		entries, err := fs.ReadDir(files, ".")
		if err != nil {
			return nil, fmt.Errorf("read preset %q: %w", name, err)
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
