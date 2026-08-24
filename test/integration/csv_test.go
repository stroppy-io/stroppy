//go:build integration

package integration

import (
	"crypto/sha256"
	stdcsv "encoding/csv"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"
)

type csvManifest struct {
	Workload     string                      `json:"workload"`
	InsertMethod string                      `json:"insert_method"`
	Config       csvManifestConfig           `json:"config"`
	Tables       map[string]csvManifestTable `json:"tables"`
}

type csvManifestConfig struct {
	Dir       string `json:"dir"`
	Separator string `json:"separator"`
	Header    bool   `json:"header"`
	Merge     bool   `json:"merge"`
	NullValue string `json:"null_value"`
}

type csvManifestTable struct {
	Rows    int64    `json:"rows"`
	Shards  int      `json:"shards"`
	Columns []string `json:"columns"`
}

var tpcbCSVColumns = map[string][]string{
	"pgbench_branches": {"bid", "bbalance", "filler"},
	"pgbench_tellers":  {"tid", "bid", "tbalance", "filler"},
	"pgbench_accounts": {"aid", "bid", "abalance", "filler"},
}

var tpcbCSVRows = map[string]int64{
	"pgbench_branches": 1,
	"pgbench_tellers":  10,
	"pgbench_accounts": 100_000,
}

// TestCSVDriverMergedOutput validates the merged files, row counts, manifest,
// and removal of temporary shards after driver teardown.
func TestCSVDriverMergedOutput(t *testing.T) {
	outDir := t.TempDir()
	runCSVTPCB(t, outDir, "tpcb", true, 4)

	workloadDir := filepath.Join(outDir, "tpcb")
	assertDirectoryEntries(t, workloadDir, []string{
		"MANIFEST.json",
		"pgbench_accounts.csv",
		"pgbench_branches.csv",
		"pgbench_tellers.csv",
	})

	manifest := readCSVManifest(t, workloadDir)
	assertCSVManifest(t, manifest, outDir, "tpcb", true, map[string]int{
		"pgbench_branches": 1,
		"pgbench_tellers":  4,
		"pgbench_accounts": 4,
	})

	for table, wantRows := range tpcbCSVRows {
		records := readCSVRecords(t, filepath.Join(workloadDir, table+".csv"))
		if len(records) == 0 {
			t.Fatalf("%s.csv is empty", table)
		}
		if !slices.Equal(records[0], tpcbCSVColumns[table]) {
			t.Errorf("%s header = %v, want %v", table, records[0], tpcbCSVColumns[table])
		}
		if got := int64(len(records) - 1); got != wantRows {
			t.Errorf("%s rows = %d, want %d", table, got, wantRows)
		}
	}

	if _, err := os.Stat(filepath.Join(workloadDir, ".shards")); !os.IsNotExist(err) {
		t.Errorf("merge=true left .shards behind: %v", err)
	}
}

