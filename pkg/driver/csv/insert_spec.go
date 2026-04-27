package csv

import (
	"bufio"
	"context"
	stdcsv "encoding/csv"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/stroppy-io/stroppy/pkg/datagen/dgproto"
	"github.com/stroppy-io/stroppy/pkg/datagen/runtime"
	"github.com/stroppy-io/stroppy/pkg/driver"
	"github.com/stroppy-io/stroppy/pkg/driver/common"
	"github.com/stroppy-io/stroppy/pkg/driver/stats"
)

// ErrUnsupportedInsertMethod is returned when an InsertSpec requests
// anything other than NATIVE. CSV is write-only: PLAIN_BULK and
// PLAIN_QUERY imply SQL-shaped emission, which the CSV driver does
// not synthesize. Matches the rejection pattern used by the other
// drivers.
var ErrUnsupportedInsertMethod = errors.New("csv: unsupported InsertSpec method")

// InsertSpec runs one relational InsertSpec through the CSV driver by
// draining a seed runtime.Runtime into one file per worker. Under
// parallelism each worker writes to its own shard so the hot path is
// lock-free; final per-table merge happens at Teardown when
// merge=true.
func (d *Driver) InsertSpec(
	ctx context.Context,
	spec *dgproto.InsertSpec,
) (*stats.Query, error) {
	if spec == nil {
		return nil, fmt.Errorf("csv: %w", runtime.ErrInvalidSpec)
	}

	if spec.GetMethod() != dgproto.InsertMethod_NATIVE {
		return nil, fmt.Errorf("%w: %s", ErrUnsupportedInsertMethod, spec.GetMethod().String())
	}

	workers := int(spec.GetParallelism().GetWorkers())
	if workers <= 1 {
		return d.insertSpecSingle(spec)
	}

	return d.insertSpecParallel(ctx, spec, workers)
}

// insertSpecSingle runs the spec as a single shard labeled w000.
func (d *Driver) insertSpecSingle(spec *dgproto.InsertSpec) (*stats.Query, error) {
	rt, err := runtime.NewRuntime(spec)
	if err != nil {
		return nil, fmt.Errorf("csv: build runtime: %w", err)
	}

	start := time.Now()

	count, err := d.writeShard(spec.GetTable(), rt, 0, -1)
	if err != nil {
		return nil, err
	}

	d.recordShards(spec.GetTable(), rt.Columns(), 1, count)

	return &stats.Query{Elapsed: time.Since(start)}, nil
}

// insertSpecParallel fans the spec out across workers goroutines via
// common.RunParallel. Each worker writes its own shard file labeled
// w%03d where %d is the chunk index, so contention is limited to the
// two small metadata structures (d.tables and common.RunParallel's
// errgroup) and not to file I/O.
func (d *Driver) insertSpecParallel(
	ctx context.Context,
	spec *dgproto.InsertSpec,
	workers int,
) (*stats.Query, error) {
	total := spec.GetSource().GetPopulation().GetSize()
	chunks := common.SplitChunks(total, workers)

	start := time.Now()

	var columns []string

	err := common.RunParallel(ctx, spec, chunks,
		func(_ context.Context, chunk common.Chunk, rt *runtime.Runtime) error {
			rowCount, err := d.writeShard(spec.GetTable(), rt, chunk.Index, chunk.Count)
			if err != nil {
				return err
			}

			d.recordShards(spec.GetTable(), rt.Columns(), 1, rowCount)

			if chunk.Index == 0 {
				columns = append([]string(nil), rt.Columns()...)
			}

			return nil
		})
	if err != nil {
		return nil, err
	}

	// Make sure the registry has the canonical column order even when
	// the first-indexed worker completed after a later one.
	if len(columns) > 0 {
		d.recordShards(spec.GetTable(), columns, 0, 0)
	}

	return &stats.Query{Elapsed: time.Since(start)}, nil
}

// writeShard drains rt (or stops after count rows when count >= 0),
// serializing each row into the shard file for table/worker. Returns
// the number of rows written.
func (d *Driver) writeShard(
	table string,
	rt *runtime.Runtime,
	workerIdx int,
	count int64,
) (int64, error) {
	shardPath := d.shardPath(table, workerIdx)

	if err := os.MkdirAll(filepath.Dir(shardPath), dirMode); err != nil {
		return 0, fmt.Errorf("csv: mkdir %q: %w", filepath.Dir(shardPath), err)
	}

	file, err := os.Create(shardPath)
	if err != nil {
		return 0, fmt.Errorf("csv: create %q: %w", shardPath, err)
	}

	buf := bufio.NewWriterSize(file, csvBufferSize)
	writer := stdcsv.NewWriter(buf)
	writer.Comma = d.cfg.separator

	written, err := drainRows(rt, writer, table, count)
	if err != nil {
		_ = file.Close()

		return written, err
	}

	writer.Flush()

	if werr := writer.Error(); werr != nil {
		_ = file.Close()

		return written, fmt.Errorf("csv: flush %q: %w", table, werr)
	}

	if ferr := buf.Flush(); ferr != nil {
		_ = file.Close()

		return written, fmt.Errorf("csv: bufio flush %q: %w", table, ferr)
	}

	if cerr := file.Close(); cerr != nil {
		return written, fmt.Errorf("csv: close %q: %w", shardPath, cerr)
	}

	return written, nil
}

