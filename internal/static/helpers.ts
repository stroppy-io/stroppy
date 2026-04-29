import { Counter, Rate, Trend } from "k6/metrics";
export { Counter, Rate, Trend };
import { test } from "k6/execution"
import encoding from "k6/x/encoding";
globalThis.TextEncoder = encoding.TextEncoder;
globalThis.TextDecoder = encoding.TextDecoder;

import {
  NewDriver,
  NotifyStep,
  DeclareEnv,
  Once,
  Driver,
  Tx,
  QueryStats,
  QueryResult,
} from "k6/x/stroppy";
import {
  DriverConfig,
  InsertSpec as DatagenInsertSpec,
  DriverConfig_ErrorMode,
  DriverConfig_DriverType,
  DriverConfig_PostgresConfig,
  DriverConfig_SqlConfig,
  InsertMethod,
  StroppyRun_Status,
  TxIsolationLevel,
} from "./stroppy.pb.js";

import { ParsedQuery } from "./parse_sql.js";

declare const __ENV: Record<string, string>;

type AutoDefault = "<auto>";

export function ENV(env: string | string[], default_: AutoDefault, description?: string): string | undefined;
export function ENV(env: string | string[], default_?: string, description?: string): string;
export function ENV(env: string | string[], default_?: number, description?: string): number;
export function ENV(env: string | string[], default_?: string | number, description?: string): string | number | undefined {
  const names = Array.isArray(env) ? env : [env];
  const isAuto = default_ === ENV.auto;
  DeclareEnv(names, isAuto ? "<auto>" : String(default_ ?? ""), description ?? "");
  const asNum = typeof default_ === "number";
  for (const name of names) {
    const val = __ENV[name];
    if (val !== undefined && val !== "") return asNum ? Number(val) : val;
  }
  if (isAuto) return undefined;
  return default_ as string | number;
}
ENV.auto = "<auto>" as AutoDefault;


export type ErrorModeName = "silent" | "log" | "throw" | "fail" | "abort";

const errorModeMap: Record<ErrorModeName, DriverConfig_ErrorMode> = {
  silent: DriverConfig_ErrorMode.ERROR_MODE_SILENT,
  log: DriverConfig_ErrorMode.ERROR_MODE_LOG,
  throw: DriverConfig_ErrorMode.ERROR_MODE_THROW,
  fail: DriverConfig_ErrorMode.ERROR_MODE_FAIL,
  abort: DriverConfig_ErrorMode.ERROR_MODE_ABORT,
};

export type DriverTypeName = "postgres" | "mysql" | "picodata" | "ydb" | "noop" | "csv";

const driverTypeMap: Record<DriverTypeName, DriverConfig_DriverType> = {
  postgres: DriverConfig_DriverType.DRIVER_TYPE_POSTGRES,
  mysql: DriverConfig_DriverType.DRIVER_TYPE_MYSQL,
  picodata: DriverConfig_DriverType.DRIVER_TYPE_PICODATA,
  ydb: DriverConfig_DriverType.DRIVER_TYPE_YDB,
  noop: DriverConfig_DriverType.DRIVER_TYPE_NOOP,
  csv: DriverConfig_DriverType.DRIVER_TYPE_CSV,
};

const _envErrorMode = ENV("STROPPY_ERROR_MODE", undefined, 
"(default: by config, else 'log') error handling mode: silent, log, throw, fail, abort",
) as ErrorModeName | undefined;

export type TxIsolationName =
  | "read_uncommitted"
  | "read_committed"
  | "repeatable_read"
  | "serializable"
  | "db_default"
  | "conn"
  | "none";

const txIsolationMap: Record<TxIsolationName, TxIsolationLevel> = {
  db_default: TxIsolationLevel.UNSPECIFIED,
  read_uncommitted: TxIsolationLevel.READ_UNCOMMITTED,
  read_committed: TxIsolationLevel.READ_COMMITTED,
  repeatable_read: TxIsolationLevel.REPEATABLE_READ,
  serializable: TxIsolationLevel.SERIALIZABLE,
  conn: TxIsolationLevel.CONNECTION_ONLY,
  none: TxIsolationLevel.NONE,
};

export type InsertMethodName = "plain_query" | "plain_bulk" | "native";

