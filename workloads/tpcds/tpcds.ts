import { Options } from "k6/options";
import exec from "k6/execution";
import { Teardown, GenerateTpcdsQueries } from "k6/x/stroppy";
import { DriverX, Step, execEachLogged, ENV, GlobalOnce, declareDriverSetup, declareScenario } from "./helpers.ts";
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
const POOL_SIZE = ENV("POOL_SIZE", 50, "Connection pool size");

if (!Number.isFinite(SCALE_FACTOR) || SCALE_FACTOR <= 0) {
  throw new Error(`SCALE_FACTOR must be a positive number, got ${SCALE_FACTOR}`);
}

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
  scenarios: declareScenario("tpcds", {
    vus: THROUGHPUT ? STREAMS : 1,
    iterations: THROUGHPUT ? STREAMS : 1,
  }),
};

const driverConfig = declareDriverSetup(0, {
  url: "postgres://postgres:postgres@localhost:5432",
  driverType: "postgres",
  pool: { maxConns: POOL_SIZE, minConns: POOL_SIZE },
});

const driver = DriverX.create().setup(driverConfig);

// PostgreSQL only: flip to UNLOGGED for a WAL-free bulk load of the 24 tables,
// back to LOGGED after. Disable with PG_UNLOGGED=false.
const PG_UNLOGGED = ENV("PG_UNLOGGED", "true", "pg only: bulk-load with UNLOGGED tables, flip back to LOGGED after") !== "false";
const useUnlogged = PG_UNLOGGED && driverConfig.driverType === "postgres";

// The 99 TPC-DS queries, generated per dialect from the official query templates
// at the canonical qualification parameters (see workloads/tpcds/README or the
// dsqgen port). Picked by driverType; SQL_FILE overrides. Dialects without their
// own file fall back to pg.sql.
const _sqlByDriver: Record<string, string> = {
  postgres: "./pg.sql",
  mysql: "./mysql.sql",
  ydb: "./ydb.sql",
};
const _schemaByDriver: Record<string, string> = {
  postgres: "./schema.pg.sql",
  mysql: "./schema.mysql.sql",
  ydb: "./schema.ydb.sql",
};

// YDB storage layout. Default 'row': YDB row store keeps the full analytic
// query surface (window functions, grouping) that the column store still
// restricts. 'column' selects the OLAP column-store schema for scan-heavy runs.
const YDB_STORE_MODE = ENV(
  "YDB_STORE_MODE",
  "row",
  "ydb only: 'row' (default, full query surface) or 'column' (OLAP column store)",
);
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

// The in-process stream generator (GenerateTpcdsQueries) emits ANSI/pg and
// MySQL text; it cannot express the structural YQL rewrites the baked ydb.sql
// carries (named subqueries for CTEs, correlation-qualified GROUP BY). So ydb
// runs only the baked query set (power test); reject the generated-stream
// modes with a clear message instead of an opaque generator error.
if (driverConfig.driverType === "ydb" && (THROUGHPUT || QUERY_STREAM !== "")) {
  throw new Error(
    "[tpcds] ydb supports the baked query set (power test) only; STREAMS>1 and " +
      "QUERY_STREAM need the in-process generator, which does not target YQL yet.",
  );
}

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

// SF=1 reference answers, read at init only when validation will run. Guard
// the empty string the probe stubs open() with, so probe can introspect.
function readAnswersSf1(): AnswersFile | null {
  const raw = open("./answers_sf1.json");
  if (!raw) return null;
  return JSON.parse(raw) as AnswersFile;
}
const answersSf1: AnswersFile | null = VALIDATE_ANSWERS ? readAnswersSf1() : null;

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
  // Drop in reverse load order (fact tables before their dimensions). CASCADE is
  // required for idempotent re-runs: a prior run leaves dependents (e.g. views or
  // any cross-table object) that make a plain DROP fail with SQLSTATE 2BP01
  // "cannot drop table ... because other objects depend on it". MySQL accepts and
  // ignores the CASCADE keyword, so the same statement is portable.
  Step("drop_schema", () => {
    if (driverConfig.driverType === "ydb") {
      // YDB has no CASCADE keyword; drop from the schema file's drop_schema
      // section (per-table `DROP TABLE IF EXISTS`, reverse load order).
      execEachLogged(schema("drop_schema"), (q) => driver.exec(q, {}));
      return;
    }
    for (const table of [...TPCDS_TABLES].reverse()) {
      driver.exec(`DROP TABLE IF EXISTS ${table} CASCADE` as unknown as { name: string }, {});
    }
  });

  Step("create_schema", () => {
    // YDB ships two storage layouts as separate sections; every other driver
    // has a single create_schema section.
    const section =
      driverConfig.driverType === "ydb" && YDB_STORE_MODE === "column"
        ? "create_schema_column"
        : "create_schema";
    const stmts = schema(section);
    if (stmts) {
      stmts.forEach((q) => driver.exec(q, {}));
    }
  });

  // pg-only: flip the 24 tables to UNLOGGED for a WAL-free bulk load; set_logged
  // restores durability after. Driven from the TPCDS_TABLES list (like ANALYZE
  // below) rather than per-table SQL, since the schema lives in schema.*.sql.
  if (useUnlogged) {
    Step("set_unlogged", () => execEachLogged(
      TPCDS_TABLES,
      (table) => driver.exec(`ALTER TABLE ${table} SET UNLOGGED` as unknown as { name: string }, {}),
      (table) => `SET UNLOGGED ${table}`,
    ));
  }

  Step("load_data", () => {
    // Ported dsdgen: the Go side owns generation; pass table + scale factor.
    for (const table of TPCDS_TABLES) {
      driver.insertTpcds(table, SCALE_FACTOR, LOAD_WORKERS);
    }
  });

  // Indexes are built AFTER the bulk load (one-shot build is far cheaper than
  // maintaining them per-insert, and cheaper still while tables are UNLOGGED).
  // Without them the unindexed multi-way joins are unrunnable at scale,
  // especially on MySQL's nested-loop joins.
  Step("create_indexes", () => execEachLogged(schema("create_indexes"), (q) => driver.exec(q, {})));

  if (useUnlogged) {
    Step("set_logged", () => execEachLogged(
      TPCDS_TABLES,
      (table) => driver.exec(`ALTER TABLE ${table} SET LOGGED` as unknown as { name: string }, {}),
      (table) => `SET LOGGED ${table}`,
    ));
  }

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
}

export default function (): void {
  // Load runs once across all VUs (process-global); concurrent throughput VUs
  // block here until the single loader finishes, then each runs its stream.
  GlobalOnce("tpcds.prepare", prepareDatabase);

  // Power/throughput query execution. When validating or dumping, the steps
  // below execute each query (via queryRows) instead, so skip this redundant pass.
  if (!answersSf1 && !ANSWER_DUMP) {
    // The measured pass. Single gatable step name across power and throughput
    // (streams stay distinguishable by VU tag) so the two-run flow works:
    // `--no-steps workload` (prep) then `--steps workload` (measure).
    Step("workload", () => {
      resolveQueries().forEach((query) => {
        driver.exec(query, {});
      });
    }, { silent: true });
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
  Teardown();
}
