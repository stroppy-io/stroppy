// Package examples provides embedded example TypeScript scripts and SQL files.
package examples

import (
	"embed"
	"errors"
	"fmt"
	"os"
	"path"
)

// ErrUnknownPreset is returned when an unknown preset name is requested.
var ErrUnknownPreset = errors.New("unknown preset")

// Preset represents an available example preset.
type Preset string

const (
	PresetSimple     Preset = "simple"
	PresetTPCC       Preset = "tpcc"
	PresetTPCB       Preset = "tpcb"
	PresetExecuteSQL Preset = "execute_sql"
	PresetTPCDS      Preset = "tpcds"
)

// PresetFiles maps preset names to their required files.
var PresetFiles = map[Preset][]string{ //nolint: gochecknoglobals
	PresetSimple: {
		"simple.ts",
	},
	PresetTPCC: {
		"tpcc.ts",
	},
	PresetTPCB: {
		"tpcb.ts",
		"tpcb.sql",
		"tpcb_mini.sql",
		"tpcb_freeform.sql",
	},
	PresetExecuteSQL: {
		"execute_sql.ts",
		"tpcb_mini.sql",
		"tpcb.sql",
	},
	PresetTPCDS: {
		"tpcds.ts",
		"tpcds-scale-1.sql",
		"tpcds-scale-10.sql",
		"tpcds-scale-100.sql",
		"tpcds-scale-300.sql",
		"tpcds-scale-1000.sql",
		"tpcds-scale-3000.sql",
		"tpcds-scale-10000.sql",
		"tpcds-scale-30000.sql",
		"tpcds-scale-50000.sql",
		"tpcds-scale-100000.sql",
	},
}

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

//go:embed *.ts *.sql
var Content embed.FS

// CopyPresetToPath copies preset files to the target directory.
func CopyPresetToPath(targetPath string, preset Preset, perm os.FileMode) error {
	files, ok := PresetFiles[preset]
	if !ok {
		return fmt.Errorf("%w: %s", ErrUnknownPreset, preset)
	}

	for _, fileName := range files {
		data, err := Content.ReadFile(fileName)
		if err != nil {
			return fmt.Errorf("failed to read preset file %s: %w", fileName, err)
		}

		err = os.WriteFile(path.Join(targetPath, fileName), data, perm)
		if err != nil {
			return fmt.Errorf("failed to write preset file %s: %w", fileName, err)
		}
	}

	return nil
}

// CopyFileToPath copies a single file from examples to the target directory.
func CopyFileToPath(targetPath, fileName string, perm os.FileMode) error {
	data, err := Content.ReadFile(fileName)
	if err != nil {
		return fmt.Errorf("failed to read file %s: %w", fileName, err)
	}

	err = os.WriteFile(path.Join(targetPath, fileName), data, perm)
	if err != nil {
		return fmt.Errorf("failed to write file %s: %w", fileName, err)
	}

	return nil
}
