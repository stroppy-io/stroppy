package tpcb

import (
	"strings"
	"testing"

	"github.com/stroppy-io/stroppy/pkg/bench"
)

// tpcbSQL assembles a minimal TPC-B SQL document with both required setup
// sections and the five named transaction queries. missing names one
// (section, query) pair to omit so a test can assert validateSQL names it.
func tpcbSQL(missing struct{ section, query string }) string {
	var b strings.Builder

	if missing.section != "drop_schema" {
		b.WriteString("--+ drop_schema\n--=\nDROP TABLE t;\n")
	}

	if missing.section != "create_schema" {
		b.WriteString("--+ create_schema\n--=\nCREATE TABLE t (a INT);\n")
	}

	b.WriteString("--+ workload_tx_tpcb\n")
	for _, q := range requiredTxQueries {
		if missing.section == q.section && missing.query == q.query {
			continue
		}

		b.WriteString("--= " + q.query + "\nSELECT 1;\n")
	}

	return b.String()
}

func TestValidateSQLAcceptsCompleteFile(t *testing.T) {
	if err := validateSQL(bench.ParseSQL(tpcbSQL(struct{ section, query string }{}))); err != nil {
		t.Fatalf("validateSQL(complete) = %v, want nil", err)
	}
}

func TestValidateSQLNamesEveryMissingRequiredQuery(t *testing.T) {
	for _, q := range requiredTxQueries {
		t.Run(q.query, func(t *testing.T) {
			err := validateSQL(bench.ParseSQL(tpcbSQL(q)))
			if err == nil {
				t.Fatalf("validateSQL missing %s/%s = nil, want error", q.section, q.query)
			}

			if !strings.Contains(err.Error(), q.section+"/"+q.query) {
				t.Fatalf("validateSQL error %q does not name %s/%s", err, q.section, q.query)
			}
		})
	}
}

func TestValidateSQLNamesMissingRequiredSection(t *testing.T) {
	for _, name := range requiredSetupSections {
		t.Run(name, func(t *testing.T) {
			err := validateSQL(bench.ParseSQL(tpcbSQL(struct{ section, query string }{section: name})))
			if err == nil {
				t.Fatalf("validateSQL missing %q = nil, want error", name)
			}

			if !strings.Contains(err.Error(), name) {
				t.Fatalf("validateSQL error %q does not name section %q", err, name)
			}
		})
	}
}

// tpcbSQLWithEmptyQuery assembles a TPC-B document whose named query for empty
// is declared with a `--= name` marker but no statement body.
func tpcbSQLWithEmptyQuery(empty struct{ section, query string }) string {
	var b strings.Builder

	b.WriteString("--+ drop_schema\n--=\nDROP TABLE t;\n")
	b.WriteString("--+ create_schema\n--=\nCREATE TABLE t (a INT);\n")
	b.WriteString("--+ workload_tx_tpcb\n")

	for _, q := range requiredTxQueries {
		b.WriteString("--= " + q.query + "\n")
		if empty.section == q.section && empty.query == q.query {
			continue // declared but empty: the next marker follows immediately
		}

		b.WriteString("SELECT 1;\n")
	}

	return b.String()
}

func TestValidateSQLRejectsEmptyQueryBody(t *testing.T) {
	for _, q := range requiredTxQueries {
		t.Run(q.query, func(t *testing.T) {
			err := validateSQL(bench.ParseSQL(tpcbSQLWithEmptyQuery(q)))
			if err == nil {
				t.Fatalf("validateSQL empty %s/%s = nil, want error", q.section, q.query)
			}

			if !strings.Contains(err.Error(), "empty query "+q.section+"/"+q.query) {
				t.Fatalf("validateSQL error %q does not name empty %s/%s", err, q.section, q.query)
			}
		})
	}
}
