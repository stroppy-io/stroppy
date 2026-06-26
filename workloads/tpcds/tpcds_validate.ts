/**
 * tpcds_validate.ts — SF=1 answer comparator for the `validate_answers` step
 * in tpcds.ts. Answers ship as `workloads/tpcds/answers_sf1.json`, produced by
 * `cmd/tpcds-answers` from the official kit `answer_sets/*.ans` (the SF=1
 * qualification database). Because our generated data is byte-identical to the
 * C dsdgen oracle, the kit answers are a true cross-engine oracle: this step
 * validates BOTH postgres and mysql (unlike tpch, whose answers are pg-only).
 *
 * Comparison is multiset-based: rows are sorted by a rounded key before a
 * positional compare, so the row-ordering differences engines legitimately
 * produce (NULLS FIRST vs LAST, tie order) never read as mismatches. Numeric
 * cells compare within a small tolerance (decimal formatting / float drift);
 * everything else compares exact. Best-effort: deltas are logged, not thrown.
 */
import type { SqlQuery } from "./helpers.ts";

/** Answer payload shape — mirrors cmd/tpcds-answers JSON output. */
export interface AnswerBlock {
  columns: string[];
  rows: string[][];
}

/** Top-level shape of answers_sf1.json. */
export interface AnswersFile {
  version: string;
  source: string;
  answers: Record<string, AnswerBlock>;
}

/** Per-statement comparison result. */
export interface QueryCompareResult {
  query: string;
  status: "ok" | "skipped" | "mismatch" | "error";
  gotRows: number;
  wantRows: number;
  deltas: string[];
  errorMessage?: string;
}

type DbCell = string | number | bigint | boolean | null | Date | undefined;

const TOLERANCE_REL = 1e-3; // 0.1% — covers decimal/float formatting drift
const TOLERANCE_ABS = 0.01; // and sub-cent money rounding

/** Normalize a DB cell to a comparison string (dates → ISO date, null → ""). */
function normalizeCell(v: DbCell): string {
  if (v === null || v === undefined) return "";
  if (typeof v === "string") return v.trim();
  if (typeof v === "boolean") return v ? "t" : "f";
  if (typeof v === "bigint") return v.toString();
  if (typeof v === "number") return Number.isFinite(v) ? String(v) : "";
  if (v instanceof Date) return v.toISOString().slice(0, 10);
  /* eslint-disable-next-line @typescript-eslint/no-base-to-string */
  return String(v);
}

/** Numeric-aware cell match: exact string, else both-numeric within tolerance. */
function cellsMatch(got: string, want: string): boolean {
  if (got === want) return true;
  const gN = parseFloat(got);
  const wN = parseFloat(want);
  if (!Number.isFinite(gN) || !Number.isFinite(wN)) return false;
  // Reject when one side is non-numeric text that parseFloat partially ate.
  if (!isNumeric(got) || !isNumeric(want)) return false;
  const absDelta = Math.abs(gN - wN);
  if (absDelta <= TOLERANCE_ABS) return true;
  return absDelta / Math.max(Math.abs(wN), 1) <= TOLERANCE_REL;
}

const NUMERIC_RE = /^[+-]?(\d+\.?\d*|\.\d+)([eE][+-]?\d+)?$/;
function isNumeric(s: string): boolean {
  return s !== "" && NUMERIC_RE.test(s);
}

/** Sort key for a row: numbers rounded to 2dp so near-equal rows group. */
function rowKey(cells: string[]): string {
  return cells
    .map((c) => (isNumeric(c) ? parseFloat(c).toFixed(2) : c))
    .join("");
}

function sortRows(rows: string[][]): string[][] {
  return rows
    .map((r) => ({ r, k: rowKey(r) }))
    .sort((a, b) => (a.k < b.k ? -1 : a.k > b.k ? 1 : 0))
    .map((x) => x.r);
}

/**
 * Compare a result set against the reference answer as a sorted multiset.
 * Returns a structured delta summary (first few mismatches only).
 */
export function compareQueryResult(
  query: string,
  gotRowsRaw: DbCell[][],
  want: AnswerBlock,
): QueryCompareResult {
  const got = sortRows(gotRowsRaw.map((row) => row.map(normalizeCell)));
  const wantSorted = sortRows(want.rows.map((row) => row.map((c) => (c ?? "").trim())));

  const deltas: string[] = [];
  const budget = Math.max(got.length, wantSorted.length);
  const MAX_DELTAS = 5;
  for (let i = 0; i < budget && deltas.length < MAX_DELTAS; i++) {
    const g = got[i];
    const w = wantSorted[i];
    if (!g) {
      deltas.push(`row ${i}: missing, want=${JSON.stringify(w)}`);
      continue;
    }
    if (!w) {
      deltas.push(`row ${i}: extra, got=${JSON.stringify(g)}`);
      continue;
    }
    const colBudget = Math.max(g.length, w.length);
    for (let c = 0; c < colBudget; c++) {
      if (!cellsMatch(g[c] ?? "", w[c] ?? "")) {
        deltas.push(`row ${i} col ${c}: got=${g[c]} want=${w[c]}`);
        break;
      }
    }
  }

  return {
    query,
    status: deltas.length === 0 && got.length === wantSorted.length ? "ok" : "mismatch",
    gotRows: got.length,
    wantRows: wantSorted.length,
    deltas,
  };
}

