// Package workloads provides embedded TypeScript scripts and SQL files workloads.
package workloads

import (
	"embed"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
)

// Preset represents an available example preset.
type Preset string

const (
	PresetSimple     Preset = "simple"
	PresetTPCC       Preset = "tpcc"
	PresetTPCB       Preset = "tpcb"
	PresetTPCDS      Preset = "tpcds"
	PresetExecuteSQL Preset = "execute_sql"
)

// ErrUnknownPreset is returned when an unknown preset name is requested.
var ErrUnknownPreset = errors.New("unknown preset")

//go:embed *
var Content embed.FS

// AvailablePresets returns list of available preset names.
func AvailablePresets() []string {
	return []string{
		string(PresetSimple),
		string(PresetTPCC),
		string(PresetTPCB),
		string(PresetExecuteSQL),
		string(PresetTPCDS),
	}
}

// CopyPresetToPath copies preset files to the target directory.
func CopyPresetToPath(targetPath string, preset Preset, perm os.FileMode) error {
	entries, err := Content.ReadDir(string(preset))
	if err != nil {
		return fmt.Errorf("%w: %s", ErrUnknownPreset, preset)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		err = copyFileToPath(targetPath, string(preset), entry.Name(), perm)
		if err != nil {
			return fmt.Errorf("preset '%s' file copy error: %w", preset, err)
		}
	}

	return nil
}

// ReadPresetFile reads a single file from an embedded preset.
// presetName is the preset directory (e.g., "tpcc"), fileName is the file within it (e.g., "tpcc.ts").
func ReadPresetFile(presetName, fileName string) ([]byte, error) {
	return Content.ReadFile(path.Join(presetName, fileName))
}

// copyFileToPath copies a single file from examples to the target directory.
func copyFileToPath(targetPath, preset, fileName string, perm os.FileMode) error {
	file, err := Content.Open(path.Join(preset, fileName))
	if err != nil {
		return fmt.Errorf("failed to open file %s: %w", fileName, err)
	}
	defer file.Close()

	destFile, err := os.OpenFile(
		path.Join(targetPath, fileName),
		os.O_WRONLY|os.O_CREATE|os.O_TRUNC,
		perm,
	)
	if err != nil {
		return fmt.Errorf("failed to open file %s: %w", fileName, err)
	}
	defer destFile.Close()

	_, err = io.Copy(destFile, file)
	if err != nil {
		return fmt.Errorf("failed to copy file %s to %s: %w", fileName, targetPath, err)
	}

	return nil
}