const insertMethodMap: Record<InsertMethodName, InsertMethod> = {
  plain_query: InsertMethod.PLAIN_QUERY,
  plain_bulk: InsertMethod.PLAIN_BULK,
  native: InsertMethod.NATIVE,
};

const insertMetric = new Trend("insert_duration", true);
const insertErrRateMetric = new Rate("insert_error_rate");
const runQueryMetric = new Trend("run_query_duration", true);
const runQueryCounterMetric = new Counter("run_query_count");
const runQueryErrRateMetric = new Rate("run_query_error_rate");
const txTotalDurationMetric = new Trend("tx_total_duration", true);
const txCleanDurationMetric = new Trend("tx_clean_duration", true);
const txCommitRateMetric = new Rate("tx_commit_rate");
const txErrRateMetric = new Rate("tx_error_rate");
const txQueriesPerTxMetric = new Trend("tx_queries_per_tx", false);

export interface TaggedQuery {
  sql: string | ParsedQuery;
  tags?: Record<string, string>;
}

export type SqlQuery = string | ParsedQuery | TaggedQuery;

function resolveSqlQuery(arg: SqlQuery): {
  sql: string;
  tags: Record<string, string> | undefined;
} {
  // Plain SQL string
  if (typeof arg === "string") return { sql: arg, tags: undefined };

  // ParsedQuery (has name, type, params — check before TaggedQuery since both have "sql")
  if ("params" in arg) {
    const pq = arg as ParsedQuery;
    return { sql: pq.sql, tags: { name: pq.name, type: pq.type } };
  }

  // TaggedQuery
  const inner = arg as TaggedQuery;
  const parsed =
    typeof inner.sql === "string" ? inner.sql : (inner.sql as ParsedQuery);
  const baseTags =
    typeof parsed === "string"
      ? undefined
      : { name: parsed.name, type: parsed.type };
  return {
    sql: typeof parsed === "string" ? parsed : parsed.sql,
    tags: inner.tags ? { ...baseTags, ...inner.tags } : baseTags,
  };
}

// Sugar interface for convenient query patterns.
// Reusable across DriverX, TxX.
// All methods accept a raw SQL string, a ParsedQuery, or a TaggedQuery.
// All methods throw on query execution error.
export interface QueryAPI {
  exec(sql: SqlQuery, args?: Record<string, any>): QueryStats;
  queryRows(sql: SqlQuery, args?: Record<string, any>, limit?: number): any[][];
  queryRow(sql: SqlQuery, args?: Record<string, any>): any[] | undefined;
  queryValue<T = any>(sql: SqlQuery, args?: Record<string, any>): T | undefined;
  queryCursor(sql: SqlQuery, args?: Record<string, any>): QueryResult | undefined;
}

type RunQueryFn = (sql: string, args: Record<string, any>) => QueryResult;

function handleError(mode: ErrorModeName, e: unknown, tags?: Record<string, string>): void {
  if (mode !== "silent") {
    console.error(`[stroppy] query error${tags ? ` [${Object.values(tags).join(",")}]` : ""}: ${e}`);
  } 
  if (mode === "throw") {
    throw e;
  } else if (mode === "fail") {
    test.fail(`failed due to sql error: ${e}`)
  } else if (mode === "abort") {
    test.abort(`aborted due to sql error: ${e}`)
  }
}

function createQueryAPI(rawRunQuery: RunQueryFn, getErrorMode: () => ErrorModeName, isTx = false): QueryAPI {
  function run(sql: SqlQuery, args: Record<string, any>): QueryResult | undefined {
    const { sql: s, tags } = resolveSqlQuery(sql);
    try {
      const result = rawRunQuery(s, args);
      // .seconds() returns a float — multiply by 1000 for sub-ms precision.
      // .milliseconds() truncates to int64 and reports 0 for sub-ms queries.
      runQueryMetric.add(result.stats.elapsed.seconds() * 1000, tags);
      runQueryErrRateMetric.add(0, tags);
      runQueryCounterMetric.add(1, tags);
      return result;
    } catch (e) {
      runQueryErrRateMetric.add(1, tags);
      if (isTx) { throw e }
      handleError(getErrorMode(), e, tags);
      return undefined;
    }
  }

  return {
    exec(sql: SqlQuery, args?: Record<string, any>): QueryStats {
      const result = run(sql, args ?? {});
      if (!result) return undefined as unknown as QueryStats;
      result.rows.close();
      return result.stats;
    },

    queryRows(
      sql: SqlQuery,
      args?: Record<string, any>,
      limit?: number,
    ): any[][] {
      const result = run(sql, args ?? {});
      if (!result) return [];
      return result.rows.readAll(limit ?? 0);
    },

    queryRow(sql: SqlQuery, args?: Record<string, any>): any[] | undefined {
      const result = run(sql, args ?? {});
      if (!result) return undefined;
      const row = result.rows.next() ? result.rows.values() : undefined;
      result.rows.close();
      return row;
    },

    queryValue<T = any>(
      sql: SqlQuery,
      args?: Record<string, any>,
    ): T | undefined {
      const result = run(sql, args ?? {});
      if (!result) return undefined;
      if (!result.rows.next()) {
        result.rows.close();
        return undefined;
      }
      const vals = result.rows.values();
      result.rows.close();
      return vals?.length ? (vals[0] as T) : undefined;
    },

    queryCursor(sql: SqlQuery, args?: Record<string, any>): QueryResult | undefined {
      const result = run(sql, args ?? {});
      if (!result) return undefined;
      return result as QueryResult;
    },
  };
}

