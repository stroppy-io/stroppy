package csv

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/stroppy-io/stroppy/internal/version"
)

// manifest is the shape of <outdir>/<workload>/MANIFEST.json. It captures
// enough metadata for a downstream consumer to verify which stroppy build
// produced the files, how they are encoded, and whether runs are comparable.
type manifest struct {
	Workload     string                   `json:"workload"`
	Generated    string                   `json:"generated"`
	FrameworkVer string                   `json:"framework_version"`
	InsertMethod string                   `json:"insert_method"`
	Config       manifestConfig           `json:"config"`
	Tables       map[string]manifestTable `json:"tables"`
}

// manifestConfig mirrors the effective driver configuration.
type manifestConfig struct {
	Dir       string `json:"dir"`
	Separator string `json:"separator"`
	Header    bool   `json:"header"`
	Merge     bool   `json:"merge"`
	NullValue string `json:"null_value"`
}

// manifestTable records per-table row count, shard count, and column order.
type manifestTable struct {
	Rows    int64    `json:"rows"`
	Shards  int      `json:"shards"`
	Columns []string `json:"columns"`
}

const manifestFilename = "MANIFEST.json"

// invalidateManifest removes the completion marker before any output from a
// new generation can replace files from an earlier run.
func invalidateManifest(ctx context.Context, workloadDir string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	path := filepath.Join(workloadDir, manifestFilename)
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove prior manifest %q: %w", path, err)
	}

	return ctx.Err()
}

func manifestPublished(workloadDir string) (bool, error) {
	path := filepath.Join(workloadDir, manifestFilename)

	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}

	if err != nil {
		return false, fmt.Errorf("stat manifest %q: %w", path, err)
	}

	return info.Mode().IsRegular(), nil
}

// writeManifest publishes MANIFEST.json only after finalization succeeds.
func writeManifest(
	ctx context.Context,
	workloadDir, workloadName string,
	cfg config,
	tables map[string]*tableState,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	if err := os.MkdirAll(workloadDir, dirMode); err != nil {
		return fmt.Errorf("mkdir %q: %w", workloadDir, err)
	}

	doc := manifest{
		Workload:     workloadName,
		Generated:    time.Now().UTC().Format(time.RFC3339),
		FrameworkVer: version.Version,
		InsertMethod: "NATIVE",
		Config: manifestConfig{
			Dir:       cfg.dir,
			Separator: string(cfg.separator),
			Header:    cfg.header,
			Merge:     cfg.merge,
			NullValue: "",
		},
		Tables: make(map[string]manifestTable, len(tables)),
	}

	for name, state := range tables {
		if err := ctx.Err(); err != nil {
			return err
		}

		doc.Tables[name] = manifestTable{
			Rows:    state.rowCount,
			Shards:  state.shards,
			Columns: append([]string(nil), state.columns...),
		}
	}

	blob, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal manifest: %w", err)
	}

	if err := ctx.Err(); err != nil {
		return err
	}

	path := filepath.Join(workloadDir, manifestFilename)

	err = writeAtomic(ctx, path, func(out *os.File) error {
		_, writeErr := out.Write(blob)

		return writeErr
	})
	if err != nil {
		return fmt.Errorf("write %q: %w", path, err)
	}

	return nil
}
