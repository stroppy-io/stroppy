package dsdgen

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// oracleBin returns the path to the reference C dsdgen binary, or skips the test
// if it is not available. Set DSDGEN_BIN to the compiled binary and DSDGEN_DIR
// to the tools dir (which must contain tpcds.idx) to enable these tests.
func oracleBin(t *testing.T) (bin, toolsDir string) {
	t.Helper()
	bin = os.Getenv("DSDGEN_BIN")
	if bin == "" {
		t.Skip("DSDGEN_BIN not set; skipping byte-equality test against reference dsdgen")
	}
	toolsDir = os.Getenv("DSDGEN_DIR")
	if toolsDir == "" {
		toolsDir = filepath.Dir(bin)
	}

	return bin, toolsDir
}

// runOracle generates one table with the reference binary and returns the raw
// .dat bytes. dsdgen requires running from the tools dir so it finds tpcds.idx.
func runOracle(t *testing.T, bin, toolsDir, table string, scale int) []byte {
	t.Helper()
	out := t.TempDir()
	cmd := exec.Command(bin,
		"-table", table,
		"-scale", fmt.Sprint(scale),
		"-force", "Y",
		"-terminate", "N",
		"-dir", out,
	)
	cmd.Dir = toolsDir
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("dsdgen failed: %v\n%s", err, b)
	}

	data, err := os.ReadFile(filepath.Join(out, table+".dat"))
	if err != nil {
		t.Fatalf("read oracle output: %v", err)
	}

	return data
}

// formatTable renders the whole table with the Go port using the dsdgen field
// layout: "|"-separated fields, one row per line, no trailing separator.
func formatTable(tbl *Table, sf float64) []byte {
	var b strings.Builder
	s := tbl.NewStream(sf, 1, -1)
	for {
		row, ok := s.Next()
		if !ok {
			break
		}
		for i, v := range row {
			if i > 0 {
				b.WriteByte('|')
			}
			if v != nil { // a nil column is a SQL null: dsdgen prints an empty field
				fmt.Fprintf(&b, "%v", v)
			}
		}
		b.WriteByte('\n')
	}

	return []byte(b.String())
}

// assertPartitionByteEqual generates ONLY the row range [start, start+count) via
// an independent Stream (as a parallel worker would) and checks it matches the
// corresponding lines of the full reference output. This guards the core
// parallel/streamable guarantee: a partition started mid-table must be
// byte-identical regardless of any earlier rows, with no shared state.
func assertPartitionByteEqual(t *testing.T, tbl *Table, scale int, start, count int64) {
	t.Helper()
	bin, toolsDir := oracleBin(t)
	want := bytes.Split(runOracle(t, bin, toolsDir, tbl.Name, scale), []byte{'\n'})
	// runOracle output ends with a trailing newline -> drop the empty last field.
	if len(want) > 0 && len(want[len(want)-1]) == 0 {
		want = want[:len(want)-1]
	}
	hi := start - 1 + count
	if hi > int64(len(want)) {
		hi = int64(len(want))
	}
	wantChunk := append(bytes.Join(want[start-1:hi], []byte{'\n'}), '\n')

	var got bytes.Buffer
	s := tbl.NewStream(float64(scale), start, count)
	for {
		row, ok := s.Next()
		if !ok {
			break
		}
		for i, v := range row {
			if i > 0 {
				got.WriteByte('|')
			}
			if v != nil {
				fmt.Fprintf(&got, "%v", v)
			}
		}
		got.WriteByte('\n')
	}

	if !bytes.Equal(got.Bytes(), wantChunk) {
		t.Errorf("%s partition [%d,%d) at sf=%d differs from dsdgen\n--- got ---\n%s\n--- want ---\n%s",
			tbl.Name, start, start+count, scale, firstLines(got.Bytes(), 4), firstLines(wantChunk, 4))
	}
}

// assertTableByteEqual is the per-table byte-equality harness: it generates tbl
// with the Go port at each scale and compares it byte-for-byte to the reference
// dsdgen output. Each ported table gets a small _test.go calling this.
func assertTableByteEqual(t *testing.T, tbl *Table, scales ...int) {
	t.Helper()
	bin, toolsDir := oracleBin(t)
	for _, scale := range scales {
		scale := scale
		t.Run(fmt.Sprintf("sf%d", scale), func(t *testing.T) {
			want := runOracle(t, bin, toolsDir, tbl.Name, scale)
			got := formatTable(tbl, float64(scale))
			if string(got) != string(want) {
				t.Errorf("%s output differs from dsdgen at sf=%d\n--- got ---\n%s\n--- want ---\n%s",
					tbl.Name, scale, firstLines(got, 5), firstLines(want, 5))
			}
		})
	}
}

func firstLines(b []byte, n int) string {
	lines := strings.SplitN(string(b), "\n", n+1)
	if len(lines) > n {
		lines = lines[:n]
	}

	return strings.Join(lines, "\n")
}
