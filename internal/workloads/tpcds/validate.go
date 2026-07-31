package tpcds

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/stroppy-io/stroppy/pkg/bench"
	"github.com/stroppy-io/stroppy/workloads"
)

// SF=1 answer comparator (port of tpcds_validate.ts). Multiset comparison: rows are
// sorted by a rounded key before a positional compare, so legitimate row-ordering
// differences (NULLS FIRST vs LAST, tie order) never read as mismatches. Numeric cells
// compare within a small tolerance; everything else compares exact. Best-effort: deltas
// are logged, not thrown. The generated data is byte-identical to the C dsdgen oracle,
// so the kit answers validate both postgres and mysql.

type answerBlock struct {
	Columns []string   `json:"columns"`
	Rows    [][]string `json:"rows"`
}

type answersFile struct {
	Version string                 `json:"version"`
	Source  string                 `json:"source"`
	Answers map[string]answerBlock `json:"answers"`
}

func loadAnswers() (*answersFile, error) {
	data, err := workloads.ReadPresetFile(preset, "answers_sf1.json")
	if err != nil {
		return nil, err
	}

	var af answersFile
	if err := json.Unmarshal(data, &af); err != nil {
		return nil, err
	}

	return &af, nil
}

const (
	toleranceRel = 1e-3 // 0.1% — covers decimal/float formatting drift
	toleranceAbs = 0.01 // sub-cent money rounding
	maxDeltas    = 5
)

var numericRE = regexp.MustCompile(`^[+-]?(\d+\.?\d*|\.\d+)([eE][+-]?\d+)?$`)

func isNumeric(s string) bool { return s != "" && numericRE.MatchString(s) }

// normalizeCell coerces a DB value to a comparison string (dates → ISO date, nil → "").
func normalizeCell(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(x)
	case []byte:
		return strings.TrimSpace(string(x))
	case bool:
		if x {
			return "t"
		}

		return "f"
	case float64:
		if math.IsInf(x, 0) || math.IsNaN(x) {
			return ""
		}

		return strconv.FormatFloat(x, 'g', -1, 64)
	case int64:
		return strconv.FormatInt(x, 10)
	case int32:
		return strconv.FormatInt(int64(x), 10)
	case int:
		return strconv.Itoa(x)
	case time.Time:
		return x.UTC().Format("2006-01-02")
	default:
		return fmt.Sprint(v)
	}
}

// cellsMatch: exact string match, else both-numeric within tolerance.
func cellsMatch(got, want string) bool {
	if got == want {
		return true
	}

	gN, errG := strconv.ParseFloat(got, 64)

	wN, errW := strconv.ParseFloat(want, 64)
	if errG != nil || errW != nil || math.IsInf(gN, 0) || math.IsInf(wN, 0) {
		return false
	}
	// Reject when one side is non-numeric text that ParseFloat partially ate.
	if !isNumeric(got) || !isNumeric(want) {
		return false
	}

	abs := math.Abs(gN - wN)
	if abs <= toleranceAbs {
		return true
	}

	return abs/math.Max(math.Abs(wN), 1) <= toleranceRel
}

// rowKey is the sort key for a row: numbers rounded to 2dp so near-equal rows group.
func rowKey(cells []string) string {
	parts := make([]string, len(cells))
	for i, c := range cells {
		if isNumeric(c) {
			f, _ := strconv.ParseFloat(c, 64)
			parts[i] = strconv.FormatFloat(f, 'f', 2, 64)
		} else {
			parts[i] = c
		}
	}

	return strings.Join(parts, "")
}

func sortedRows(raw [][]any) [][]string {
	out := make([][]string, len(raw))
	for i, row := range raw {
		cells := make([]string, len(row))
		for c, cv := range row {
			cells[c] = normalizeCell(cv)
		}

		out[i] = cells
	}

	sort.SliceStable(out, func(i, j int) bool { return rowKey(out[i]) < rowKey(out[j]) })

	return out
}

func sortedWant(rows [][]string) [][]string {
	out := make([][]string, len(rows))
	for i, row := range rows {
		cells := make([]string, len(row))
		for c, cv := range row {
			cells[c] = strings.TrimSpace(strings.TrimSpace(cv))
		}

		out[i] = cells
	}

	sort.SliceStable(out, func(i, j int) bool { return rowKey(out[i]) < rowKey(out[j]) })

	return out
}

type compareResult struct {
	query    string
	status   string // ok | mismatch | skipped | error
	gotRows  int
	wantRows int
	deltas   []string
	errMsg   string
}

