package sqldriver

import (
	"context"
	"database/sql"
	sqldriver "database/sql/driver"
	"errors"
	"io"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"go.uber.org/zap"
)

type rowsTestConnector struct {
	newRows func(context.Context) sqldriver.Rows
}

func (c *rowsTestConnector) Connect(context.Context) (sqldriver.Conn, error) {
	return &rowsTestConn{newRows: c.newRows}, nil
}

func (*rowsTestConnector) Driver() sqldriver.Driver {
	return rowsTestDriver{}
}

type rowsTestDriver struct{}

func (rowsTestDriver) Open(string) (sqldriver.Conn, error) {
	return nil, sqldriver.ErrBadConn
}

type rowsTestConn struct {
	newRows func(context.Context) sqldriver.Rows
}

func (*rowsTestConn) Prepare(string) (sqldriver.Stmt, error) { return nil, sqldriver.ErrSkip }
func (*rowsTestConn) Close() error                           { return nil }
func (*rowsTestConn) Begin() (sqldriver.Tx, error)           { return nil, sqldriver.ErrSkip }

func (c *rowsTestConn) QueryContext(
	ctx context.Context,
	_ string,
	_ []sqldriver.NamedValue,
) (sqldriver.Rows, error) {
	return c.newRows(ctx), nil
}

func openRowsTestDB(t *testing.T, newRows func(context.Context) sqldriver.Rows) *sql.DB {
	t.Helper()

	db := sql.OpenDB(&rowsTestConnector{newRows: newRows})

	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("close test database: %v", err)
		}
	})

	return db
}

type scriptedResultRows struct {
	sets        [][][]sqldriver.Value
	set         int
	row         int
	terminalErr error
	closeErr    error
	closeCalls  int
	events      []string
}

func (*scriptedResultRows) Columns() []string { return []string{"value"} }

func (r *scriptedResultRows) Next(dest []sqldriver.Value) error {
	if r.row < len(r.sets[r.set]) {
		dest[0] = r.sets[r.set][r.row][0]
		r.events = append(r.events, "row")
		r.row++

		return nil
	}

	r.events = append(r.events, "end")
	if r.set == len(r.sets)-1 && r.terminalErr != nil {
		return r.terminalErr
	}

	return io.EOF
}

func (r *scriptedResultRows) HasNextResultSet() bool {
	return r.set+1 < len(r.sets)
}

func (r *scriptedResultRows) NextResultSet() error {
	r.events = append(r.events, "result")
	if !r.HasNextResultSet() {
		return io.EOF
	}

	r.set++
	r.row = 0

	return nil
}

func (r *scriptedResultRows) Close() error {
	r.events = append(r.events, "close")
	r.closeCalls++

	return r.closeErr
}

func TestRowsCloseDrainsEveryResultSet(t *testing.T) {
	raw := &scriptedResultRows{
		sets: [][][]sqldriver.Value{
			{{int64(1)}, {int64(2)}},
			{{int64(3)}, {int64(4)}},
		},
	}
	db := openRowsTestDB(t, func(context.Context) sqldriver.Rows { return raw })

	sqlRows, err := db.QueryContext(context.Background(), "CALL test()")
	if err != nil {
		t.Fatalf("query: %v", err)
	}

	rows := NewRows(sqlRows)
	if !rows.Next() {
		t.Fatalf("first Next() = false, err = %v", rows.Err())
	}

	if got := rows.Values(); !reflect.DeepEqual(got, []any{int64(1)}) {
		t.Fatalf("first row = %#v, want [1]", got)
	}

	if err := rows.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	wantEvents := []string{"row", "row", "end", "result", "row", "row", "end", "close"}
	if !reflect.DeepEqual(raw.events, wantEvents) {
		t.Fatalf("events = %v, want %v", raw.events, wantEvents)
	}

	if err := rows.Close(); err != nil {
		t.Fatalf("second Close() error = %v", err)
	}

	if raw.closeCalls != 1 {
		t.Fatalf("driver Close() calls = %d, want 1", raw.closeCalls)
	}
}

