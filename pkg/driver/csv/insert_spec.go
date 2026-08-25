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
	"slices"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/stroppy-io/stroppy/pkg/datagen/source"
	"github.com/stroppy-io/stroppy/pkg/driver"
	"github.com/stroppy-io/stroppy/pkg/driver/common"
	"github.com/stroppy-io/stroppy/pkg/driver/insertprogress"
	"github.com/stroppy-io/stroppy/pkg/driver/stats"
	"github.com/stroppy-io/stroppy/pkg/gen"
)

// ErrUnsupportedInsertMethod is returned when an InsertRequest requests
// anything other than NATIVE. CSV is write-only: PLAIN_BULK and
// PLAIN_QUERY imply SQL-shaped emission, which the CSV driver does
// not synthesize. Matches the rejection pattern used by the other
// drivers.
var (
	ErrUnsupportedInsertMethod = fmt.Errorf("csv: %w", driver.ErrInsertMethodNotSupported)
	errColumnLayoutChanged     = errors.New("csv: table column layout changed")
	errTableNotPrepared        = errors.New("csv: table was not prepared")
)

// Insert runs a typed [driver.InsertRequest] through the CSV driver by
// draining each worker's [gen.Cursor] partition into a distinct shard file.
// NATIVE is the only method: CSV is write-only and does not synthesize
// SQL-shaped emission for PLAIN_BULK/PLAIN_QUERY.
func (d *Driver) Insert(
	ctx context.Context,
	req *driver.InsertRequest,
) (_ *stats.Query, retErr error) {
	d.lifecycleMu.Lock()
	defer d.lifecycleMu.Unlock()
	defer func() {
		if retErr != nil {
			d.markInsertFailed()
		}
	}()

	if err := driver.ValidateInsert(req); err != nil {
		return nil, err
	}

	if req.Method != driver.InsertNative {
		return nil, fmt.Errorf("%w: %s", ErrUnsupportedInsertMethod, req.Method)
	}

	workers := req.Workers
	if workers < 1 {
		workers = 1
	}

	columns := req.Source.Schema().ColumnNames()

	startShard, err := d.prepareTable(ctx, req.Table, columns)
	if err != nil {
		return nil, err
	}

	start := time.Now()

	rows, err := common.RunParallelBatch(ctx, req.Source, workers, csvBatchRows,
		func(workerCtx context.Context, chunk common.Chunk, cur gen.Cursor) error {
			src := common.NewBatchRowSource(cur, columns, len(columns))

			rowCount, err := d.writeShard(workerCtx, req.Table, src, startShard+chunk.Index)
			if err != nil {
				return err
			}

			return d.recordShard(req.Table, rowCount)
		})
	if err != nil {
		return nil, err
	}

	return &stats.Query{Elapsed: time.Since(start), Rows: rows}, nil
}

// writeShard drains src to EOF, serializing each row into the shard file
// for table/worker. Returns the number of rows written.
func (d *Driver) writeShard(
	ctx context.Context,
	table string,
	src source.RowSource,
	workerIdx int,
) (int64, error) {
	shardPath := d.shardPath(table, workerIdx)

	if err := os.MkdirAll(filepath.Dir(shardPath), dirMode); err != nil {
		return 0, fmt.Errorf("csv: mkdir %q: %w", filepath.Dir(shardPath), err)
	}

	var written int64

	start := time.Now()

	err := writeAtomic(ctx, shardPath, func(file *os.File) error {
		buf := bufio.NewWriterSize(file, csvBufferSize)
		writer := stdcsv.NewWriter(buf)
		writer.Comma = d.cfg.separator

		var err error

		written, err = drainRows(ctx, src, writer, table)
		if err != nil {
			return err
		}

		writer.Flush()

		if err := writer.Error(); err != nil {
			return fmt.Errorf("csv: flush %q: %w", table, err)
		}

		if err := buf.Flush(); err != nil {
			return fmt.Errorf("csv: bufio flush %q: %w", table, err)
		}

		return ctx.Err()
	})
	if err != nil {
		return written, fmt.Errorf("csv: write shard %q: %w", shardPath, err)
	}

	insertprogress.AddBatch(ctx, written, time.Since(start))

	return written, nil
}

// drainRows pulls rows from src, encodes each into record strings, and
// writes them to writer until EOF. writer.Flush is the caller's
// responsibility.

// csvBatchRows is the per-cursor typed-batch capacity the CSV path
// prepares. CSV reads one row at a time and has no driver bulk size, so
// this only sizes the reusable gen batch; a modest value balances cursor
// fill cost against memory.
const csvBatchRows = 4096

func drainRows(
	ctx context.Context,
	src source.RowSource,
	writer *stdcsv.Writer,
	table string,
) (int64, error) {
	var (
		generatedProgress = insertprogress.NewGeneratedRowCounter(ctx)
		confirmedProgress = insertprogress.NewConfirmedRowCounter(ctx)
		written           int64
		record            []string
	)
	defer generatedProgress.Flush()
	defer confirmedProgress.Flush()

	insertprogress.SetStage(ctx, insertprogress.StageCSVWrite)

	for {
		if err := insertprogress.Canceled(ctx); err != nil {
			return written, err
		}

		row, err := src.Next()
		if errors.Is(err, io.EOF) {
			break
		}

		if err != nil {
			return written, fmt.Errorf("csv: source.Next %q: %w", table, err)
		}

		generatedProgress.Add(1)

		record = record[:0]
		for _, v := range row {
			record = append(record, encodeValue(v))
		}

		if err := writer.Write(record); err != nil {
			return written, fmt.Errorf("csv: write %q row %d: %w", table, written, err)
		}

		written++

		confirmedProgress.Add(1)
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

// prepareTable invalidates any prior completed generation, clears stale shards
// on first use of a table, and returns the first unused shard index.
func (d *Driver) prepareTable(ctx context.Context, table string, columns []string) (int, error) {
	workloadDir := d.resolveWorkload()
	if err := invalidateManifest(ctx, workloadDir); err != nil {
		return 0, fmt.Errorf("csv: invalidate manifest: %w", err)
	}

	d.mu.Lock()

	ts := d.tables[table]
	if ts != nil {
		defer d.mu.Unlock()

		if !slices.Equal(ts.columns, columns) {
			return 0, fmt.Errorf("%w for %q: %v to %v", errColumnLayoutChanged, table, ts.columns, columns)
		}

		return ts.shards, nil
	}
	d.mu.Unlock()

	if err := removeTableArtifacts(ctx, workloadDir, table); err != nil {
		return 0, err
	}

	d.mu.Lock()
	d.tables[table] = &tableState{columns: append([]string(nil), columns...)}
	d.mu.Unlock()

	return 0, ctx.Err()
}

func removeTableArtifacts(ctx context.Context, workloadDir, table string) error {
	paths := []string{
		filepath.Join(workloadDir, table+".csv"),
		filepath.Join(workloadDir, table+".header.csv"),
	}

	for _, dir := range []string{workloadDir, filepath.Join(workloadDir, ".shards")} {
		shards, err := shardFiles(dir, table)
		if err != nil {
			return err
		}

		paths = append(paths, shards...)
	}

	for _, path := range paths {
		if err := ctx.Err(); err != nil {
			return err
		}

		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("csv: remove stale artifact %q: %w", path, err)
		}
	}

	return ctx.Err()
}

func (d *Driver) recordShard(table string, rows int64) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	ts := d.tables[table]
	if ts == nil {
		return fmt.Errorf("%w: %q", errTableNotPrepared, table)
	}

	ts.shards++
	ts.rowCount += rows

	return nil
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
			return csvTrue
		}

		return csvFalse
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
