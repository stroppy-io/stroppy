import { Options } from "k6/options";
import exec from "k6/execution";
import { Teardown, GenerateTpcdsQueries } from "k6/x/stroppy";
import { DriverX, Step, ENV, GlobalOnce, declareDriverSetup } from "./helpers.ts";
import { parse_sql, parse_sql_with_sections } from "./parse_sql.js";
import type { SqlQuery } from "./helpers.ts";
import {
  runAndCapture,
  logSummary,
  type AnswersFile,
  type NamedQuery,
} from "./tpcds_validate.ts";

// Data generation: the ported dsdgen generator owns it; we pass table + scale.
const SCALE_FACTOR = Number(
  ENV("SCALE_FACTOR", "1", "TPC-DS scale factor; fractional allowed for smoke tests"),
);
const LOAD_WORKERS = ENV(
  "LOAD_WORKERS",
  0,
  "Load-time worker count per table (0 = framework default)",
) as number;

if (!Number.isFinite(SCALE_FACTOR) || SCALE_FACTOR <= 0) {
  throw new Error(`SCALE_FACTOR must be a positive number, got ${SCALE_FACTOR}`);
}

// A full load + single query pass at large scale far exceeds k6's default 10m
// cap, so the workload sets its own. Override with MAX_DURATION if needed.
const MAX_DURATION = ENV("MAX_DURATION", "24h", "Max wall-clock for the run (k6 duration)");

// Table load order: dimensions and static tables first, fan-out fact tables
// last (each returns table after its parent sales table).
const TPCDS_TABLES = [
  "income_band", "ship_mode", "reason", "household_demographics",
  "customer_demographics", "date_dim", "time_dim", "warehouse",
  "web_page", "web_site", "catalog_page", "customer_address",
  "customer", "call_center", "store", "promotion", "item", "inventory",
  "store_sales", "store_returns", "catalog_sales", "catalog_returns",
  "web_sales", "web_returns",
];

// STREAMS > 1 runs a Throughput Test: that many concurrent query streams (TPC-DS
// Clause 7.1.9), one per VU, each executing all 99 queries in its own
// permutation. STREAMS <= 1 is the single-stream Power Test (Clause 7.1.10).
const STREAMS = Number(
  ENV("STREAMS", "1", "Concurrent throughput query streams (1 = single power-test stream)"),
);
const THROUGHPUT = Number.isFinite(STREAMS) && STREAMS > 1;

// One iteration per stream. Declared as a scenario (not the vus/iterations
// shorthand) so maxDuration can lift k6's 10m default for large-scale loads.
export const options: Options = {
  scenarios: {
    tpcds: {
      executor: "shared-iterations",
      vus: THROUGHPUT ? STREAMS : 1,
      iterations: THROUGHPUT ? STREAMS : 1,
      maxDuration: MAX_DURATION,
    },
  },
};

const driverConfig = declareDriverSetup(0, {
  url: "postgres://postgres:postgres@localhost:5432",
  driverType: "postgres",
});

const driver = DriverX.create().setup(driverConfig);

// The 99 TPC-DS queries, generated per dialect from the official query templates
// at the canonical qualification parameters (see workloads/tpcds/README or the
// dsqgen port). Picked by driverType; SQL_FILE overrides. Dialects without their
// own file fall back to pg.sql.
const _sqlByDriver: Record<string, string> = {
  postgres: "./pg.sql",
  mysql: "./mysql.sql",
};
const _schemaByDriver: Record<string, string> = {
  postgres: "./schema.pg.sql",
  mysql: "./schema.mysql.sql",
};
const SQL_FILE =
  ENV("SQL_FILE", "", "Path to TPC-DS query SQL file (defaults per driverType)") ||
  _sqlByDriver[driverConfig.driverType!] ||
  "./pg.sql";
const SCHEMA_FILE =
  _schemaByDriver[driverConfig.driverType!] || "./schema.pg.sql";

// Query source. Default: the baked canonical (qualification) query set for the
// driver. If QUERY_STREAM is set, generate that stream's parameters in-process
// (no offline step) — valid, scale-correct, varied per seed.
const QUERY_STREAM = ENV(
  "QUERY_STREAM",
  "",
  "Generate query stream N in-process (empty = baked canonical set)",
);
const QUERY_SEED = Number(
  ENV("QUERY_SEED", "19620718", "RNG seed for generated query streams"),
);

// Answer validation runs only for the baked qualification set at SF=1 (the
// only scale the kit ships answers for). The kit answers are byte-oracle
// derived, so they validate any engine — here postgres and mysql.
// VALIDATE_FORCE=1 runs the validation path at any scale (answers stay SF=1, so
// rows will DIFF at other scales) — a fast way to exercise the comparator.
const VALIDATE_FORCE = ENV("VALIDATE_FORCE", "", "Force answer validation at any scale") !== "";

// ANSWER_DUMP emits each query's normalized result to the run log (one line
// per query, prefixed __TPCDS_DUMP__) so cmd/tpcds-diff can compare engines
// against each other at any scale (the cross-DB / pg-oracle check).
const ANSWER_DUMP = ENV("ANSWER_DUMP", "", "Dump normalized query results to the log for cross-DB diff") !== "";
const VALIDATE_ANSWERS =
  !THROUGHPUT &&
  QUERY_STREAM === "" &&
  (VALIDATE_FORCE || Math.abs(SCALE_FACTOR - 1) < 1e-9) &&
  (driverConfig.driverType === "postgres" || driverConfig.driverType === "mysql");

