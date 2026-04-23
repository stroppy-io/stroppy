package csv

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	stroppy "github.com/stroppy-io/stroppy/pkg/common/proto/stroppy"
	"github.com/stroppy-io/stroppy/pkg/driver"
	"github.com/stroppy-io/stroppy/pkg/driver/stats"
)

// ErrCsvDriverNoQuery is returned when a non-DDL query reaches the
// CSV driver. CSV is write-only: it has no result set to produce and
// no transaction to run under. DDL emitted by the drop_schema and
// create_schema workload steps is recognized and handled out-of-band
// (DROP removes the workload's output directory; CREATE is a noop),
// so these steps remain runnable alongside load_data.
var ErrCsvDriverNoQuery = errors.New("csv: driver does not execute queries")

// RunQuery accepts DDL (CREATE/DROP/TRUNCATE/ALTER/COMMENT) as a noop
// so workload drop_schema and create_schema steps stay valid with the
// CSV driver selected. DROP is treated as a directive to wipe the
// workload's output directory; everything else silently succeeds.
// Non-DDL queries return ErrCsvDriverNoQuery.
func (d *Driver) RunQuery(
	_ context.Context,
	sqlStr string,
	_ map[string]any,
) (*driver.QueryResult, error) {
	verb := firstKeyword(sqlStr)

	switch verb {
	case "":
		// Empty / whitespace-only SQL — treat as noop.
		return emptyQueryResult(), nil
	case "DROP", "TRUNCATE":
		if err := d.wipeWorkloadDir(); err != nil {
			return nil, err
		}

		return emptyQueryResult(), nil
	case "CREATE", "ALTER", "COMMENT", "SET":
		return emptyQueryResult(), nil
	default:
		return nil, fmt.Errorf("%w: %s", ErrCsvDriverNoQuery, verb)
	}
}

// Begin refuses to start a transaction. CSV writes have no rollback
// semantics and workloads that call tx.* are not supported.
func (d *Driver) Begin(_ context.Context, _ stroppy.TxIsolationLevel) (driver.Tx, error) {
	return nil, fmt.Errorf("%w: Begin", ErrCsvDriverNoQuery)
}

// wipeWorkloadDir deletes the workload output directory when it
// exists. Used to honor drop_schema's intent under the CSV driver so
// successive runs do not accumulate stale shards. A missing dir is
// not an error.
func (d *Driver) wipeWorkloadDir() error {
	dir := d.resolveWorkload()

	if err := os.RemoveAll(dir); err != nil {
		return fmt.Errorf("csv: wipe %q: %w", dir, err)
	}

	d.mu.Lock()
	d.tables = make(map[string]*tableState)
	d.mu.Unlock()

	return nil
}

// firstKeyword returns the first uppercase SQL keyword in sqlStr (up
// to the first whitespace / semicolon / open-paren).
// "  DROP TABLE foo" -> "DROP".
func firstKeyword(sqlStr string) string {
	trimmed := strings.TrimSpace(sqlStr)
	if trimmed == "" {
		return ""
	}

	end := len(trimmed)

	for i, r := range trimmed {
		if r == ' ' || r == '\t' || r == '\n' || r == '\r' || r == ';' || r == '(' {
			end = i

			break
		}
	}

	return strings.ToUpper(trimmed[:end])
}

// emptyQueryResult returns a DDL-style QueryResult: stats with zero
// elapsed and an empty rows cursor. Workloads that observe the result
// cannot inspect affected-row counts, which aligns with the noop
// driver's shape.
func emptyQueryResult() *driver.QueryResult {
	return &driver.QueryResult{
		Stats: &stats.Query{},
		Rows:  &emptyRows{},
	}
}

// emptyRows is a one-shot empty cursor returned by DDL-noop RunQuery
// calls. Any attempt to read from it reports zero rows.
type emptyRows struct{}

var _ driver.Rows = (*emptyRows)(nil)

func (*emptyRows) Columns() []string     { return []string{} }
func (*emptyRows) Next() bool            { return false }
func (*emptyRows) Values() []any         { return nil }
func (*emptyRows) ReadAll(_ int) [][]any { return nil }
func (*emptyRows) Err() error            { return nil }
func (*emptyRows) Close() error          { return nil }
