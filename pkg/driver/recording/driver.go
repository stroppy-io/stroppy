// Package recording implements a database-free driver that records operations.
package recording

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"sync/atomic"
	"time"

	"github.com/stroppy-io/stroppy/v6/pkg/config"
	"github.com/stroppy-io/stroppy/v6/pkg/driver"
	"github.com/stroppy-io/stroppy/v6/pkg/driver/common"
	"github.com/stroppy-io/stroppy/v6/pkg/driver/insertprogress"
	"github.com/stroppy-io/stroppy/v6/pkg/driver/stats"
	"github.com/stroppy-io/stroppy/v6/pkg/gen"
	"github.com/stroppy-io/stroppy/v6/pkg/record"
)

const (
	recordingFileMode  = 0o600
	maximumSampleRows  = 10000
	recordingBatchRows = 64
)

func init() { driver.RegisterDriver(config.DriverTypeRecording, newDriver) }

type Driver struct {
	recorder *record.Recorder
	output   *os.File
}

func newDriver(_ context.Context, options driver.Options) (driver.Driver, error) {
	d := &Driver{recorder: options.Config.Recording}
	if d.recorder != nil {
		return d, nil
	}

	d.recorder = &record.Recorder{}

	path := options.Config.URL
	if path == "" {
		path = "recording.json"
	}

	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, recordingFileMode)
	if err != nil {
		return nil, fmt.Errorf("create recording: %w", err)
	}

	d.output = file

	return d, nil
}

func (d *Driver) Teardown(context.Context) error {
	if d.output == nil {
		return nil
	}

	return errors.Join(d.recorder.WriteTo(d.output), d.output.Close())
}
func (*Driver) ClassifyError(err error) driver.ErrorFacts { return driver.DefaultErrorFacts(err) }
func (d *Driver) RunQuery(ctx context.Context, sql string, args map[string]any) (*driver.QueryResult, error) {
	return d.query(ctx, 0, sql, args)
}

func (d *Driver) query(
	ctx context.Context, transaction uint64, sql string, args map[string]any,
) (*driver.QueryResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	operation := record.Operation{
		Scope: record.ContextScope(ctx), Kind: "query", Transaction: transaction,
		SQL: sql, Arguments: map[string]record.Value{},
	}

	for name, value := range args {
		encoded, err := record.Encode(value)
		if err != nil {
			return nil, err
		}

		operation.Arguments[name] = encoded
	}

	response := d.recorder.Answer(sql)
	if response.Err != nil {
		operation.Error = response.Err.Error()
	}

	d.recorder.Add(operation)

	if response.Err != nil {
		return nil, response.Err
	}

	return &driver.QueryResult{Stats: &stats.Query{}, Rows: &rows{response: response, index: -1}}, nil
}

func (d *Driver) Begin(ctx context.Context, isolation config.TxIsolationLevel) (driver.Tx, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	scope := record.ContextScope(ctx)
	id := d.recorder.Begin(scope)
	d.recorder.Add(record.Operation{Scope: scope, Kind: "begin", Transaction: id, Isolation: isolationName(isolation)})

	return &transaction{driver: d, id: id, isolation: isolation}, nil
}

func isolationName(value config.TxIsolationLevel) string {
	names := map[config.TxIsolationLevel]string{
		config.TxIsolationLevelUnspecified:     "db_default",
		config.TxIsolationLevelReadUncommitted: "read_uncommitted",
		config.TxIsolationLevelReadCommitted:   "read_committed",
		config.TxIsolationLevelRepeatableRead:  "repeatable_read",
		config.TxIsolationLevelSerializable:    "serializable",
		config.TxIsolationLevelConnectionOnly:  "conn",
		config.TxIsolationLevelNone:            "none",
	}

	return names[value]
}

type transaction struct {
	driver    *Driver
	id        uint64
	isolation config.TxIsolationLevel
}

func (tx *transaction) RunQuery(ctx context.Context, sql string, args map[string]any) (*driver.QueryResult, error) {
	return tx.driver.query(ctx, tx.id, sql, args)
}
func (tx *transaction) Commit(ctx context.Context) error   { return tx.end(ctx, "commit") }
func (tx *transaction) Rollback(ctx context.Context) error { return tx.end(ctx, "rollback") }
func (tx *transaction) Isolation() config.TxIsolationLevel { return tx.isolation }
func (tx *transaction) end(ctx context.Context, kind string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	tx.driver.recorder.Add(record.Operation{Scope: record.ContextScope(ctx), Kind: kind, Transaction: tx.id})

	return nil
}

type rows struct {
	response record.Response
	index    int
	closed   bool
}

func (r *rows) Columns() []string { return slices.Clone(r.response.Columns) }
func (r *rows) Next() bool {
	if r.closed {
		return false
	}

	r.index++

	return r.index < len(r.response.Rows)
}
func (r *rows) Values() []any { return r.response.Rows[r.index] }
func (r *rows) ReadAll(limit int) [][]any {
	out := [][]any{}
	for (limit <= 0 || len(out) < limit) && r.Next() {
		out = append(out, r.Values())
	}

	return out
}
func (*rows) Err() error { return nil }
func (r *rows) Close() error {
	r.closed = true

	return nil
}

//nolint:gocognit // row draining, bounded samples and operation outcome are recorded together.
func (d *Driver) Insert(ctx context.Context, request *driver.InsertRequest) (*stats.Query, error) {
	if err := driver.ValidateInsert(request); err != nil {
		return nil, err
	}

	started := time.Now()
	columns := request.Source.Schema().ColumnNames()

	var count atomic.Int64

	limit := min(max(d.recorder.SampleRows, 0), maximumSampleRows)
	collected := [][]record.Value{}

	workers := request.Workers
	if limit > 0 {
		workers = 1
	}

	drain := func(ctx context.Context, chunk common.Chunk, cursor gen.Cursor) error {
		source := common.NewBatchRowSource(cursor, columns, len(columns))
		generated := insertprogress.NewGeneratedRowCounter(ctx)
		confirmed := insertprogress.NewConfirmedRowCounter(ctx)

		defer generated.Flush()
		defer confirmed.Flush()

		index := chunk.Start

		for {
			if err := ctx.Err(); err != nil {
				return err
			}

			row, err := source.Next()
			if errors.Is(err, io.EOF) {
				return nil
			}

			if err != nil {
				return err
			}

			count.Add(1)
			generated.Add(1)
			confirmed.Add(1)

			if index < int64(limit) {
				values := make([]record.Value, len(row))
				for i, value := range row {
					encoded, err := record.Encode(value)
					if err != nil {
						return err
					}

					values[i] = encoded
				}

				collected = append(collected, values)
			}

			index++
		}
	}
	_, err := common.RunParallelBatch(ctx, request.Source, workers, recordingBatchRows, drain)

	operation := record.Operation{
		Scope: record.ContextScope(ctx), Kind: "insert", Table: request.Table,
		Columns: columns, Method: request.Method.String(), Rows: count.Load(), Sample: collected,
	}

	if err != nil {
		operation.Error = err.Error()
	}

	d.recorder.Add(operation)

	return &stats.Query{Rows: count.Load(), Elapsed: time.Since(started)}, err
}
