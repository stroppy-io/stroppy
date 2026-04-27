package csv

import (
	"bufio"
	stdcsv "encoding/csv"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
)

// mergeAll concatenates every table's worker shards into one CSV per
// table, writing a single header row first. On success the per-table
// .shards/ directory is removed. Merge is sequential: the driver's
// contention budget during the run was spent on parallel writes; the
// merge pass is O(total bytes) and runs once at Teardown.
func (d *Driver) mergeAll(workloadDir string, tables map[string]*tableState) error {
	shardDir := filepath.Join(workloadDir, ".shards")

	if _, err := os.Stat(shardDir); err != nil {
		if os.IsNotExist(err) {
			return nil
		}

		return fmt.Errorf("csv: stat shards %q: %w", shardDir, err)
	}

	names := sortedTableNames(tables)

	for _, name := range names {
		ts := tables[name]
		if err := d.mergeTable(shardDir, workloadDir, name, ts); err != nil {
			return err
		}
	}

	if err := os.RemoveAll(shardDir); err != nil {
		return fmt.Errorf("csv: cleanup %q: %w", shardDir, err)
	}

	return nil
}

// mergeTable writes <workloadDir>/<table>.csv by concatenating every
// shard it can find on disk for that table. Shard paths are
// discovered by glob so even empty / partial runs merge correctly.
func (d *Driver) mergeTable(
	shardDir, workloadDir, table string,
	ts *tableState,
) error {
	pattern := filepath.Join(shardDir, table+".w*.csv")

	matches, err := filepath.Glob(pattern)
	if err != nil {
		return fmt.Errorf("csv: glob shards %q: %w", pattern, err)
	}

	sort.Strings(matches)

	outPath := filepath.Join(workloadDir, table+".csv")

	out, err := os.Create(outPath)
	if err != nil {
		return fmt.Errorf("csv: create merged %q: %w", outPath, err)
	}

	buf := bufio.NewWriterSize(out, csvBufferSize)

	if d.cfg.header {
		if err := writeHeader(buf, ts.columns, d.cfg.separator); err != nil {
			_ = out.Close()

			return fmt.Errorf("csv: header %q: %w", outPath, err)
		}
	}

	for _, shard := range matches {
		if err := appendFile(buf, shard); err != nil {
			_ = out.Close()

			return fmt.Errorf("csv: concat %q: %w", shard, err)
		}
	}

	if err := buf.Flush(); err != nil {
		_ = out.Close()

		return fmt.Errorf("csv: flush %q: %w", outPath, err)
	}

	if err := out.Close(); err != nil {
		return fmt.Errorf("csv: close %q: %w", outPath, err)
	}

	return nil
}

// emitHeaderSidecars writes a sidecar <table>.header.csv alongside
// each table's worker shards when merge=false. Downstream tools that
// want a header can prepend the sidecar; raw shards stay bare so
// consumers accepting globs do not need to strip duplicate headers.
func (d *Driver) emitHeaderSidecars(workloadDir string, tables map[string]*tableState) error {
	if !d.cfg.header {
		return nil
	}

	for _, name := range sortedTableNames(tables) {
		ts := tables[name]

		outPath := filepath.Join(workloadDir, name+".header.csv")

		out, err := os.Create(outPath)
		if err != nil {
			return fmt.Errorf("csv: header sidecar %q: %w", outPath, err)
		}

		buf := bufio.NewWriterSize(out, csvBufferSize)

		if err := writeHeader(buf, ts.columns, d.cfg.separator); err != nil {
			_ = out.Close()

			return fmt.Errorf("csv: header sidecar %q: %w", outPath, err)
		}

		if err := buf.Flush(); err != nil {
			_ = out.Close()

			return fmt.Errorf("csv: header sidecar flush %q: %w", outPath, err)
		}

		if err := out.Close(); err != nil {
			return fmt.Errorf("csv: header sidecar close %q: %w", outPath, err)
		}
	}

	return nil
}

// writeHeader emits the column-name row using encoding/csv so any
// separator/special characters in column identifiers get the correct
// RFC-4180 quoting.
func writeHeader(w io.Writer, columns []string, sep rune) error {
	cw := stdcsv.NewWriter(w)
	cw.Comma = sep

	if err := cw.Write(columns); err != nil {
		return err
	}

	cw.Flush()

	return cw.Error()
}

// appendFile streams src into dst. Neither side adds or strips a
// trailing newline: encoding/csv always terminates its last record
// with "\n", so concatenated shards join cleanly.
func appendFile(dst io.Writer, src string) error {
	f, err := os.Open(src)
	if err != nil {
		return err
	}

	defer f.Close()

	_, err = io.Copy(dst, f)

	return err
}

// sortedTableNames returns table names in deterministic order. Merge
// iteration order is not observable by callers, but sorted iteration
// keeps logs and error ordering stable across runs.
func sortedTableNames(tables map[string]*tableState) []string {
	names := make([]string, 0, len(tables))

	for name := range tables {
		names = append(names, name)
	}

	sort.Strings(names)

	return names
}