// drainRows pulls rows from rt, encodes each into record strings, and
// writes them to writer until EOF or count is reached. writer.Flush
// is the caller's responsibility.
func drainRows(
	rt *runtime.Runtime,
	writer *stdcsv.Writer,
	table string,
	count int64,
) (int64, error) {
	var (
		written int64
		record  []string
	)

	for count < 0 || written < count {
		row, err := rt.Next()
		if errors.Is(err, io.EOF) {
			break
		}

		if err != nil {
			return written, fmt.Errorf("csv: runtime.Next %q: %w", table, err)
		}

		record = record[:0]
		for _, v := range row {
			record = append(record, encodeValue(v))
		}

		if err := writer.Write(record); err != nil {
			return written, fmt.Errorf("csv: write %q row %d: %w", table, written, err)
		}

		written++
	}

	return written, nil
}

// shardPath returns the filesystem path for the given table/worker
// shard. Layout depends on cfg.merge:
//   - merge=true:  <outdir>/<workload>/.shards/<table>.w%03d.csv
//   - merge=false: <outdir>/<workload>/<table>.w%03d.csv
func (d *Driver) shardPath(table string, workerIdx int) string {
	dir := d.resolveWorkload()

	if d.cfg.merge {
		dir = filepath.Join(dir, ".shards")
	}

	name := fmt.Sprintf("%s.w%03d.csv", table, workerIdx)

	return filepath.Join(dir, name)
}

// recordShards accumulates shard and row counts for the given table,
// lazily installing a tableState on first observation. Column order
// is captured on first non-empty input and never overwritten — every
// shard in a single InsertSpec run reports the same column order.
func (d *Driver) recordShards(table string, columns []string, shards int, rows int64) {
	d.mu.Lock()
	defer d.mu.Unlock()

	ts, ok := d.tables[table]
	if !ok {
		ts = &tableState{columns: append([]string(nil), columns...)}
		d.tables[table] = ts
	}

	if len(ts.columns) == 0 && len(columns) > 0 {
		ts.columns = append([]string(nil), columns...)
	}

	ts.shards += shards
	ts.rowCount += rows
}

// encodeValue converts a runtime-produced value into its CSV field
// representation. nil maps to an empty string (the PostgreSQL COPY
// default, and what every downstream CSV loader expects). All other
// types use a stable, RFC-4180-compatible text form.
func encodeValue(val any) string {
	switch typed := val.(type) {
	case nil:
		return ""
	case string:
		return typed
	case []byte:
		return string(typed)
	case bool:
		if typed {
			return "true"
		}

		return "false"
	case time.Time:
		return typed.UTC().Format(time.RFC3339Nano)
	case *time.Time:
		if typed == nil {
			return ""
		}

		return typed.UTC().Format(time.RFC3339Nano)
	case decimal.Decimal:
		return typed.String()
	case *decimal.Decimal:
		if typed == nil {
			return ""
		}

		return typed.String()
	case uuid.UUID:
		return typed.String()
	case fmt.Stringer:
		return typed.String()
	default:
		if s, ok := encodeNumeric(val); ok {
			return s
		}

		return fmt.Sprint(val)
	}
}

// encodeNumeric handles every integer and floating-point arm. Split
// out so encodeValue stays under the cyclomatic-complexity cap.
func encodeNumeric(val any) (string, bool) {
	switch typed := val.(type) {
	case int:
		return strconv.FormatInt(int64(typed), 10), true
	case int32:
		return strconv.FormatInt(int64(typed), 10), true
	case int64:
		return strconv.FormatInt(typed, 10), true
	case uint32:
		return strconv.FormatUint(uint64(typed), 10), true
	case uint64:
		return strconv.FormatUint(typed, 10), true
	case float32:
		return strconv.FormatFloat(float64(typed), 'g', -1, 32), true
	case float64:
		return strconv.FormatFloat(typed, 'g', -1, 64), true
	default:
		return "", false
	}
}

// Ensure driver.Driver stays satisfied when this file is compiled
// alongside driver.go. The interface conformance assertion in
// driver.go keeps the two files in lockstep.
var _ driver.Driver = (*Driver)(nil)