/** Minimal driver surface this module needs. */
export interface AnswerRunner {
  queryRows(sql: SqlQuery, args?: Record<string, unknown>, limit?: number): DbCell[][];
}

/** A baked query statement: its marker name and parsed SQL. */
export interface NamedQuery {
  name: string;
  query: SqlQuery;
}

/**
 * Run every baked statement, compare to its reference answer, return per-query
 * results. Statements without a reference answer (e.g. query_39_a/b) are
 * skipped, not failed.
 */
export function runAndCompareAllQueries(
  runner: AnswerRunner,
  queries: NamedQuery[],
  answers: AnswersFile,
): QueryCompareResult[] {
  const results: QueryCompareResult[] = [];
  for (const { name, query } of queries) {
    const want = answers.answers[name];
    if (!want) {
      results.push({ query: name, status: "skipped", gotRows: 0, wantRows: 0, deltas: ["no reference answer"] });
      continue;
    }
    try {
      const rows = runner.queryRows(query, {});
      results.push(compareQueryResult(name, rows, want));
    } catch (e) {
      results.push({
        query: name,
        status: "error",
        gotRows: 0,
        wantRows: want.rows.length,
        deltas: [],
        errorMessage: (e as Error)?.message ?? String(e),
      });
    }
  }
  return results;
}

/**
 * Run each query and return its result as normalized string rows, tagged with
 * the marker name. Used by the cross-DB dump mode: every engine emits the same
 * normalized shape, so an offline diff (cmd/tpcds-diff) can compare engines at
 * any scale without an official answer set. Errors are captured as an empty
 * result plus an `error` field so a single failing query does not abort the dump.
 */
export interface DumpedQuery {
  name: string;
  rows: string[][];
  error?: string;
}

export function captureNormalized(runner: AnswerRunner, queries: NamedQuery[]): DumpedQuery[] {
  const out: DumpedQuery[] = [];
  for (const { name, query } of queries) {
    try {
      const rows = runner.queryRows(query, {});
      out.push({ name, rows: rows.map((row) => row.map(normalizeCell)) });
    } catch (e) {
      out.push({ name, rows: [], error: (e as Error)?.message ?? String(e) });
    }
  }
  return out;
}

/**
 * Execute each query ONCE and return both its normalized dump (for cross-DB
 * diff) and, when `answers` is given, its comparison against the official SF=1
 * answer set. Sharing the single queryRows call avoids running the (heavy)
 * queries twice when both validation and dumping are requested.
 */
export function runAndCapture(
  runner: AnswerRunner,
  queries: NamedQuery[],
  answers: AnswersFile | null,
): { dumps: DumpedQuery[]; results: QueryCompareResult[] } {
  const dumps: DumpedQuery[] = [];
  const results: QueryCompareResult[] = [];
  for (const { name, query } of queries) {
    let rows: DbCell[][] | null = null;
    try {
      rows = runner.queryRows(query, {});
      dumps.push({ name, rows: rows.map((row) => row.map(normalizeCell)) });
    } catch (e) {
      const msg = (e as Error)?.message ?? String(e);
      dumps.push({ name, rows: [], error: msg });
      if (answers?.answers[name]) {
        results.push({ query: name, status: "error", gotRows: 0, wantRows: answers.answers[name].rows.length, deltas: [], errorMessage: msg });
      }
      continue;
    }
    const want = answers?.answers[name];
    if (want) results.push(compareQueryResult(name, rows, want));
  }
  return { dumps, results };
}

/** Pretty-print a comparison summary to stdout. */
export function logSummary(results: QueryCompareResult[]): void {
  let ok = 0, mismatch = 0, skipped = 0, error = 0;
  const lines: string[] = ["===== TPC-DS query validation vs answers_sf1.json ====="];
  for (const r of results) {
    switch (r.status) {
      case "ok":
        ok++;
        lines.push(`  ${r.query.padEnd(14)}: OK      rows=${r.gotRows}`);
        break;
      case "mismatch": {
        mismatch++;
        const preview = r.deltas.slice(0, 3).join("; ") + (r.deltas.length > 3 ? " …" : "");
        lines.push(`  ${r.query.padEnd(14)}: DIFF    rows=${r.gotRows}/${r.wantRows}  ${preview}`);
        break;
      }
      case "skipped":
        skipped++;
        lines.push(`  ${r.query.padEnd(14)}: SKIP    ${r.deltas.join("; ")}`);
        break;
      case "error":
        error++;
        lines.push(`  ${r.query.padEnd(14)}: ERROR   ${r.errorMessage ?? "(no message)"}`);
        break;
    }
  }
  lines.push(`  total=${results.length}  ok=${ok}  diff=${mismatch}  skipped=${skipped}  error=${error}`);
  console.log(lines.join("\n"));
}