export class TxX implements QueryAPI {
  private tx: Tx;
  private q: QueryAPI;
  readonly isolation: TxIsolationName;
  readonly name: string | undefined;
  private _startTime: number;
  private _cleanDuration: number = 0;
  private _queryCount: number = 0;

  exec!: QueryAPI["exec"];
  queryRows!: QueryAPI["queryRows"];
  queryRow!: QueryAPI["queryRow"];
  queryValue!: QueryAPI["queryValue"];
  queryCursor!: QueryAPI["queryCursor"];

  constructor(tx: Tx, isolation: TxIsolationName, getErrorMode: () => ErrorModeName, name?: string) {
    this.tx         = tx;
    this.isolation  = isolation;
    this.name       = name;
    this._startTime = Date.now();
    this.q = createQueryAPI(
      (sql, args) => {
        this._queryCount++; 
        const res = tx.runQuery(sql, args);
        this._cleanDuration += res.stats.elapsed.seconds() * 1000;
        return res;
      },
      getErrorMode,
      true,
    );
    this.exec        = this.q.exec;
    this.queryRows   = this.q.queryRows;
    this.queryRow    = this.q.queryRow;
    this.queryValue  = this.q.queryValue;
    this.queryCursor = this.q.queryCursor;
  }

  private _tags(action?: "commit" | "rollback"): Record<string, string> {
    const tags: Record<string, string> = {};
    if (action)         tags.tx_action    = action;
    if (this.name)      tags.tx_name      = this.name;
    if (this.isolation) tags.tx_isolation = this.isolation;
    return tags;
  }

  commit(): void {
    const elapsed = Date.now() - this._startTime;
    this.tx.commit();
    const tags = this._tags("commit");
    txTotalDurationMetric.add(elapsed, tags);
    txCleanDurationMetric.add(this._cleanDuration, tags);
    txCommitRateMetric.add(1, tags);
    txQueriesPerTxMetric.add(this._queryCount, tags);
  }

  rollback(): void {
    const elapsed = Date.now() - this._startTime;
    const tags = this._tags("rollback");
    txTotalDurationMetric.add(elapsed, tags);
    txCleanDurationMetric.add(this._cleanDuration, tags);
    txCommitRateMetric.add(0, tags);
    txQueriesPerTxMetric.add(this._queryCount, tags);
    this.tx.rollback();
  }
}

/** Unified pool configuration sugar. Mapped to postgres:{} or sql:{} by driverType. */
export interface PoolConfig {
  maxConns?: number;
  minConns?: number;
  maxConnLifetime?: string;
  maxConnIdleTime?: string;
}

export type DriverSetup = Omit<Partial<DriverConfig>, "errorMode" | "driverType" | "driverSpecific"> & {
  errorMode?: ErrorModeName;
  driverType?: DriverTypeName;
  defaultTxIsolation?: TxIsolationName;
  /** Driver-level insert method; pins every InsertSpec's method when set.
   *  Useful for cross-DB raw-insert comparison. Per-spec method field is
   *  overridden when this is set. */
  defaultInsertMethod?: InsertMethodName;
  /** Unified pool config — mapped to postgres:{} or sql:{} based on driverType. */
  pool?: PoolConfig;
  /** PostgreSQL-specific pool config (takes priority over pool if set). */
  postgres?: Partial<DriverConfig_PostgresConfig>;
  /** Generic SQL pool config (takes priority over pool if set). */
  sql?: Partial<DriverConfig_SqlConfig>;
}

