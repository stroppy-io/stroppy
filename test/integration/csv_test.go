//go:build integration

package integration

import (
	"bytes"
	"context"
	"crypto/sha256"
	stdcsv "encoding/csv"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

// TestCsvDriverTpcbSF001 drives workloads/tpcb/tx.ts end-to-end with
// the CSV driver at SF=1, writes every row to CSV shards, merges them
// at teardown, and reads the resulting files back to assert:
//   - one MANIFEST.json alongside the merged CSVs
//   - expected per-table row counts (SF=1 TPC-B: 1/10/100000)
//   - header row present as first line of every .csv
//
// The binary path and stroppy repo root resolution match the tpcb /
// tpcc workload tests.
func TestCsvDriverTpcbSF001(t *testing.T) {
	if os.Getenv(envSkip) == "1" {
		t.Skipf("skipping integration test: %s=1", envSkip)
	}

	repoRoot := findRepoRoot(t)

	binary := filepath.Join(repoRoot, "build", "stroppy")
	if _, err := os.Stat(binary); err != nil {
		t.Fatalf("stroppy binary not found at %s (run `make build` first): %v", binary, err)
	}

	outDir := t.TempDir()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	runCsvTpcb(t, ctx, repoRoot, binary, outDir, "tpcb", "true", "1")

	workloadDir := filepath.Join(outDir, "tpcb")

	// MANIFEST should land alongside the merged CSVs.
	if _, err := os.Stat(filepath.Join(workloadDir, "MANIFEST.json")); err != nil {
		t.Fatalf("MANIFEST.json missing: %v", err)
	}

	expected := map[string]int64{
		"pgbench_branches": 1,
		"pgbench_tellers":  10,
		"pgbench_accounts": 100_000,
	}

	for table, want := range expected {
		path := filepath.Join(workloadDir, table+".csv")

		got, header := csvRowCount(t, path)
		if got != want {
			t.Errorf("%s rows = %d, want %d", table, got, want)
		}

		if header == "" {
			t.Errorf("%s missing header row", table)
		}
	}

	// merge=true must clean up the shards dir.
	if _, err := os.Stat(filepath.Join(workloadDir, ".shards")); !os.IsNotExist(err) {
		t.Errorf(".shards dir still exists post-merge: %v", err)
	}
}

// TestCsvDriverGoldenTpcbSF1 pins the byte-for-byte content of the
// CSV driver's output against committed SHA-256 hashes. A failure
// means either (a) seed derivation changed, (b) CSV encoding changed,
// or (c) tpcb spec changed. Any of these is load-bearing; the fix is
// to validate manually and update testdata/csv/tpcb_sf1/*.sha256.
func TestCsvDriverGoldenTpcbSF1(t *testing.T) {
	if os.Getenv(envSkip) == "1" {
		t.Skipf("skipping integration test: %s=1", envSkip)
	}

	repoRoot := findRepoRoot(t)

	binary := filepath.Join(repoRoot, "build", "stroppy")
	if _, err := os.Stat(binary); err != nil {
		t.Fatalf("stroppy binary not found: %v", err)
	}

	outDir := t.TempDir()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	runCsvTpcb(t, ctx, repoRoot, binary, outDir, "tpcb_sf1", "true", "1")

	workloadDir := filepath.Join(outDir, "tpcb_sf1")
	goldenDir := filepath.Join(repoRoot, "testdata", "csv", "tpcb_sf1")

	for _, table := range []string{
		"pgbench_branches",
		"pgbench_tellers",
		"pgbench_accounts",
	} {
		got := sha256OfFile(t, filepath.Join(workloadDir, table+".csv"))
		want := readGolden(t, filepath.Join(goldenDir, table+".csv.sha256"))

		if got != want {
			t.Errorf("%s SHA mismatch\n  got  %s\n  want %s", table, got, want)
		}
	}
}

// TestCsvDriverDeterminismAcrossWorkers runs the tpcb workload at
// LOAD_WORKERS ∈ {1, 4, 16} with ?merge=true, sorts every emitted
// table's lines, and asserts all three workers produce identical
// sorted multisets. This is the CLAUDE.md §Parallelism §1 contract.
func TestCsvDriverDeterminismAcrossWorkers(t *testing.T) {
	if os.Getenv(envSkip) == "1" {
		t.Skipf("skipping integration test: %s=1", envSkip)
	}

	repoRoot := findRepoRoot(t)

	binary := filepath.Join(repoRoot, "build", "stroppy")
	if _, err := os.Stat(binary); err != nil {
		t.Fatalf("stroppy binary not found: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	hashes := make(map[string][3]string)

	for i, workers := range []string{"1", "4", "16"} {
		outDir := t.TempDir()

		runCsvTpcb(t, ctx, repoRoot, binary, outDir, "tpcb", "true", workers)

		workloadDir := filepath.Join(outDir, "tpcb")

		for _, table := range []string{"pgbench_branches", "pgbench_tellers", "pgbench_accounts"} {
			h := sha256OfSortedBody(t, filepath.Join(workloadDir, table+".csv"))

			snap := hashes[table]
			snap[i] = h
			hashes[table] = snap
		}
	}

	for table, tri := range hashes {
		if !(tri[0] == tri[1] && tri[1] == tri[2]) {
			t.Errorf("%s: non-deterministic across workers {1,4,16}: %v", table, tri)
		}
	}
}

// runCsvTpcb invokes `./build/stroppy run` against the tpcb workload
// with the CSV driver, the given URL-encoded merge flag, and the
// chosen LOAD_WORKERS count. Output goes to outDir/<workload>/.
func runCsvTpcb(
	t *testing.T,
	ctx context.Context,
	repoRoot, binary, outDir, workload, merge, workers string,
) {
	t.Helper()

	url := fmt.Sprintf("%s?merge=%s&workload=%s", outDir, merge, workload)

	cmd := exec.CommandContext(ctx, binary,
		"run", "./workloads/tpcb/tx.ts",
		"-D", "url="+url,
		"-D", "driverType=csv",
		"-e", "SCALE_FACTOR=1",
		"-e", "LOAD_WORKERS="+workers,
		"--steps", "drop_schema,create_schema,load_data",
	)
	cmd.Dir = repoRoot

	var stdout, stderr bytes.Buffer

	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		t.Fatalf("stroppy run (csv, workers=%s) failed: %v\n--- stdout ---\n%s\n--- stderr ---\n%s",
			workers, err, stdout.String(), stderr.String())
	}
}

// csvRowCount returns (rowsExcludingHeader, headerString) for the
// given CSV file. Fails the test if the file does not exist or is
// empty.
func csvRowCount(t *testing.T, path string) (int64, string) {
	t.Helper()

	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open %q: %v", path, err)
	}

	defer f.Close()

	r := stdcsv.NewReader(f)
	r.FieldsPerRecord = -1

	all, err := r.ReadAll()
	if err != nil {
		t.Fatalf("read %q: %v", path, err)
	}

	if len(all) == 0 {
		return 0, ""
	}

	return int64(len(all) - 1), strings.Join(all[0], ",")
}

// sha256OfFile returns the SHA-256 hex digest of the file at path.
// Used by the golden-hash test to compare against committed digests.
func sha256OfFile(t *testing.T, path string) string {
	t.Helper()

	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %q: %v", path, err)
	}

	sum := sha256.Sum256(b)

	return hex.EncodeToString(sum[:])
}