// TestCSVDriverUnmergedShards validates shard names, header sidecars, content,
// row totals, and the unmerged manifest layout.
func TestCSVDriverUnmergedShards(t *testing.T) {
	outDir := t.TempDir()
	runCSVTPCB(t, outDir, "tpcb_sharded", false, 3)

	workloadDir := filepath.Join(outDir, "tpcb_sharded")
	assertDirectoryEntries(t, workloadDir, []string{
		"MANIFEST.json",
		"pgbench_accounts.header.csv",
		"pgbench_accounts.w000.csv",
		"pgbench_accounts.w001.csv",
		"pgbench_accounts.w002.csv",
		"pgbench_branches.header.csv",
		"pgbench_branches.w000.csv",
		"pgbench_tellers.header.csv",
		"pgbench_tellers.w000.csv",
		"pgbench_tellers.w001.csv",
		"pgbench_tellers.w002.csv",
	})

	manifest := readCSVManifest(t, workloadDir)
	assertCSVManifest(t, manifest, outDir, "tpcb_sharded", false, map[string]int{
		"pgbench_branches": 1,
		"pgbench_tellers":  3,
		"pgbench_accounts": 3,
	})

	for table, wantRows := range tpcbCSVRows {
		header := readCSVRecords(t, filepath.Join(workloadDir, table+".header.csv"))
		if len(header) != 1 || !slices.Equal(header[0], tpcbCSVColumns[table]) {
			t.Errorf("%s header sidecar = %v, want one %v row", table, header, tpcbCSVColumns[table])
		}

		shards, err := filepath.Glob(filepath.Join(workloadDir, table+".w*.csv"))
		if err != nil {
			t.Fatalf("glob %s shards: %v", table, err)
		}
		if len(shards) != manifest.Tables[table].Shards {
			t.Errorf("%s shard files = %d, manifest = %d", table, len(shards), manifest.Tables[table].Shards)
		}

		var rows int64
		for _, shard := range shards {
			records := readCSVRecords(t, shard)
			if len(records) == 0 {
				t.Errorf("shard %s is empty", filepath.Base(shard))
				continue
			}
			if slices.Equal(records[0], tpcbCSVColumns[table]) {
				t.Errorf("shard %s unexpectedly contains a header", filepath.Base(shard))
			}
			for rowIndex, record := range records {
				if len(record) != len(tpcbCSVColumns[table]) {
					t.Errorf("%s row %d has %d fields, want %d", filepath.Base(shard), rowIndex, len(record), len(tpcbCSVColumns[table]))
				}
			}
			rows += int64(len(records))
		}
		if rows != wantRows {
			t.Errorf("%s shard rows = %d, want %d", table, rows, wantRows)
		}
		if _, err := os.Stat(filepath.Join(workloadDir, table+".csv")); !os.IsNotExist(err) {
			t.Errorf("merge=false produced merged %s.csv: %v", table, err)
		}
	}
}

// TestCSVDriverGoldenTPCB pins the byte-for-byte merged Go workload output.
func TestCSVDriverGoldenTPCB(t *testing.T) {
	repoRoot := findRepoRoot(t)
	outDir := t.TempDir()
	runCSVTPCB(t, outDir, "tpcb_sf1", true, 1)

	workloadDir := filepath.Join(outDir, "tpcb_sf1")
	goldenDir := filepath.Join(repoRoot, "testdata", "csv", "tpcb_sf1")
	for table := range tpcbCSVRows {
		got := sha256OfFile(t, filepath.Join(workloadDir, table+".csv"))
		want := readGolden(t, filepath.Join(goldenDir, table+".csv.sha256"))
		if got != want {
			t.Errorf("%s SHA mismatch\n  got  %s\n  want %s", table, got, want)
		}
	}
}

// TestCSVDriverDeterminismAcrossWorkers checks worker-count invariance.
func TestCSVDriverDeterminismAcrossWorkers(t *testing.T) {
	hashes := make(map[string][3]string)
	for i, workers := range []int{1, 4, 16} {
		outDir := t.TempDir()
		runCSVTPCB(t, outDir, "tpcb", true, workers)

		for table := range tpcbCSVRows {
			hash := sha256OfSortedBody(t, filepath.Join(outDir, "tpcb", table+".csv"))
			tableHashes := hashes[table]
			tableHashes[i] = hash
			hashes[table] = tableHashes
		}
	}

	for table, tableHashes := range hashes {
		if tableHashes[0] != tableHashes[1] || tableHashes[1] != tableHashes[2] {
			t.Errorf("%s is non-deterministic across workers {1,4,16}: %v", table, tableHashes)
		}
	}
}

