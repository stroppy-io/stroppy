package csv

import (
	"bufio"
	"context"
	stdcsv "encoding/csv"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
)

// mergeAll concatenates every table's worker shards into one CSV per
// table, writing a single header row first. Shards are removed only after
// every table has been published successfully.
func (d *Driver) mergeAll(
	ctx context.Context,
	workloadDir string,
	tables map[string]*tableState,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	shardDir := filepath.Join(workloadDir, ".shards")

	if _, err := os.Stat(shardDir); err != nil {
		if os.IsNotExist(err) {
			return nil
		}

		return fmt.Errorf("csv: stat shards %q: %w", shardDir, err)
	}

	for _, name := range sortedTableNames(tables) {
		if err := ctx.Err(); err != nil {
			return err
		}

		if err := d.mergeTable(ctx, shardDir, workloadDir, name, tables[name]); err != nil {
			return err
		}
	}

	if err := ctx.Err(); err != nil {
		return err
	}

	if err := os.RemoveAll(shardDir); err != nil {
		return fmt.Errorf("csv: cleanup %q: %w", shardDir, err)
	}

	return nil
}

// mergeTable publishes <workloadDir>/<table>.csv atomically after every
// discovered shard has been copied. Cancellation removes only the temporary
// output, leaving the source shards available for a later teardown attempt.
func (d *Driver) mergeTable(
	ctx context.Context,
	shardDir, workloadDir, table string,
	ts *tableState,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	pattern := filepath.Join(shardDir, table+".w*.csv")

	matches, err := filepath.Glob(pattern)
	if err != nil {
		return fmt.Errorf("csv: glob shards %q: %w", pattern, err)
	}

	sort.Strings(matches)

	outPath := filepath.Join(workloadDir, table+".csv")

	err = writeAtomic(ctx, outPath, func(out *os.File) error {
		buf := bufio.NewWriterSize(out, csvBufferSize)

		if d.cfg.header {
			if err := writeHeader(buf, ts.columns, d.cfg.separator); err != nil {
				return fmt.Errorf("header: %w", err)
			}
		}

		for _, shard := range matches {
			if err := ctx.Err(); err != nil {
				return err
			}

			if err := appendFile(ctx, buf, shard); err != nil {
				return fmt.Errorf("concat %q: %w", shard, err)
			}
		}

		if err := ctx.Err(); err != nil {
			return err
		}

		if err := buf.Flush(); err != nil {
			return fmt.Errorf("flush: %w", err)
		}

		return ctx.Err()
	})
	if err != nil {
		return fmt.Errorf("csv: merge %q: %w", outPath, err)
	}

	return nil
}

// emitHeaderSidecars writes a sidecar <table>.header.csv alongside each
// table's worker shards when merge=false. Each sidecar is published atomically.
func (d *Driver) emitHeaderSidecars(
	ctx context.Context,
	workloadDir string,
	tables map[string]*tableState,
) error {
	if !d.cfg.header {
		return ctx.Err()
	}

	for _, name := range sortedTableNames(tables) {
		if err := ctx.Err(); err != nil {
			return err
		}

		ts := tables[name]
		outPath := filepath.Join(workloadDir, name+".header.csv")

		err := writeAtomic(ctx, outPath, func(out *os.File) error {
			buf := bufio.NewWriterSize(out, csvBufferSize)
			if err := writeHeader(buf, ts.columns, d.cfg.separator); err != nil {
				return err
			}

			if err := ctx.Err(); err != nil {
				return err
			}

			if err := buf.Flush(); err != nil {
				return err
			}

			return ctx.Err()
		})
		if err != nil {
			return fmt.Errorf("csv: header sidecar %q: %w", outPath, err)
		}
	}

	return nil
}

// writeHeader emits the column-name row using encoding/csv so any separator
// or special characters in column identifiers receive RFC-4180 quoting.
func writeHeader(w io.Writer, columns []string, sep rune) error {
	cw := stdcsv.NewWriter(w)
	cw.Comma = sep

	if err := cw.Write(columns); err != nil {
		return err
	}

	cw.Flush()

	return cw.Error()
}

// appendFile streams one shard into dst and checks cancellation between
// bounded copy chunks.
func appendFile(ctx context.Context, dst io.Writer, src string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	file, err := os.Open(src)
	if err != nil {
		return err
	}
	defer file.Close()

	return copyContext(ctx, dst, file)
}

func copyContext(ctx context.Context, dst io.Writer, src io.Reader) error {
	buf := make([]byte, csvBufferSize)

	for {
		if err := ctx.Err(); err != nil {
			return err
		}

		read, readErr := src.Read(buf)
		if read > 0 {
			if err := ctx.Err(); err != nil {
				return err
			}

			written, writeErr := dst.Write(buf[:read])
			if writeErr != nil {
				return writeErr
			}

			if written != read {
				return io.ErrShortWrite
			}

			if err := ctx.Err(); err != nil {
				return err
			}
		}

		switch readErr {
		case nil:
			continue
		case io.EOF:
			return nil
		default:
			return readErr
		}
	}
}

// writeAtomic writes a sibling temporary file and renames it only after the
// callback and context both succeed. Failed or canceled writes leave no file
// that can be mistaken for finalized output.
func writeAtomic(ctx context.Context, path string, write func(*os.File) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	temp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}

	removeTemp := true

	defer func() {
		_ = temp.Close()
		if removeTemp {
			_ = os.Remove(temp.Name())
		}
	}()

	if err := temp.Chmod(fileMode); err != nil {
		return err
	}

	if err := ctx.Err(); err != nil {
		return err
	}

	if err := write(temp); err != nil {
		return err
	}

	if err := ctx.Err(); err != nil {
		return err
	}

	if err := temp.Close(); err != nil {
		return err
	}

	if err := ctx.Err(); err != nil {
		return err
	}

	if err := os.Rename(temp.Name(), path); err != nil {
		return err
	}

	removeTemp = false

	return nil
}

// sortedTableNames returns table names in deterministic order. Merge
// iteration order is not observable by callers, but sorted iteration keeps
// logs and error ordering stable across runs.
func sortedTableNames(tables map[string]*tableState) []string {
	names := make([]string, 0, len(tables))

	for name := range tables {
		names = append(names, name)
	}

	sort.Strings(names)

	return names
}