// Schema DDL (one "create_schema" section), read at module init.
const schema = parse_sql_with_sections(open(SCHEMA_FILE));

// SF=1 reference answers, read at init only when validation will run.
const answersSf1: AnswersFile | null = VALIDATE_ANSWERS
  ? (JSON.parse(open("./answers_sf1.json")) as AnswersFile)
  : null;

// Baked canonical query set, read at init only when neither throughput nor an
// explicit QUERY_STREAM is requested (open() is allowed only during init).
const baked =
  !THROUGHPUT && QUERY_STREAM === "" ? parse_sql(open(SQL_FILE)) : null;

// resolveQueries returns this VU's query list. Throughput: VU N runs stream N
// (in-process generated + permuted). Single QUERY_STREAM: that stream. Otherwise
// the baked canonical set. Memoized per VU.
let myQueries: Array<{ name: string }> | null = null;
function resolveQueries(): Array<{ name: string }> {
  if (myQueries) return myQueries;
  if (baked) {
    myQueries = baked() as Array<{ name: string }>;
    return myQueries;
  }
  const streamIdx = THROUGHPUT
    ? exec.vu.idInTest - 1
    : Number(QUERY_STREAM || "0");
  myQueries = GenerateTpcdsQueries(
    driverConfig.driverType ?? "postgres",
    SCALE_FACTOR,
    QUERY_SEED,
    streamIdx,
  );
  return myQueries;
}

// prepareDatabase creates the schema, then generates and bulk-loads every table
// with the ported dsdgen generator. Runs once per process via GlobalOnce.
function prepareDatabase(): void {
  Step("create_schema", () => {
    const stmts = schema("create_schema");
    if (stmts) {
      stmts.forEach((q) => driver.exec(q, {}));
    }
  });

  Step("load_data", () => {
    // Ported dsdgen: the Go side owns generation; pass table + scale factor.
    for (const table of TPCDS_TABLES) {
      driver.insertTpcds(table, SCALE_FACTOR, LOAD_WORKERS);
    }
  });

  // Indexes are built AFTER the bulk load (one-shot build is far cheaper than
  // maintaining them per-insert). Without them the unindexed multi-way joins
  // are unrunnable at scale, especially on MySQL's nested-loop joins.
  Step("create_indexes", () => {
    const stmts = schema("create_indexes");
    if (stmts) {
      stmts.forEach((q) => driver.exec(q, {}));
    }
  });

  // Bulk load leaves the planner with no statistics; without ANALYZE pg/mysql
  // pick catastrophic plans (e.g. O(n²) correlated nested loops) on big queries.
  Step("analyze", () => {
    const dt = driverConfig.driverType;
    if (dt === "postgres") {
      driver.exec("ANALYZE" as unknown as { name: string }, {});
    } else if (dt === "mysql") {
      for (const table of TPCDS_TABLES) {
        driver.exec(`ANALYZE TABLE ${table}` as unknown as { name: string }, {});
      }
    }
  });

  Step.begin("workload");
}

export function setup(): void {
  return;
}

export default function (): void {
  // Load runs once across all VUs (process-global); concurrent throughput VUs
  // block here until the single loader finishes, then each runs its stream.
  GlobalOnce("tpcds.prepare", prepareDatabase);

  // Power/throughput query execution. When validating or dumping, the steps
  // below execute each query (via queryRows) instead, so skip this redundant pass.
  if (!answersSf1 && !ANSWER_DUMP) {
    const stepName = THROUGHPUT ? `queries_stream_${exec.vu.idInTest}` : "queries";
    Step(stepName, () => {
      resolveQueries().forEach((query) => {
        driver.exec(query, {});
      });
    });
  }

  const named = (): NamedQuery[] =>
    resolveQueries().map((q) => ({
      name: (q as { name: string }).name,
      query: q as unknown as SqlQuery,
    }));

  // Safety: cap any single query so a pathological plan can't wedge the run.
  // A capped query throws → captured as ERR: in the dump, run continues.
  const QUERY_CAP_MS = Number(ENV("QUERY_CAP_MS", "180000", "Per-query timeout (ms) in validate/dump"));
  if (answersSf1 || ANSWER_DUMP) {
    try {
      if (driverConfig.driverType === "postgres") {
        driver.exec(`SET statement_timeout = '${QUERY_CAP_MS}'` as unknown as SqlQuery, {});
      } else if (driverConfig.driverType === "mysql") {
        driver.exec(`SET SESSION max_execution_time = ${QUERY_CAP_MS}` as unknown as SqlQuery, {});
      }
    } catch (_e) { /* best-effort */ }
  }

  // Run each query once; feed both the official SF=1 compare (answersSf1) and
  // the cross-DB dump (ANSWER_DUMP) from the same execution.
  if (answersSf1 || ANSWER_DUMP) {
    Step("validate_answers", () => {
      const { dumps, results } = runAndCapture(driver, named(), answersSf1);
      if (ANSWER_DUMP) {
        for (const d of dumps) {
          console.log(`__TPCDS_DUMP__\t${d.name}\t${d.error ? "ERR:" + d.error : JSON.stringify(d.rows)}`);
        }
      }
      if (answersSf1) logSummary(results);
    });
  }
}

export function teardown(): void {
  Step.end("workload");
  Teardown();
}