func TestRowsClosePreservesTerminalErrors(t *testing.T) {
	tests := []struct {
		name        string
		terminalErr error
		closeErr    error
	}{
		{name: "iteration", terminalErr: errors.New("iteration failed")},
		{name: "close", closeErr: errors.New("close failed")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw := &scriptedResultRows{
				sets:        [][][]sqldriver.Value{{{int64(1)}}},
				terminalErr: tt.terminalErr,
				closeErr:    tt.closeErr,
			}
			db := openRowsTestDB(t, func(context.Context) sqldriver.Rows { return raw })

			sqlRows, err := db.QueryContext(context.Background(), "SELECT 1")
			if err != nil {
				t.Fatalf("query: %v", err)
			}

			rows := NewRows(sqlRows)
			err = rows.Close()

			wantErr := tt.terminalErr
			if wantErr == nil {
				wantErr = tt.closeErr
			}

			if !errors.Is(err, wantErr) {
				t.Fatalf("Close() error = %v, want %v", err, wantErr)
			}

			if tt.terminalErr != nil && !errors.Is(rows.Err(), tt.terminalErr) {
				t.Fatalf("Err() = %v, want terminal error", rows.Err())
			}
		})
	}
}

type contextResultRows struct {
	ctx            context.Context
	rowReturned    bool
	advanced       atomic.Bool
	closeCalls     atomic.Int32
	advanceStarted chan struct{}
	closeStarted   chan struct{}
	releaseClose   chan struct{}
	advanceOnce    sync.Once
	closeOnce      sync.Once
	releaseOnce    sync.Once
}

func newContextResultRows(ctx context.Context) *contextResultRows {
	return &contextResultRows{
		ctx:            ctx,
		advanceStarted: make(chan struct{}),
		closeStarted:   make(chan struct{}),
		releaseClose:   make(chan struct{}),
	}
}

func (*contextResultRows) Columns() []string { return []string{"value"} }

func (r *contextResultRows) Next(dest []sqldriver.Value) error {
	if !r.rowReturned {
		r.rowReturned = true
		dest[0] = int64(1)

		return nil
	}

	return io.EOF
}

func (r *contextResultRows) HasNextResultSet() bool { return !r.advanced.Load() }

func (r *contextResultRows) NextResultSet() error {
	r.advanced.Store(true)
	r.advanceOnce.Do(func() { close(r.advanceStarted) })
	<-r.ctx.Done()

	return r.ctx.Err()
}

func (r *contextResultRows) Close() error {
	r.closeCalls.Add(1)
	r.closeOnce.Do(func() { close(r.closeStarted) })

	if !r.advanced.Load() {
		<-r.releaseClose
	}

	return nil
}

func (r *contextResultRows) release() {
	r.releaseOnce.Do(func() { close(r.releaseClose) })
}

func TestRowsCloseKeepsContextActiveWhileAdvancingResultSets(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var raw *contextResultRows

	db := openRowsTestDB(t, func(queryCtx context.Context) sqldriver.Rows {
		raw = newContextResultRows(queryCtx)

		return raw
	})

	res, err := RunQuery(ctx, db, NewRows, testDialect{}, zap.NewNop(), "CALL test()", nil, 0)
	if err != nil {
		t.Fatalf("RunQuery() error = %v", err)
	}

	if !res.Rows.Next() {
		t.Fatalf("first Next() = false, err = %v", res.Rows.Err())
	}

	closeDone := make(chan error, 1)
	go func() { closeDone <- res.Rows.Close() }()

	select {
	case <-raw.advanceStarted:
		cancel()
	case <-raw.closeStarted:
		raw.release()
		<-closeDone
		t.Fatal("driver rows closed before advancing the unread result set")
	case <-time.After(time.Second):
		cancel()
		raw.release()
		t.Fatal("timed out waiting for result-set cleanup")
	}

	select {
	case err = <-closeDone:
	case <-time.After(time.Second):
		raw.release()
		t.Fatal("timed out waiting for rows to close")
	}

	raw.release()

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Close() error = %v, want context.Canceled", err)
	}

	if calls := raw.closeCalls.Load(); calls != 1 {
		t.Fatalf("driver Close() calls = %d, want 1", calls)
	}
}