// readGolden reads a single-line hex SHA-256 from path, trimmed of
// surrounding whitespace. Committed hashes are one-per-file so the
// lineage to `sha256sum` output stays obvious.
func readGolden(t *testing.T, path string) string {
	t.Helper()

	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden %q: %v", path, err)
	}

	return strings.TrimSpace(string(b))
}

// sha256OfSortedBody returns the SHA-256 of the file's rows, excluding
// the header, after sorting them lexicographically. Two runs of the
// same workload with different worker counts must match on this hash.
func sha256OfSortedBody(t *testing.T, path string) string {
	t.Helper()

	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open %q: %v", path, err)
	}

	defer f.Close()

	r := stdcsv.NewReader(f)
	r.FieldsPerRecord = -1

	all, err := r.ReadAll()
	if err != nil {
		t.Fatalf("read %q: %v", path, err)
	}

	if len(all) < 1 {
		return ""
	}

	body := make([]string, 0, len(all)-1)
	for _, rec := range all[1:] {
		body = append(body, strings.Join(rec, ""))
	}

	sort.Strings(body)

	h := sha256.New()

	for _, line := range body {
		_, _ = h.Write([]byte(line))
		_, _ = h.Write([]byte{'\n'})
	}

	return hex.EncodeToString(h.Sum(nil))
}