type ScalarKind = "string" | "number" | "boolean";
type FieldRule = ScalarKind | { enum: Set<string> } | { object: Schema };
type Schema = Record<string, FieldRule>;

const poolSchema: Schema = {
  maxConns: "number",
  minConns: "number",
  maxConnLifetime: "string",
  maxConnIdleTime: "string",
};

const postgresSchema: Schema = {
  traceLogLevel: "string",
  maxConnLifetime: "string",
  maxConnIdleTime: "string",
  maxConns: "number",
  minConns: "number",
  minIdleConns: "number",
  defaultQueryExecMode: "string",
  descriptionCacheCapacity: "number",
  statementCacheCapacity: "number",
};

const sqlSchema: Schema = {
  maxOpenConns: "number",
  maxIdleConns: "number",
  connMaxLifetime: "string",
  connMaxIdleTime: "string",
};

const driverSetupSchema: Schema = {
  url: "string",
  driverType: { enum: new Set(Object.keys(driverTypeMap)) },
  defaultTxIsolation: { enum: new Set(Object.keys(txIsolationMap)) },
  defaultInsertMethod: { enum: new Set(Object.keys(insertMethodMap)) },
  errorMode: { enum: new Set(Object.keys(errorModeMap)) },
  pool: { object: poolSchema },
  postgres: { object: postgresSchema },
  sql: { object: sqlSchema },
  bulkSize: "number",
  caCertFile: "string",
  authToken: "string",
  authUser: "string",
  authPassword: "string",
  tlsInsecureSkipVerify: "boolean",
};

