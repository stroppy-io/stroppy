package tpch

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/stroppy-io/stroppy/pkg/bench"
	"github.com/stroppy-io/stroppy/workloads"
)

// SF=1 answer comparator (port of tpch_validate.ts). Best-effort: logs deltas, never
// aborts — the simplified generation drifts some answer magnitudes beyond spec tolerance,
// so the step proves queries parse/execute/return type-shaped rows rather than gating.

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
	toleranceRel = 0.01 // ±1%
	toleranceAbs = 100  // ±$100
)

// normalizeCell coerces a DB value to a comparison string (mirrors tpch_validate).
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

// cellsMatch: exact string match, else numeric fuzzy (±100 or ±1%).
func cellsMatch(got, want string) bool {
	if got == want {
		return true
	}

	gN, errG := strconv.ParseFloat(got, 64)

	wN, errW := strconv.ParseFloat(want, 64)
	if errG != nil || errW != nil || math.IsInf(gN, 0) || math.IsInf(wN, 0) {
		return false
	}

	abs := math.Abs(gN - wN)
	if abs <= toleranceAbs {
		return true
	}

	denom := math.Max(math.Abs(wN), 1)

	return abs/denom <= toleranceRel
}

type compareResult struct {
	query    string
	status   string // ok | mismatch | skipped | error
	gotRows  int
	wantRows int
	deltas   []string
	errMsg   string
}

func compareQuery(query string, gotRows [][]any, want answerBlock) compareResult {
	var deltas []string

	rowBudget := max(len(gotRows), len(want.Rows))
	for i := range rowBudget {
		var (
			got []any
			w   []string
		)

		if i < len(gotRows) {
			got = gotRows[i]
		}

		if i < len(want.Rows) {
			w = want.Rows[i]
		}

		if got == nil {
			deltas = append(deltas, fmt.Sprintf("row %d: missing, want=%v", i, w))

			continue
		}

		if w == nil {
			ncells := make([]string, len(got))
			for c, cv := range got {
				ncells[c] = normalizeCell(cv)
			}

			deltas = append(deltas, fmt.Sprintf("row %d: extra, got=%v", i, ncells))

			continue
		}

		colBudget := max(len(got), len(w))
		for c := range colBudget {
			g := ""
			if c < len(got) {
				g = normalizeCell(got[c])
			}

			ww := ""
			if c < len(w) {
				ww = strings.TrimSpace(w[c])
			}

			if !cellsMatch(g, ww) {
				deltas = append(deltas, fmt.Sprintf("row %d col %d: got=%s want=%s", i, c, g, ww))
			}
		}
	}

	status := "ok"
	if len(deltas) > 0 {
		status = "mismatch"
	}

	return compareResult{query: query, status: status, gotRows: len(gotRows), wantRows: len(want.Rows), deltas: deltas}
}

// validateAnswers runs all 22 queries against the SF=1 reference answers and logs a
// summary. No-op unless SF=1 and postgres (the only driver the answers were generated
// against). Params are passed raw (no withEndDates): postgres does date math server-side.
func validateAnswers(ctx context.Context, b *bench.Bench, sql *bench.SQL, params map[string]map[string]any, scaleFactor float64, dt bench.DriverTypeName) {
	lg := b.Logger().Sugar()
	if math.Abs(scaleFactor-1) > 1e-9 {
		lg.Info("[tpch_validate] skipped: answers_sf1 is SF=1 only")

		return
	}

	if dt != bench.DriverPostgres {
		lg.Infof("[tpch_validate] skipped: answers_sf1 generated against postgres only; driverType=%s", dt)

		return
	}

	af, err := loadAnswers()
	if err != nil {
		lg.Errorf("[tpch_validate] failed to load answers: %v", err)

		return
	}

	var results []compareResult

	for _, name := range queryNames {
		body, ok := sql.Query(name, "body")

		want, hasWant := af.Answers[name]
		if !ok {
			results = append(results, compareResult{query: name, status: "skipped", deltas: []string{"query text missing"}})

			continue
		}

		if !hasWant {
			results = append(results, compareResult{query: name, status: "skipped", deltas: []string{"no reference answer"}})

			continue
		}

		rows, qerr := b.QueryRows(ctx, body, params[name])
		if qerr != nil {
			results = append(results, compareResult{query: name, status: "error", wantRows: len(want.Rows), errMsg: qerr.Error()})

			continue
		}

		results = append(results, compareQuery(name, rows, want))
	}

	logSummary(b, results)
}

func logSummary(b *bench.Bench, results []compareResult) {
	var ok, mismatch, skipped, errN int

	lines := []string{"===== TPC-H query validation vs answers_sf1.json ====="}

	for _, r := range results {
		switch r.status {
		case "ok":
			ok++

			lines = append(lines, fmt.Sprintf("  %-4s: OK      rows=%d (want %d)", r.query, r.gotRows, r.wantRows))
		case "mismatch":
			mismatch++

			preview := strings.Join(r.deltas[:min(3, len(r.deltas))], "; ")
			if len(r.deltas) > 3 {
				preview += fmt.Sprintf(" … (+%d more)", len(r.deltas)-3)
			}

			lines = append(lines, fmt.Sprintf("  %-4s: DIFF    rows=%d/%d  %s", r.query, r.gotRows, r.wantRows, preview))
		case "skipped":
			skipped++

			lines = append(lines, fmt.Sprintf("  %-4s: SKIP    %s", r.query, strings.Join(r.deltas, "; ")))
		case "error":
			errN++

			lines = append(lines, fmt.Sprintf("  %-4s: ERROR   %s", r.query, r.errMsg))
		}
	}

	lines = append(lines, fmt.Sprintf("  total=%d  ok=%d  diff=%d  skipped=%d  error=%d", len(results), ok, mismatch, skipped, errN))
	b.Logger().Sugar().Info(strings.Join(lines, "\n"))
}