func runCSVTPCB(t *testing.T, outDir, workload string, merge bool, workers int) {
	t.Helper()

	url := fmt.Sprintf("%s?merge=%t&workload=%s", outDir, merge, workload)
	runStroppy(t, 2*time.Minute,
		"run", "tpcb/tx",
		"-D", "driverType=csv",
		"-D", "url="+url,
		"--scale-factor", "1",
		"--load-workers", fmt.Sprint(workers),
		"--executor", "shared-iterations",
		"--iterations", "1",
		"--steps", "drop_schema,create_schema,load_data",
	)
}

func readCSVManifest(t *testing.T, workloadDir string) csvManifest {
	t.Helper()

	data, err := os.ReadFile(filepath.Join(workloadDir, "MANIFEST.json"))
	if err != nil {
		t.Fatalf("read MANIFEST.json: %v", err)
	}

	var manifest csvManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatalf("decode MANIFEST.json: %v", err)
	}

	return manifest
}

func assertCSVManifest(
	t *testing.T,
	manifest csvManifest,
	outDir, workload string,
	merge bool,
	shards map[string]int,
) {
	t.Helper()

	if manifest.Workload != workload {
		t.Errorf("manifest workload = %q, want %q", manifest.Workload, workload)
	}
	if manifest.InsertMethod != "NATIVE" {
		t.Errorf("manifest insert_method = %q, want NATIVE", manifest.InsertMethod)
	}
	if manifest.Config.Dir != outDir || manifest.Config.Separator != "," ||
		!manifest.Config.Header || manifest.Config.Merge != merge || manifest.Config.NullValue != "" {
		t.Errorf("manifest config = %+v, want dir=%q separator=comma header=true merge=%t", manifest.Config, outDir, merge)
	}
	if len(manifest.Tables) != len(tpcbCSVRows) {
		t.Errorf("manifest tables = %d, want %d", len(manifest.Tables), len(tpcbCSVRows))
	}

	for table, wantRows := range tpcbCSVRows {
		got, ok := manifest.Tables[table]
		if !ok {
			t.Errorf("manifest missing table %s", table)
			continue
		}
		if got.Rows != wantRows || got.Shards != shards[table] || !slices.Equal(got.Columns, tpcbCSVColumns[table]) {
			t.Errorf("manifest %s = %+v, want rows=%d shards=%d columns=%v", table, got, wantRows, shards[table], tpcbCSVColumns[table])
		}
	}
}

func assertDirectoryEntries(t *testing.T, dir string, want []string) {
	t.Helper()

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read directory %q: %v", dir, err)
	}

	got := make([]string, len(entries))
	for i, entry := range entries {
		got[i] = entry.Name()
	}
	sort.Strings(got)
	sort.Strings(want)
	if !slices.Equal(got, want) {
		t.Errorf("directory %s entries = %v, want %v", dir, got, want)
	}
}

func readCSVRecords(t *testing.T, path string) [][]string {
	t.Helper()

	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("open %q: %v", path, err)
	}
	defer file.Close()

	reader := stdcsv.NewReader(file)
	reader.FieldsPerRecord = -1
	records, err := reader.ReadAll()
	if err != nil {
		t.Fatalf("read %q: %v", path, err)
	}

	return records
}

func sha256OfFile(t *testing.T, path string) string {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %q: %v", path, err)
	}

	sum := sha256.Sum256(data)

	return hex.EncodeToString(sum[:])
}

func readGolden(t *testing.T, path string) string {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden %q: %v", path, err)
	}

	return strings.TrimSpace(string(data))
}

func sha256OfSortedBody(t *testing.T, path string) string {
	t.Helper()

	records := readCSVRecords(t, path)
	if len(records) == 0 {
		return ""
	}

	body := make([]string, 0, len(records)-1)
	for _, record := range records[1:] {
		body = append(body, strings.Join(record, "\x1f"))
	}
	sort.Strings(body)

	hash := sha256.New()
	for _, line := range body {
		_, _ = hash.Write([]byte(line))
		_, _ = hash.Write([]byte{'\n'})
	}

	return hex.EncodeToString(hash.Sum(nil))
}