function isPlainObject(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function schemaKeys(schema: Schema): string {
  return Object.keys(schema).sort().join(", ");
}

function validateSchema(source: string, path: string, value: unknown, schema: Schema): void {
  if (!isPlainObject(value)) {
    throw new Error(`${source}: ${path} must be object`);
  }

  for (const [key, nested] of Object.entries(value)) {
    if (nested === undefined) continue;

    const rule = schema[key];
    if (!rule) {
      throw new Error(`${source}: unknown ${path} option "${key}" (available: ${schemaKeys(schema)})`);
    }

    const fieldPath = path === "driver" ? key : `${path}.${key}`;
    if (typeof rule === "string") {
      validateKind(source, fieldPath, nested, rule);
    } else if ("enum" in rule) {
      validateEnum(source, fieldPath, nested, rule.enum);
    } else {
      validateSchema(source, fieldPath, nested, rule.object);
    }
  }
}

function validateKind(source: string, path: string, value: unknown, kind: ScalarKind): void {
  if (typeof value !== kind) {
    throw new Error(`${source}: ${path} must be ${kind}, got ${typeof value}`);
  }
}

function validateEnum(source: string, path: string, value: unknown, allowed: Set<string>): void {
  validateKind(source, path, value, "string");
  if (!allowed.has(value as string)) {
    throw new Error(`${source}: ${path} must be one of: ${[...allowed].sort().join(", ")}`);
  }
}

export function validateDriverSetup(source: string, setup: unknown): asserts setup is DriverSetup {
  validateSchema(source, "driver", setup, driverSetupSchema);
}

function mergeDriverSetup(defaults: DriverSetup, cli: Partial<DriverSetup>): DriverSetup {
  const merged: Record<string, unknown> = { ...defaults };
  for (const [key, value] of Object.entries(cli)) {
    if (value === undefined) continue;
    if ((key === "pool" || key === "postgres" || key === "sql") && isPlainObject(value)) {
      merged[key] = { ...((defaults as Record<string, unknown>)[key] as object | undefined), ...value };
    } else {
      merged[key] = value;
    }
  }

  return merged as DriverSetup;
}

/** Resolve pool sugar into the appropriate driver-specific config. */
function resolvePoolConfig(config: DriverSetup): {
  postgres?: Partial<DriverConfig_PostgresConfig>;
  sql?: Partial<DriverConfig_SqlConfig>;
} {
  // Explicit postgres/sql takes priority over pool sugar.
  if (config.postgres) return { postgres: config.postgres };
  if (config.sql) return { sql: config.sql };
  if (!config.pool) return {};

  const p = config.pool;
  const driverType = config.driverType ?? "postgres";

  if (driverType === "noop" || driverType === "csv") {
    return {};
  }

  if (driverType === "mysql" || driverType === "ydb") {
    return {
      sql: {
        maxOpenConns: p.maxConns,
        maxIdleConns: p.minConns,
        connMaxLifetime: p.maxConnLifetime,
        connMaxIdleTime: p.maxConnIdleTime,
      },
    };
  }

  // postgres, picodata, and anything else default to postgres pool config
  return {
    postgres: {
      maxConns: p.maxConns,
      minConns: p.minConns,
      maxConnLifetime: p.maxConnLifetime,
      maxConnIdleTime: p.maxConnIdleTime,
    },
  };
}

// For Go probe spy function
declare function DeclareDriverSetup(index: number, defaults: DriverSetup): DriverSetup;

/**
 * Declare a driver setup with defaults, optionally overridden by CLI via STROPPY_DRIVER_N env.
 * Returns the merged DriverSetup — the caller decides when to instantiate DriverX.
 * @param index Driver index (0 for first/only driver, 1 for second, etc.)
 * @param defaults Script-defined default configuration
 */
export function declareDriverSetup(index: number, defaults: DriverSetup): DriverSetup {
  // Notify probe spy if present (set by Go VM during probe)
  if (typeof DeclareDriverSetup !== 'undefined') {
    DeclareDriverSetup(index, defaults);
  }
  const envKey = `STROPPY_DRIVER_${index}`;
  const raw = __ENV[envKey];
  if (!raw || raw === "") return defaults;

  let cli: unknown;
  try {
    cli = JSON.parse(raw);
  } catch (e) {
    throw new Error(`[stroppy] failed to parse ${envKey}: ${e}`);
  }

  validateDriverSetup(envKey, cli);
  const merged = mergeDriverSetup(defaults, cli);
  validateDriverSetup(`${envKey} merged`, merged);

  return merged;
}

export class DriverX implements QueryAPI {
  private driver: Driver;
  private q: QueryAPI;
  private _errorMode: ErrorModeName = "log";
  private _defaultTxIsolation: TxIsolationName = "db_default";
  private _defaultInsertMethod?: InsertMethodName;

  exec!: QueryAPI["exec"];
  queryRows!: QueryAPI["queryRows"];
  queryRow!: QueryAPI["queryRow"];
  queryValue!: QueryAPI["queryValue"];
  queryCursor!: QueryAPI["queryCursor"];

  private constructor(driver: Driver) {
    this.driver = driver;
    this.q = createQueryAPI(
      (sql, args) => driver.runQuery(sql, args),
      () => this._errorMode,
    );
    this.exec = this.q.exec;
    this.queryRows = this.q.queryRows;
    this.queryRow = this.q.queryRow;
    this.queryValue = this.q.queryValue;
    this.queryCursor = this.q.queryCursor;
  }

  /** Create an empty driver shell. Call setup() to configure it. */
  static create(): DriverX {
    return new DriverX(NewDriver());
  }

  /** Store driver configuration. Safe to call every iteration (runs once).
   *  If called at init phase: marks driver as shared.
   *  If called at iteration/setup phase: marks driver as per-VU.
   *  The driver is lazily dispatched on first use (ensuring DialFunc is available). */
  setup(config: DriverSetup): DriverX {
    // Resolve error mode. Precedence: ENV > config > default ("log")
    if (_envErrorMode) {
      this._errorMode = _envErrorMode;
    } else if (config.errorMode) {
      this._errorMode = config.errorMode;
    }
    // Resolve default tx isolation
    if (config.defaultTxIsolation) {
      this._defaultTxIsolation = config.defaultTxIsolation;
    }
    // Resolve default insert method (pins every InsertSpec when set).
    if (config.defaultInsertMethod) {
      this._defaultInsertMethod = config.defaultInsertMethod;
    }
    // Convert DriverSetup to proto DriverConfig
    const resolved = resolvePoolConfig(config);
    const { postgres: _pg, sql: _sql, pool: _pool, defaultTxIsolation: _dti, defaultInsertMethod: _dim, ...rest } = config;
    const postgres = resolved.postgres;
    const sql = resolved.sql;
    const driverSpecific: DriverConfig["driverSpecific"] = postgres
      ? { oneofKind: "postgres", postgres: DriverConfig_PostgresConfig.create(postgres) }
      : sql
        ? { oneofKind: "sql", sql: DriverConfig_SqlConfig.create(sql) }
        : { oneofKind: undefined };
    const protoConfig: Partial<DriverConfig> = {
      ...rest,
      errorMode: config.errorMode ? errorModeMap[config.errorMode] : undefined,
      driverType: config.driverType ? driverTypeMap[config.driverType] : undefined,
      driverSpecific,
    };
    this.driver.setup(
      DriverConfig.toBinary(DriverConfig.create(protoConfig)),
    );
    return this;
  }

  /** Run a relational InsertSpec through the driver. Metrics and error
   *  handling share the code path used by ad-hoc query exec so workload
   *  dashboards keep working. */
  insertSpec(spec: Partial<DatagenInsertSpec>): void {
    const table = spec.table ?? "unknown";
    const metricTags = { table_name: table };

    // Driver-level default pins every InsertSpec's method when set, so
    // cross-DB runs exercise the same protocol for fair comparison.
    const effectiveSpec = this._defaultInsertMethod !== undefined
      ? { ...spec, method: insertMethodMap[this._defaultInsertMethod] }
      : spec;

    console.log(`InsertSpec into '${table}' starting...`);

    try {
      const protoBytes = DatagenInsertSpec.toBinary(DatagenInsertSpec.create(effectiveSpec));
      const stats = this.driver.insertSpecBin(protoBytes);
      insertErrRateMetric.add(0, metricTags);
      insertMetric.add(stats.elapsed.seconds() * 1000, metricTags);
      console.log(`InsertSpec into '${table}' ended in ${stats.elapsed.string()}`);
    } catch (e) {
      insertErrRateMetric.add(1, metricTags);
      handleError(this._errorMode, e, metricTags);
    }
  }

  /** Start a transaction manually. Call tx.commit() or tx.rollback() when done. */
  begin(options?: { isolation?: TxIsolationName; name?: string }): TxX {
    const level = options?.isolation ?? this._defaultTxIsolation;
    const tx = this.driver.begin(txIsolationMap[level]);
    return new TxX(tx, level, () => this._errorMode, options?.name);
  }

  /** Execute a callback within a transaction. Auto-commits on success, auto-rollbacks on error. */
  beginTx(fn: (tx: TxX) => void): void;
  beginTx(options: { isolation?: TxIsolationName; name?: string }, 
    fn: (tx: TxX) => void): void;

  beginTx(
    optionsOrFn: { isolation?: TxIsolationName; name?: string } | ((tx: TxX) => void), 
    maybeFn?: (tx: TxX) => void): void {

    const isOptions = typeof optionsOrFn === "object";
    const options = isOptions ? optionsOrFn : undefined;
    const fn = isOptions ? maybeFn! : optionsOrFn;

    const tx = this.begin(options);
    const errTags = tx.name ? { name: tx.name } : undefined;
    try {
      fn(tx);
      tx.commit();
      txErrRateMetric.add(0, errTags);
    } catch (e) {
      txErrRateMetric.add(1, errTags);
      try { tx.rollback(); } catch (_) { /* ignore rollback error */ }
      throw e;
    }
  }

  getDriver(): Driver {
    return this.driver;
  }
}


const _stepFilter: Set<string> | null = (() => {
  const only = ENV("STROPPY_STEPS", "", "comma-separated list of steps to run (allowlist), same as --steps");
  if (only) return new Set(only.split(","));
  return null;
})();

const _stepSkip: Set<string> | null = (() => {
  const skip = ENV("STROPPY_NO_STEPS", "", "comma-separated list of steps to skip (blocklist), same as --no-steps");
  if (skip) return new Set(skip.split(","));
  return null;
})();

function isStepEnabled(name: string): boolean {
  if (_stepFilter) return _stepFilter.has(name);
  if (_stepSkip) return !_stepSkip.has(name);
  return true;
}

export const Step = Object.assign(
  (name: string, step: () => void): void => {
    if (!isStepEnabled(name)) {
      console.log(`Skipping step '${name}'`);
      return;
    }
    Step.begin(name);
    step();
    Step.end(name);
  },
  {
    begin: (name: string): void => {
      NotifyStep(name, StroppyRun_Status.STATUS_RUNNING);
      console.log(`Start of '${name}' step`);
    },
    end: (name: string): void => {
      console.log(`End of '${name}' step`);
      NotifyStep(name, StroppyRun_Status.STATUS_COMPLETED);
    },
  }
);


/** Wrap a function so it executes only once per VU.
 *  Call once() during init to capture the guard, then invoke the
 *  returned function during iterations — it only fires on the first call. */
export const once = Once;

// ============================================================================
// T2.3: SQLSTATE 40001 / deadlock retry helper
// ============================================================================
//
// PG REPEATABLE READ uses snapshot isolation, so concurrent updates to the
// same row abort with SQLSTATE 40001 ("could not serialize access due to
// concurrent update"). MySQL's row-locking equivalent is "Deadlock found
// when trying to get lock" (Error 1213, SQLSTATE 40001). The TPC-C spec
// §5.2.5 caps the total error rate at 1%, so we retry these aborts a few
// times before letting them bubble up to k6's tx_error_rate.
//
// Audit (2026-04-09) of pkg/driver/{postgres,mysql,sqldriver,...}: the
// stroppy Go layer wraps every driver error with `fmt.Errorf("...: %w",
// err)` (see sqldriver/run_query.go and cmd/xk6air/driver_wrapper.go). Both
// pgx's pgconn.PgError.Error() and go-sql-driver's mysql.MySQLError.Error()
// stringify to formats that include enough textual signal:
//
//   pg:    "ERROR: could not serialize access due to concurrent update (SQLSTATE 40001)"
//   pg:    "ERROR: deadlock detected (SQLSTATE 40P01)"
//   mysql: "Error 1213 (40001): Deadlock found when trying to get lock; ..."
//
// So no Go-side classification is needed — the JS catch sees the wrapped
// message via `e.message` and a regex match is enough.
//
// IMPORTANT: this MUST return false for the spec §2.4.2.3 New-Order rollback
// sentinel ("tpcc_rollback:item_not_found"). On PG that path is raised via
// `RAISE EXCEPTION` (SQLSTATE P0001) and on MySQL via `SIGNAL SQLSTATE
// '45000'`, neither of which match the serialization patterns below — but we
// add an explicit early-out so a future regex tweak can't accidentally trip
// the rollback path.

/** Check if an error is a transient serialization failure or deadlock that
 *  should be retried. Matches both PostgreSQL and MySQL error texts.
 *  Explicitly excludes the TPC-C `tpcc_rollback:` sentinel so the spec
 *  §2.4.2.3 New-Order rollback path is never retried. */
/* eslint-disable @typescript-eslint/no-explicit-any */
export const isSerializationError = (e: any): boolean => {
  const msg = String(e?.message ?? e);
  // Defensive: never retry the spec-mandated rollback sentinel.
  if (msg.indexOf("tpcc_rollback:") >= 0) return false;
  return /SQLSTATE 40001/i.test(msg)
      || /could not serialize access/i.test(msg)
      || /SQLSTATE 40P01/i.test(msg)
      || /deadlock detected/i.test(msg)        // pg
      || /Deadlock found/i.test(msg)           // mysql
      || /Error 1213/i.test(msg);              // mysql numeric
};
/* eslint-enable @typescript-eslint/no-explicit-any */

/** Run `fn`, retrying up to `maxAttempts - 1` times when `isRetryable(e)`
 *  returns true. The optional `onRetry` callback fires once per retry,
 *  before re-invoking `fn`, and receives the upcoming attempt number
 *  (2-based) plus the caught error. No backoff: serialization retries are
 *  immediate by design — sleeping inside a tx body would just deepen the
 *  contention window. Use the callback to bump a counter so operators can
 *  observe how often retries fire. */
/* eslint-disable @typescript-eslint/no-explicit-any */
export const retry = <T>(
  maxAttempts: number,
  isRetryable: (e: any) => boolean,
  fn: () => T,
  onRetry?: (attempt: number, e: any) => void,
): T => {
  let lastErr: any;
  for (let attempt = 1; attempt <= maxAttempts; attempt++) {
    try {
      return fn();
    } catch (e) {
      lastErr = e;
      if (!isRetryable(e) || attempt === maxAttempts) {
        throw e;
      }
      if (onRetry) onRetry(attempt + 1, e);
    }
  }
  throw lastErr; // unreachable — last iteration always throws or returns
};
/* eslint-enable @typescript-eslint/no-explicit-any */
