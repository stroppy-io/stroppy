package tpcds

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/stroppy-io/stroppy/v6/pkg/bench"
)

var (
	errMaintenanceState = errors.New("tpcds: refresh state mismatch or incomplete prior set; reload or reconcile")
	errMaintenanceSQL   = errors.New("tpcds: missing maintenance SQL")
	errMaintenanceRange = errors.New("tpcds: malformed refresh delete range")
)

const maintenanceReadState = "SELECT scale_factor,next_set,active_set " +
	"FROM stroppy_tpcds_refresh WHERE id=1 FOR UPDATE"

type maintenanceFunction struct {
	name    string
	table   string
	sources []string
}

var maintenanceFunctions = []maintenanceFunction{
	{"DF_CS", "catalog_sales", nil},
	{"DF_SS", "store_sales", nil},
	{"DF_WS", "web_sales", nil},
	{"DF_I", "inventory", nil},
	{"LF_CS", "catalog_sales", []string{"s_catalog_order", "s_catalog_order_lineitem"}},
	{"LF_CR", "catalog_returns", []string{"s_catalog_returns"}},
	{"LF_SS", "store_sales", []string{"s_purchase", "s_purchase_lineitem"}},
	{"LF_SR", "store_returns", []string{"s_store_returns"}},
	{"LF_WS", "web_sales", []string{"s_web_order", "s_web_order_lineitem"}},
	{"LF_WR", "web_returns", []string{"s_web_returns"}},
	{"LF_I", "inventory", []string{"s_inventory"}},
}

type maintenanceAttempt struct {
	Set      int64   `json:"set"`
	Function string  `json:"function"`
	Rows     int64   `json:"committed_rows"`
	Seconds  float64 `json:"seconds"`
	Status   string  `json:"status"`
	Error    string  `json:"error,omitempty"`
}

func (w *workload) initializeMaintenance(ctx context.Context, b *bench.Bench) error {
	session, err := b.Session(ctx)
	if err != nil {
		return err
	}

	if err := session.Exec(ctx, `CREATE TABLE IF NOT EXISTS stroppy_tpcds_refresh (
 id INTEGER PRIMARY KEY, scale_factor VARCHAR(32) NOT NULL,
 next_set BIGINT NOT NULL, active_set BIGINT NOT NULL)`, nil); err != nil {
		return err
	}

	return session.BeginTx(ctx, bench.BeginOpts{Isolation: bench.IsoReadCommitted}, func(tx *bench.Tx) error {
		rows, err := tx.RawRows(ctx, maintenanceReadState, nil)
		if err != nil {
			return err
		}

		if len(rows) == 0 {
			if w.benchmark.FirstSet != 1 {
				return errMaintenanceState
			}

			return tx.Exec(ctx, "INSERT INTO stroppy_tpcds_refresh VALUES (1, :scale, 1, 0)",
				map[string]any{"scale": strconv.FormatFloat(w.scaleFactor, 'g', -1, 64)})
		}

		return w.checkMaintenanceRow(rows[0], w.benchmark.FirstSet)
	})
}

func (w *workload) checkMaintenanceRow(row []any, set int64) error {
	if len(row) != 3 {
		return errMaintenanceState
	}

	scale, err := strconv.ParseFloat(fmt.Sprint(row[0]), 64)
	if err != nil || scale != w.scaleFactor || fmt.Sprint(row[1]) != strconv.FormatInt(set, 10) ||
		fmt.Sprint(row[2]) != "0" {
		return errMaintenanceState
	}

	return nil
}

func (w *workload) claimMaintenance(ctx context.Context, session *bench.Session, set int64) error {
	return session.BeginTx(ctx, bench.BeginOpts{Isolation: bench.IsoReadCommitted}, func(tx *bench.Tx) error {
		rows, err := tx.RawRows(ctx, maintenanceReadState, nil)
		if err != nil {
			return err
		}

		if len(rows) != 1 {
			return errMaintenanceState
		}

		if err := w.checkMaintenanceRow(rows[0], set); err != nil {
			return err
		}

		return tx.Exec(ctx, "UPDATE stroppy_tpcds_refresh SET active_set = :set WHERE id=1", map[string]any{"set": set})
	})
}

