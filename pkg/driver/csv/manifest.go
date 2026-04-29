package csv

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	stroppy "github.com/stroppy-io/stroppy/pkg/common/proto/stroppy"
)

// manifest is the shape of <outdir>/<workload>/MANIFEST.json. It
// captures enough metadata for a downstream consumer to verify which
// stroppy build produced the files, how they are encoded, and
// whether two runs are directly comparable.
type manifest struct {
	Workload     string                   `json:"workload"`
	Generated    string                   `json:"generated"`
	FrameworkVer string                   `json:"framework_version"`
	InsertMethod string                   `json:"insert_method"`
	Config       manifestConfig           `json:"config"`
	Tables       map[string]manifestTable `json:"tables"`
}

// manifestConfig mirrors the effective driver configuration so the
// MANIFEST is a faithful snapshot of the URL that produced the files.
type manifestConfig struct {
	Dir       string `json:"dir"`
	Separator string `json:"separator"`
	Header    bool   `json:"header"`
	Merge     bool   `json:"merge"`
	NullValue string `json:"null_value"`
}

// manifestTable records per-table row count, shard count, and
// canonical column order. Column order is the stable output order
// used by every shard and the merged CSV's header row.
type manifestTable struct {
	Rows    int64    `json:"rows"`
	Shards  int      `json:"shards"`
	Columns []string `json:"columns"`
}

// writeManifest emits MANIFEST.json into the workload output
// directory. Called at Teardown after the optional merge pass so the
// file enumerates the final artifacts rather than pre-merge shards.
func writeManifest(
	workloadDir, workloadName string,
	cfg config,
	tables map[string]*tableState,
) error {
	if err := os.MkdirAll(workloadDir, dirMode); err != nil {
		return fmt.Errorf("mkdir %q: %w", workloadDir, err)
	}

	doc := manifest{
		Workload:     workloadName,
		Generated:    time.Now().UTC().Format(time.RFC3339),
		FrameworkVer: stroppy.Version,
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

	for name, ts := range tables {
		doc.Tables[name] = manifestTable{
			Rows:    ts.rowCount,
			Shards:  ts.shards,
			Columns: append([]string(nil), ts.columns...),
		}
	}

	blob, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal manifest: %w", err)
	}

	path := filepath.Join(workloadDir, "MANIFEST.json")

	if err := os.WriteFile(path, blob, fileMode); err != nil { //nolint:gosec // manifest is plain metadata, never secret
		return fmt.Errorf("write %q: %w", path, err)
	}

	return nil
}
