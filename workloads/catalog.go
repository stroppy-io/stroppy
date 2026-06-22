package workloads

import (
	"bytes"
	"fmt"
	"path"
	"sort"
	"strings"
)

// runnableMarker identifies a TypeScript file as a runnable k6 entrypoint.
// Helper modules export only named functions and lack this declaration (the
// probe VM panics on them in unwrapOptions), so this static check is both the
// fast and the reliable signal for "can this script be run?".
const runnableMarker = "export const options"

// ScriptInfo describes a single .ts file within a preset.
type ScriptInfo struct {
	Name     string `json:"name"`     // e.g. "procs.ts"
	Runnable bool   `json:"runnable"` // true if it is a k6 entrypoint
}

// PresetInfo describes one embedded preset and its files.
type PresetInfo struct {
	Name    string       `json:"name"` // e.g. "tpcc"
	Scripts []ScriptInfo `json:"scripts,omitempty"`
	SQL     []string     `json:"sql,omitempty"`  // dialect/variant stems, e.g. "pg"
	Docs    []string     `json:"docs,omitempty"` // e.g. "README.md"
}

// Catalog walks the embedded presets and classifies their files so callers can
// show users which presets exist and which scripts are runnable.
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
			case strings.HasSuffix(fileName, ".ts"):
				info.Scripts = append(info.Scripts, ScriptInfo{
					Name:     fileName,
					Runnable: isRunnable(name, fileName),
				})
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

// isRunnable reports whether a preset's .ts file is a k6 entrypoint.
func isRunnable(preset, fileName string) bool {
	content, err := Content.ReadFile(path.Join(preset, fileName))
	if err != nil {
		return false
	}

	return bytes.Contains(content, []byte(runnableMarker))
}