func (w *workload) runMaintenance(ctx context.Context, b *bench.Bench, first int64) error {
	session, err := b.Session(ctx)
	if err != nil {
		return err
	}

	for set := first; set < first+int64(w.streams/2); set++ {
		if err := w.claimMaintenance(ctx, session, set); err != nil {
			return err
		}

		for _, function := range maintenanceFunctions {
			if err := w.runMaintenanceFunction(ctx, session, set, function); err != nil {
				return err
			}

			attempt := w.benchmark.Attempts[len(w.benchmark.Attempts)-1]
			b.Log.Info("maintenance completed", "set", set, "function", function.name,
				"rows", attempt.Rows, "seconds", attempt.Seconds)
		}

		if err := session.BeginTx(ctx, bench.BeginOpts{Isolation: bench.IsoReadCommitted}, func(tx *bench.Tx) error {
			return tx.Exec(ctx, "UPDATE stroppy_tpcds_refresh SET next_set = :next, active_set=0 "+
				"WHERE id=1 AND active_set = :set", map[string]any{"set": set, "next": set + 1})
		}); err != nil {
			w.benchmark.NextSet = 0

			return err
		}

		w.benchmark.NextSet = set + 1
	}

	return nil
}

func (w *workload) runMaintenanceFunction(
	ctx context.Context, session *bench.Session, set int64, function maintenanceFunction,
) error {
	attempt := maintenanceAttempt{Set: set, Function: function.name, Status: "failed"}
	started := time.Now()

	var rows int64

	err := session.BeginTx(ctx, bench.BeginOpts{Isolation: bench.IsoReadCommitted, Name: function.name},
		func(tx *bench.Tx) error {
			var err error
			if len(function.sources) == 0 {
				rows, err = w.deleteMaintenance(ctx, tx, set, function.table)
			} else {
				rows, err = w.insertMaintenance(ctx, tx, set, function)
			}

			return err
		})

	attempt.Seconds = time.Since(started).Seconds()
	if err == nil {
		attempt.Rows, attempt.Status = rows, "completed"
	} else {
		attempt.Error = err.Error()
	}

	w.benchmark.Attempts = append(w.benchmark.Attempts, attempt)
	if err != nil {
		w.benchmark.NextSet = 0

		return fmt.Errorf("set %d %s: %w", set, function.name, err)
	}

	return nil
}

func (w *workload) insertMaintenance(
	ctx context.Context, tx *bench.Tx, set int64, function maintenanceFunction,
) (int64, error) {
	for _, source := range function.sources {
		ddl, found := w.maintenanceSQL.Query("staging", source)
		if !found {
			return 0, fmt.Errorf("%w: staging %s", errMaintenanceSQL, source)
		}

		if err := tx.Exec(ctx, ddl, nil); err != nil {
			return 0, err
		}

		path := filepath.Join(w.benchmark.directory, strconv.FormatInt(set, 10), source+".dat")
		if err := loadMaintenanceFile(ctx, tx, source, path); err != nil {
			return 0, err
		}
	}

	query, found := w.maintenanceSQL.Query(function.name, "rows")
	if !found {
		return 0, fmt.Errorf("%w: %s", errMaintenanceSQL, function.name)
	}

	if w.driver == bench.DriverMySQL {
		query = strings.ReplaceAll(query, " AS integer)", " AS SIGNED)")
	}

	rows, err := w.maintenanceAffected(ctx, tx, "INSERT INTO "+function.table+" "+query)
	if err != nil {
		return 0, err
	}

	for _, source := range function.sources {
		drop := "DROP TABLE "
		if w.driver == bench.DriverMySQL {
			drop = "DROP TEMPORARY TABLE "
		}

		if err := tx.Exec(ctx, drop+source, nil); err != nil {
			return 0, err
		}
	}

	return rows, nil
}

func (w *workload) maintenanceAffected(ctx context.Context, tx *bench.Tx, statement string) (int64, error) {
	statement = strings.TrimSuffix(strings.TrimSpace(statement), ";")
	if w.driver == bench.DriverMySQL {
		if err := tx.Exec(ctx, statement, nil); err != nil {
			return 0, err
		}

		return tx.QueryValue[int64](ctx, "SELECT ROW_COUNT()", nil)
	}

	if w.sqlDialect == "gp" {
		return maintenanceMPPAffected(ctx, tx, statement)
	}

	return tx.QueryValue[int64](ctx, "WITH affected AS ("+statement+" RETURNING 1) SELECT COUNT(*) FROM affected", nil)
}

