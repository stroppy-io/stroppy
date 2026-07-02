package sqlfile

import (
	"os"
	"path/filepath"
	"testing"
)

// corpusRoot is the real v5 SQL corpus this repo carries over verbatim
// (RFC 0001 §11); next/sqlfile/corpus_test.go lives two levels below it.
const corpusRoot = "../../workloads"

func TestCorpus_ParsesEveryFileWithoutError(t *testing.T) {
	var files []string

	err := filepath.WalkDir(corpusRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if !d.IsDir() && filepath.Ext(path) == ".sql" {
			files = append(files, path)
		}

		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", corpusRoot, err)
	}

	if len(files) == 0 {
		t.Fatalf("no .sql files found under %s", corpusRoot)
	}

	for _, path := range files {
		path := path

		t.Run(path, func(t *testing.T) {
			src, err := os.ReadFile(path) //nolint:gosec // test-only, path comes from WalkDir over a fixed corpus dir
			if err != nil {
				t.Fatalf("read: %v", err)
			}

			if _, err := Parse(src); err != nil {
				t.Fatalf("Parse: %v", err)
			}
		})
	}
}

// TestCorpus_TpccPgSections pins the section/query layout AGENTS.md
// documents for workloads/tpcc/pg.sql ("Workload Structure").
func TestCorpus_TpccPgSections(t *testing.T) {
	src, err := os.ReadFile(filepath.Join(corpusRoot, "tpcc", "pg.sql"))
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	f, err := Parse(src)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	wantSections := []string{
		"drop_schema", "create_schema", "set_unlogged", "create_procedures",
		"create_indexes", "set_logged", "create_foreign_keys", "analyze",
		"workload_procs", "workload_tx_new_order", "workload_tx_payment",
		"workload_tx_order_status", "workload_tx_delivery", "workload_tx_stock_level",
	}
	if got := f.SectionNames(); !equalStrings(got, wantSections) {
		t.Fatalf("SectionNames = %v, want %v", got, wantSections)
	}

	if got := names(f.Section("drop_schema")); !equalStrings(got, []string{"drop functions", "drop tables"}) {
		t.Fatalf("drop_schema = %v", got)
	}

	wantSchema := []string{"warehouse", "district", "customer", "history", "new_order", "orders", "order_line", "item", "stock"}
	if got := names(f.Section("create_schema")); !equalStrings(got, wantSchema) {
		t.Fatalf("create_schema = %v", got)
	}

	if got := names(f.Section("create_procedures")); !equalStrings(got, []string{"neword", "payment", "delivery", "ostat", "slev"}) {
		t.Fatalf("create_procedures = %v", got)
	}

	if got := names(f.Section("workload_procs")); !equalStrings(got, []string{"new_order", "payment", "order_status", "delivery", "stock_level"}) {
		t.Fatalf("workload_procs = %v", got)
	}

	// workload_procs calls the stored procs by name via positional SQL
	// function args ("SELECT NEWORD($1, ...)"), not named parameters, but
	// still uses ":name" markers for the driver's own positional binding.
	newOrder, ok := f.Query("workload_procs", "new_order")
	if !ok {
		t.Fatal("workload_procs/new_order not found")
	}

	if got := newOrder.Params(); !equalStrings(got, []string{"w_id", "min_w_id", "max_w_id", "d_id", "c_id", "ol_cnt", "force_rollback"}) {
		t.Fatalf("new_order params = %v", got)
	}

	if got := newOrder.Text(Dollar); got != "SELECT NEWORD($1, $2, $3, $4, $5, $6, $7)" {
		t.Fatalf("new_order Text(Dollar) = %q", got)
	}

	// create_procedures bodies are $$-quoted and opaque: no named params.
	neword, ok := f.Query("create_procedures", "neword")
	if !ok {
		t.Fatal("create_procedures/neword not found")
	}

	if got := neword.Params(); len(got) != 0 {
		t.Fatalf("neword params = %v, want none (body is $$-quoted)", got)
	}

	if got := neword.Text(Dollar); got != neword.Raw {
		t.Fatalf("neword Text(Dollar) should equal Raw verbatim")
	}

	// analyze's single "--=" entry has no name text.
	if got := names(f.Section("analyze")); !equalStrings(got, []string{""}) {
		t.Fatalf("analyze = %v", got)
	}
}