func compareQuery(query string, got [][]string, want answerBlock) compareResult {
	wantRows := sortedWant(want.Rows)

	var deltas []string

	budget := max(len(got), len(wantRows))
	for i := range budget {
		var g, w []string
		if i < len(got) {
			g = got[i]
		}

		if i < len(wantRows) {
			w = wantRows[i]
		}

		switch {
		case g == nil:
			deltas = append(deltas, fmt.Sprintf("row %d: missing, want=%v", i, w))
		case w == nil:
			deltas = append(deltas, fmt.Sprintf("row %d: extra, got=%v", i, g))
		default:
			colBudget := max(len(g), len(w))
			for c := range colBudget {
				gc, wc := "", ""
				if c < len(g) {
					gc = g[c]
				}

				if c < len(w) {
					wc = w[c]
				}

				if !cellsMatch(gc, wc) {
					deltas = append(deltas, fmt.Sprintf("row %d col %d: got=%s want=%s", i, c, gc, wc))

					break
				}
			}
		}

		if len(deltas) >= maxDeltas {
			break
		}
	}

	status := "ok"
	if len(deltas) > 0 || len(got) != len(wantRows) {
		status = "mismatch"
	}

	return compareResult{query: query, status: status, gotRows: len(got), wantRows: len(wantRows), deltas: deltas}
}

// validateAnswers runs every query against the SF=1 reference answers and logs a summary.
// No-op unless SF=1 and postgres/mysql (the engines the answers were generated to validate).
//
// The queries run inside one transaction so the schema's set_timeout / preconfigure_db
// session SETs (statement cap; enable_nestloop=off + jit=off for pg) bind to the same
// pinned connection as the queries. Without them pg picks nested-loop plans over the
// statistic-less year_total CTE self-joins and individual queries run for tens of minutes.
//
// Each query runs in its own transaction with the SETs re-applied on its pinned
// connection: the driver backs each call with a fresh pooled connection, so session
// SETs do not persist across calls, and a single transaction would let one query's
// error abort every following query (25P02 "transaction is aborted"). Per-query tx
// keeps errors isolated and the planner setup bound to the connection that runs the query.
func validateAnswers(ctx context.Context, b *bench.Bench, schema, queries *bench.SQL, names []string, scaleFactor float64, dt bench.DriverTypeName) {
	lg := b.Logger().Sugar()
	if dt != bench.DriverPostgres && dt != bench.DriverMySQL {
		lg.Infof("[tpcds_validate] skipped: answers_sf1 validates postgres/mysql only; driverType=%s", dt)

		return
	}

	if math.Abs(scaleFactor-1) > 1e-9 && bench.Env("VALIDATE_FORCE", "") == "" {
		lg.Info("[tpcds_validate] skipped: answers_sf1 is SF=1 only (set VALIDATE_FORCE=1 to run anyway)")

		return
	}

	af, err := loadAnswers()
	if err != nil {
		lg.Errorf("[tpcds_validate] failed to load answers: %v", err)

		return
	}

	sets := []string{}
	for _, section := range []string{"set_timeout", "preconfigure_db"} {
		sets = append(sets, schema.Section(section)...)
	}

	iso := bench.IsoReadCommitted
	if dt == bench.DriverPicodata {
		iso = bench.IsoNone // picodata Begin() always errors
	}

	results := make([]compareResult, 0, len(names))
	for _, name := range names {
		want, hasWant := af.Answers[name]

		body, ok := queries.Query("", name)
		if !ok {
			results = append(results, compareResult{query: name, status: "skipped", deltas: []string{"query text missing"}})

			continue
		}

		if !hasWant {
			results = append(results, compareResult{query: name, status: "skipped", deltas: []string{"no reference answer"}})

			continue
		}

		var (
			gotRows [][]any
			qerr    error
		)

		txErr := b.BeginTx(ctx, bench.BeginOpts{Isolation: iso, Name: "tpcds_validate"}, func(tx *bench.TxX) error {
			for _, set := range sets {
				_ = tx.Exec(ctx, set, nil) // best-effort; a failed SET must not abort
			}

			gotRows, qerr = tx.QueryRows(ctx, body, nil)

			return qerr
		})
		if txErr != nil {
			results = append(results, compareResult{query: name, status: "error", wantRows: len(want.Rows), errMsg: txErr.Error()})

			continue
		}

		results = append(results, compareQuery(name, sortedRows(gotRows), want))
	}

	logSummary(b, results)
}

func logSummary(b *bench.Bench, results []compareResult) {
	var ok, mismatch, skipped, errN int

	lines := []string{"===== TPC-DS query validation vs answers_sf1.json ====="}

	for _, r := range results {
		switch r.status {
		case "ok":
			ok++

			lines = append(lines, fmt.Sprintf("  %-12s: OK      rows=%d", r.query, r.gotRows))
		case "mismatch":
			mismatch++

			preview := strings.Join(r.deltas[:min(3, len(r.deltas))], "; ")
			if len(r.deltas) > 3 {
				preview += " …"
			}

			lines = append(lines, fmt.Sprintf("  %-12s: DIFF    rows=%d/%d  %s", r.query, r.gotRows, r.wantRows, preview))
		case "skipped":
			skipped++

			lines = append(lines, fmt.Sprintf("  %-12s: SKIP    %s", r.query, strings.Join(r.deltas, "; ")))
		case "error":
			errN++

			lines = append(lines, fmt.Sprintf("  %-12s: ERROR   %s", r.query, r.errMsg))
		}
	}

	lines = append(lines, fmt.Sprintf("  total=%d  ok=%d  diff=%d  skipped=%d  error=%d", len(results), ok, mismatch, skipped, errN))
	b.Logger().Sugar().Info(strings.Join(lines, "\n"))
}