func maintenanceMPPAffected(ctx context.Context, tx *bench.Tx, statement string) (int64, error) {
	var query string

	if strings.HasPrefix(statement, "INSERT INTO ") {
		_, selectSQL, found := strings.Cut(statement, " SELECT ")
		if !found {
			return 0, errMaintenanceSQL
		}

		query = "SELECT COUNT(*) FROM (SELECT " + selectSQL + ") affected"
	} else {
		query = strings.Replace(statement, "DELETE FROM ", "SELECT COUNT(*) FROM ", 1)
	}

	count, err := tx.QueryValue[int64](ctx, query, nil)
	if err != nil {
		return 0, err
	}

	if err := tx.Exec(ctx, statement, nil); err != nil {
		return 0, err
	}

	return count, nil
}

func maintenanceDeleteStatements(table string, dates []string) []string {
	dateRange := "SELECT d_date_sk FROM date_dim WHERE d_date BETWEEN '" + dates[0] + "' AND '" + dates[1] + "'"
	if table == "inventory" {
		return []string{"DELETE FROM inventory WHERE inv_date_sk IN (" + dateRange + ")"}
	}

	prefix := map[string]string{"store_sales": "ss", "catalog_sales": "cs", "web_sales": "ws"}[table]
	returns := strings.Replace(table, "sales", "returns", 1)
	returnPrefix := prefix[:1] + "r"

	key := "order_number"
	if prefix == "ss" {
		key = "ticket_number"
	}

	return []string{
		"DELETE FROM " + returns + " WHERE EXISTS (SELECT 1 FROM " + table + " WHERE " +
			prefix + "_" + key + "=" + returnPrefix + "_" + key + " AND " +
			prefix + "_item_sk=" + returnPrefix + "_item_sk AND " +
			prefix + "_sold_date_sk IN (" + dateRange + "))",
		"DELETE FROM " + table + " WHERE " + prefix + "_sold_date_sk IN (" + dateRange + ")",
	}
}

func (w *workload) deleteMaintenance(ctx context.Context, tx *bench.Tx, set int64, table string) (int64, error) {
	file := "delete"
	if table == "inventory" {
		file = "inventory_delete"
	}

	ranges, err := os.Open(filepath.Join(w.benchmark.directory, strconv.FormatInt(set, 10), file+".dat"))
	if err != nil {
		return 0, err
	}
	defer ranges.Close()

	scanner := bufio.NewScanner(ranges)

	var total int64

	for scanner.Scan() {
		parts := strings.Split(scanner.Text(), "|")
		if len(parts) != 3 {
			return 0, errMaintenanceRange
		}

		for _, date := range parts[:2] {
			if _, err := time.Parse("2006-01-02", date); err != nil {
				return 0, err
			}
		}

		for _, statement := range maintenanceDeleteStatements(table, parts) {
			count, err := w.maintenanceAffected(ctx, tx, statement)
			if err != nil {
				return 0, err
			}

			total += count
		}
	}

	return total, scanner.Err()
}

func loadMaintenanceFile(ctx context.Context, tx *bench.Tx, table, path string) (err error) {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, file.Close()) }()

	scanner := bufio.NewScanner(file)

	var values []string

	arguments := map[string]any{}
	flush := func() error {
		if len(values) == 0 {
			return nil
		}

		if err := tx.Exec(ctx, "INSERT INTO "+table+" VALUES "+strings.Join(values, ","), arguments); err != nil {
			return err
		}

		values = nil
		arguments = map[string]any{}

		return nil
	}

	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			return err
		}

		columns := strings.Split(strings.TrimSuffix(scanner.Text(), "|"), "|")

		parameters := make([]string, len(columns))
		for i, value := range columns {
			name := "v" + strconv.Itoa(len(arguments))
			parameters[i] = ":" + name

			var cell any
			if value != "" {
				cell = value
			}

			arguments[name] = cell
		}

		values = append(values, "("+strings.Join(parameters, ", ")+")")
		if len(values) == 250 {
			if err := flush(); err != nil {
				return err
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return err
	}

	return flush()
}
