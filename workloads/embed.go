// Package workloads provides workload-owned embedded SQL, JSON, and documentation files.
package workloads

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
)

// Preset identifies an embedded workload asset set.
type Preset string

const (
	PresetTPCC  Preset = "tpcc"
	PresetTPCB  Preset = "tpcb"
	PresetTPCDS Preset = "tpcds"
	PresetTPCH  Preset = "tpch"
)

// ErrUnknownPreset is returned when an unknown preset name is requested.
var ErrUnknownPreset = errors.New("unknown preset")

var registry = make(map[Preset]fs.FS)

// Register associates a preset with assets embedded during package initialization.
func Register(preset Preset, files fs.FS) {
	if files == nil {
		panic(fmt.Sprintf("workloads: register %q with nil filesystem", preset))
	}

	if _, exists := registry[preset]; exists {
		panic(fmt.Sprintf("workloads: preset %q already registered", preset))
	}

	registry[preset] = files
}

// AvailablePresets returns registered preset names in sorted order.
func AvailablePresets() []string {
	presets := make([]string, 0, len(registry))
	for preset := range registry {
		presets = append(presets, string(preset))
	}

	sort.Strings(presets)

	return presets
}

// CopyPresetToPath copies preset files to target directory.
func CopyPresetToPath(targetPath string, preset Preset, perm os.FileMode) error {
	files, err := presetFiles(preset)
	if err != nil {
		return err
	}

	entries, err := fs.ReadDir(files, ".")
	if err != nil {
		return fmt.Errorf("read preset %q: %w", preset, err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		if err := copyFileToPath(files, targetPath, entry.Name(), perm); err != nil {
			return fmt.Errorf("preset %q file copy: %w", preset, err)
		}
	}

	return nil
}

// ReadPresetFile reads one file embedded by a workload package.
func ReadPresetFile(presetName, fileName string) ([]byte, error) {
	files, err := presetFiles(Preset(presetName))
	if err != nil {
		return nil, err
	}

	return fs.ReadFile(files, fileName)
}

func presetFiles(preset Preset) (fs.FS, error) {
	files, exists := registry[preset]
	if !exists {
		return nil, fmt.Errorf("%w: %s", ErrUnknownPreset, preset)
	}

	return files, nil
}

func copyFileToPath(files fs.FS, targetPath, fileName string, perm os.FileMode) error {
	source, err := files.Open(fileName)
	if err != nil {
		return fmt.Errorf("open %s: %w", fileName, err)
	}
	defer source.Close()

	destination, err := os.OpenFile(
		filepath.Join(targetPath, fileName),
		os.O_WRONLY|os.O_CREATE|os.O_TRUNC,
		perm,
	)
	if err != nil {
		return fmt.Errorf("open %s: %w", fileName, err)
	}
	defer destination.Close()

	if _, err := io.Copy(destination, source); err != nil {
		return fmt.Errorf("copy %s to %s: %w", fileName, targetPath, err)
	}

	return nil
}
