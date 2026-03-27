import { Counter, Rate, Trend } from "k6/metrics";
import { test } from "k6/execution"
import encoding from "k6/x/encoding";
globalThis.TextEncoder = encoding.TextEncoder;
globalThis.TextDecoder = encoding.TextDecoder;

import {
  NewDriver,
  NewGeneratorByRuleBin,
  NewGroupGeneratorByRulesBin,
  NotifyStep,
  DeclareEnv,
  Once,
  Driver,
  Tx,
  QueryStats,
  QueryResult,
} from "k6/x/stroppy";
import {
  Generation_Rule,
  Generation_Distribution,
  Generation_Distribution_DistributionType,
  QueryParamGroup,
  DriverConfig,
  QueryParamDescriptor,
  InsertDescriptor,
  InsertMethod,
  DriverConfig_ErrorMode,
  DriverConfig_DriverType,
  DriverConfig_PostgresConfig,
  DriverConfig_SqlConfig,
  StroppyRun_Status,
  Timestamp,
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


export type InsertMethodName = "plain_query" | "plain_bulk" | "copy_from";

const insertMethodMap: Record<InsertMethodName, InsertMethod> = {
  plain_query: InsertMethod.PLAIN_QUERY,
  plain_bulk: InsertMethod.PLAIN_BULK,
  copy_from: InsertMethod.COPY_FROM,
};

export type ErrorModeName = "silent" | "log" | "throw" | "fail" | "abort";

const errorModeMap: Record<ErrorModeName, DriverConfig_ErrorMode> = {
  silent: DriverConfig_ErrorMode.ERROR_MODE_SILENT,
  log: DriverConfig_ErrorMode.ERROR_MODE_LOG,
  throw: DriverConfig_ErrorMode.ERROR_MODE_THROW,
  fail: DriverConfig_ErrorMode.ERROR_MODE_FAIL,
  abort: DriverConfig_ErrorMode.ERROR_MODE_ABORT,
};

export type DriverTypeName = "postgres" | "mysql" | "picodata" | "ydb";

const driverTypeMap: Record<DriverTypeName, DriverConfig_DriverType> = {
  postgres: DriverConfig_DriverType.DRIVER_TYPE_POSTGRES,
  mysql: DriverConfig_DriverType.DRIVER_TYPE_MYSQL,
  picodata: DriverConfig_DriverType.DRIVER_TYPE_PICODATA,
  ydb: DriverConfig_DriverType.DRIVER_TYPE_YDB,
};

const _envErrorMode = ENV("STROPPY_ERROR_MODE", undefined, 
"(default: by config, else 'log') error handling mode: silent, log, throw, fail, abort",
) as ErrorModeName | undefined;

interface InsertDescriptorX {
  method?: InsertMethodName;
  seed?: number;
  params?: Record<string, Generation_Rule>;
  groups?: Record<string, Record<string, Generation_Rule>>;
}

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
      runQueryMetric.add(result.stats.elapsed.milliseconds(), tags);
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
        this._cleanDuration += res.stats.elapsed.milliseconds();
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
  defaultInsertMethod?: InsertMethodName;
  defaultTxIsolation?: TxIsolationName;
  /** Unified pool config — mapped to postgres:{} or sql:{} based on driverType. */
  pool?: PoolConfig;
  /** PostgreSQL-specific pool config (takes priority over pool if set). */
  postgres?: Partial<DriverConfig_PostgresConfig>;
  /** Generic SQL pool config (takes priority over pool if set). */
  sql?: Partial<DriverConfig_SqlConfig>;
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

  try {
    const cli = JSON.parse(raw) as Partial<DriverSetup>;
    // Deep merge: CLI fields override defaults, but only if actually set.
    const merged: DriverSetup = { ...defaults };
  if (cli.driverType          !== undefined) merged.driverType          = cli.driverType          as DriverTypeName;
  if (cli.url                 !== undefined) merged.url                 = cli.url;
  if (cli.defaultInsertMethod !== undefined) merged.defaultInsertMethod = cli.defaultInsertMethod as InsertMethodName;
  if (cli.defaultTxIsolation  !== undefined) merged.defaultTxIsolation  = cli.defaultTxIsolation  as TxIsolationName;
  if (cli.errorMode           !== undefined) merged.errorMode           = cli.errorMode           as ErrorModeName;
  if (cli.pool                !== undefined) merged.pool                = cli.pool;
  if (cli.postgres            !== undefined) merged.postgres            = cli.postgres;
  if (cli.sql                 !== undefined) merged.sql                 = cli.sql;
    if ((cli as any).bulkSize !== undefined) merged.bulkSize = (cli as any).bulkSize;
  if (cli.caCertFile           !== undefined) merged.caCertFile           = cli.caCertFile;
  if (cli.authToken            !== undefined) merged.authToken            = cli.authToken;
  if (cli.authUser             !== undefined) merged.authUser             = cli.authUser;
  if (cli.authPassword         !== undefined) merged.authPassword         = cli.authPassword;
  if (cli.tlsInsecureSkipVerify !== undefined) merged.tlsInsecureSkipVerify = cli.tlsInsecureSkipVerify;
    return merged;
  } catch (e) {
    console.error(`[stroppy] failed to parse ${envKey}: ${e}`);
    return defaults;
  }
}

export class DriverX implements QueryAPI {
  private driver: Driver;
  private q: QueryAPI;
  private _errorMode: ErrorModeName = "log";
  private _defaultInsertMethod: InsertMethodName = "plain_bulk";
  private _defaultTxIsolation: TxIsolationName = "db_default";

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
    // Resolve default insert method
    if (config.defaultInsertMethod) {
      this._defaultInsertMethod = config.defaultInsertMethod;
    }
    // Resolve default tx isolation
    if (config.defaultTxIsolation) {
      this._defaultTxIsolation = config.defaultTxIsolation;
    }
    // Convert DriverSetup to proto DriverConfig
    const resolved = resolvePoolConfig(config);
    const { postgres: _pg, sql: _sql, pool: _pool, defaultInsertMethod: _dim, defaultTxIsolation: _dti, ...rest } = config;
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

  insert(insert: Partial<InsertDescriptor>): void;
  insert(tableName: string, count: number, insert: InsertDescriptorX): void;
  insert(
    insertOrTableName: string | Partial<InsertDescriptor>,
    count?: number,
    insert?: InsertDescriptorX,
  ): void {
    const isName = typeof insertOrTableName === "string";
    const descriptor = isName
      ? {
          tableName: insertOrTableName,
          method: insertMethodMap[insert?.method ?? this._defaultInsertMethod],
          seed: String(insert?.seed ?? _seed),
          params: R.group(insert?.params ?? {}),
          groups: R.groups(insert?.groups ?? {}),
          count,
        }
      : insertOrTableName;

    console.log(
      `Insertion into '${descriptor.tableName}' of ${descriptor.count} values starting...`,
    );

    const metricTags = { table_name: descriptor.tableName ?? "unknown" };
    try {
      const stats = this.driver.insertValuesBin(
        InsertDescriptor.toBinary(InsertDescriptor.create(descriptor)),
      );
      insertErrRateMetric.add(0, metricTags);
      insertMetric.add(stats.elapsed.milliseconds(), metricTags);
    } catch (e) {
      insertErrRateMetric.add(1, metricTags);
      handleError(this._errorMode, e, metricTags);
    }

    console.log(`Insertion into '${descriptor.tableName}' ended`);
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

// ============================================================================
// Module-wide seed (0 = random, >0 = fixed). Inherited by .gen() and insert().
// ============================================================================

let _seed = 0;

/** Set the module-wide default seed. 0 = random on every use, >0 = fixed. */
export function setSeed(s: number): void {
  _seed = s;
}

// ============================================================================
// Rule — Generation_Rule enriched with .gen()
// ============================================================================

export type Rule = Generation_Rule & {
  /** Create a Generator from this rule. seed: 0 = random, >0 = fixed.
   *  Falls back to the module-wide seed set via setSeed() if omitted. */
  gen(seed?: number): ReturnType<typeof NewGeneratorByRuleBin>;
};

export type GroupRule = QueryParamDescriptor[] & {
  /** Create a Generator from this group. seed: 0 = random, >0 = fixed.
   *  Falls back to the module-wide seed set via setSeed() if omitted. */
  gen(seed?: number): ReturnType<typeof NewGroupGeneratorByRulesBin>;
};

function rule(r: Generation_Rule): Rule {
  return Object.assign(r, {
    gen(seed?: number): ReturnType<typeof NewGeneratorByRuleBin> {
      return NewGeneratorByRuleBin(
        seed ?? _seed,
        Generation_Rule.toBinary(Generation_Rule.create(r)),
      );
    },
  });
}

// ============================================================================
// Distribution
// ============================================================================

export type Distribution =
  | { kind: "normal"; screw?: number }
  | { kind: "uniform" }
  | { kind: "zipf"; screw: number };

export const Dist = {
  normal: (screw = 0): Distribution => ({ kind: "normal", screw }),
  uniform: (): Distribution => ({ kind: "uniform" }),
  zipf: (screw: number): Distribution => ({ kind: "zipf", screw }),
};

function dateToTimestamp(d: Date): Timestamp {
  return { seconds: Math.floor(d.getTime() / 1000).toString(), nanos: 0 };
}

function toProtoDistribution(d: Distribution): Generation_Distribution {
  switch (d.kind) {
    case "normal":
      return { type: Generation_Distribution_DistributionType.NORMAL, screw: d.screw ?? 0 };
    case "uniform":
      return { type: Generation_Distribution_DistributionType.UNIFORM, screw: 0 };
    case "zipf":
      return { type: Generation_Distribution_DistributionType.ZIPF, screw: d.screw };
  }
}

// ============================================================================
// Alphabets
// ============================================================================

type Alphabet = Array<{ min: number; max: number }>;

export const AB = {
  en: [
    { min: 65, max: 90 },
    { min: 97, max: 122 },
  ] as const,

  enNum: [
    { min: 65, max: 90 },
    { min: 97, max: 122 },
    { min: 48, max: 57 },
  ] as const,

  num: [{ min: 48, max: 57 }] as const,

  enUpper: [{ min: 65, max: 90 }] as const,

  enSpc: [
    { min: 65, max: 90 },
    { min: 97, max: 122 },
    { min: 32, max: 33 },
  ] as const,

  enNumSpc: [
    { min: 65, max: 90 },
    { min: 97, max: 122 },
    { min: 32, max: 33 },
    { min: 48, max: 57 },
  ] as const,
} as const satisfies Record<string, Alphabet>;

// ============================================================================
// Generator builders
// ============================================================================

// Define the interface with overloads
interface ConstGenerators {
  /** Fixed string value. */
  str: (val: string) => Rule;
  /** Fixed 32-bit signed integer value. */
  int32: (val: number) => Rule;
  /** Fixed 64-bit signed integer value (proto: int64 → string). */
  int64: (val: string | number | bigint) => Rule;
  /** Fixed 32-bit unsigned integer value. */
  uint32: (val: number) => Rule;
  /** Fixed 64-bit unsigned integer value (proto: uint64 → string). */
  uint64: (val: string | number | bigint) => Rule;
  /** Fixed 32-bit float value; beware precision for currency. */
  float: (val: number) => Rule;
  /** Fixed 64-bit float value. */
  double: (val: number) => Rule;
  /** Fixed arbitrary-precision decimal value. */
  decimal: (val: string) => Rule;
  /** Fixed date/time value. */
  datetime: (val: Date) => Rule;
  /** Fixed boolean value. */
  bool: (val: boolean) => Rule;
  /** Fixed UUID value. */
  uuid: (val: string) => Rule;
}

interface RandomRangeGenerators {
  /** String constraints (length, alphabet). Proto: min_len/max_len are uint64. */
  str(len: number, alphabet?: Alphabet): Rule;
  str(minLen: number, maxLen: number, alphabet?: Alphabet): Rule;

  /** Signed 32-bit integer range (inclusive). */
  int32(min: number, max: number, distribution?: Distribution): Rule;
  /** Signed 64-bit integer range (inclusive). Proto: int64 → string. */
  int64(min: string | number | bigint, max: string | number | bigint, distribution?: Distribution): Rule;

  /** Unsigned 32-bit integer range; use for sizes/indices. */
  uint32(min: number, max: number, distribution?: Distribution): Rule;
  /** Unsigned 64-bit integer range (inclusive). Proto: uint64 → string. */
  uint64(min: string | number | bigint, max: string | number | bigint, distribution?: Distribution): Rule;

  /** 32-bit float range (inclusive); beware precision for currency. */
  float(min: number, max: number, distribution?: Distribution): Rule;
  /** 64-bit float range (inclusive) for high-precision numeric data. */
  double(min: number, max: number, distribution?: Distribution): Rule;

  /** Arbitrary-precision decimal range via double bounds. */
  decimal(min: number, max: number, distribution?: Distribution): Rule;
  /** Arbitrary-precision decimal range via string bounds (scientific notation OK). */
  decimal(min: string, max: string, distribution?: Distribution): Rule;

  /** Date/time range (inclusive). */
  datetime(min: Date, max: Date, distribution?: Distribution): Rule;

  /** Boolean with given ratio of true values; unique = true → sequence [false, true]. */
  bool: (ratio: number, unique?: boolean) => Rule;

  /** Random UUID v4. Seed is ignored. */
  uuid: () => Rule;
  /** Random UUID v4, reproducible by seed. */
  uuidSeeded: () => Rule;

  // Helpers
  group: (params: Record<string, Generation_Rule>) => GroupRule;
  groups: (
    groups: Record<string, Record<string, Generation_Rule>>,
  ) => QueryParamGroup[];
}

export const C: ConstGenerators = {
  str: (val: string): Rule =>
    rule({ kind: { oneofKind: "stringConst", stringConst: val } }),

  int32: (val: number): Rule =>
    rule({ kind: { oneofKind: "int32Const", int32Const: val } }),

  int64: (val: string | number | bigint): Rule =>
    rule({ kind: { oneofKind: "int64Const", int64Const: String(val) } }),

  uint32: (val: number): Rule =>
    rule({ kind: { oneofKind: "uint32Const", uint32Const: val } }),

  uint64: (val: string | number | bigint): Rule =>
    rule({ kind: { oneofKind: "uint64Const", uint64Const: String(val) } }),

  float: (val: number): Rule =>
    rule({ kind: { oneofKind: "floatConst", floatConst: val } }),

  double: (val: number): Rule =>
    rule({ kind: { oneofKind: "doubleConst", doubleConst: val } }),

  decimal: (val: string): Rule =>
    rule({ kind: { oneofKind: "decimalConst", decimalConst: { value: val } } }),

  datetime: (val: Date): Rule =>
    rule({
      kind: {
        oneofKind: "datetimeConst",
        datetimeConst: { value: dateToTimestamp(val) },
      },
    }),

  bool: (val: boolean): Rule =>
    rule({ kind: { oneofKind: "boolConst", boolConst: val } }),

  uuid: (val: string): Rule =>
    rule({ kind: { oneofKind: "uuidConst", uuidConst: { value: val } } }),
};

export const R: RandomRangeGenerators = {
  str(
    lenOrMin: number,
    alphabetOrMax?: Alphabet | number,
    alphabet: Alphabet = AB.en,
  ): Rule {
    const isRange = typeof alphabetOrMax === "number";
    const minLen = lenOrMin;
    const maxLen = isRange ? alphabetOrMax : lenOrMin;
    const alph = isRange ? alphabet : (alphabetOrMax ?? AB.en);

    return rule({
      kind: {
        oneofKind: "stringRange",
        stringRange: {
          minLen: minLen.toString(),
          maxLen: maxLen.toString(),
          alphabet: { ranges: alph },
        },
      },
    });
  },

  int32(min: number, max: number, distribution?: Distribution): Rule {
    return rule({
      kind: { oneofKind: "int32Range", int32Range: { min, max } },
      distribution: distribution && toProtoDistribution(distribution),
    });
  },

  int64(min: string | number | bigint, max: string | number | bigint, distribution?: Distribution): Rule {
    return rule({
      kind: { oneofKind: "int64Range", int64Range: { min: String(min), max: String(max) } },
      distribution: distribution && toProtoDistribution(distribution),
    });
  },

  uint32(min: number, max: number, distribution?: Distribution): Rule {
    return rule({
      kind: { oneofKind: "uint32Range", uint32Range: { min, max } },
      distribution: distribution && toProtoDistribution(distribution),
    });
  },

  uint64(min: string | number | bigint, max: string | number | bigint, distribution?: Distribution): Rule {
    return rule({
      kind: { oneofKind: "uint64Range", uint64Range: { min: String(min), max: String(max) } },
      distribution: distribution && toProtoDistribution(distribution),
    });
  },

  float(min: number, max: number, distribution?: Distribution): Rule {
    return rule({
      kind: { oneofKind: "floatRange", floatRange: { min, max } },
      distribution: distribution && toProtoDistribution(distribution),
    });
  },

  double(min: number, max: number, distribution?: Distribution): Rule {
    return rule({
      kind: { oneofKind: "doubleRange", doubleRange: { min, max } },
      distribution: distribution && toProtoDistribution(distribution),
    });
  },

  decimal(min: number | string, max: number | string, distribution?: Distribution): Rule {
    const isStr = typeof min === "string";
    return rule({
      kind: {
        oneofKind: "decimalRange",
        decimalRange: {
          type: isStr
            ? { oneofKind: "string", string: { min: min as string, max: max as string } }
            : { oneofKind: "double", double: { min: min as number, max: max as number } },
        },
      },
      distribution: distribution && toProtoDistribution(distribution),
    });
  },

  datetime(min: Date, max: Date, distribution?: Distribution): Rule {
    return rule({
      kind: {
        oneofKind: "datetimeRange",
        datetimeRange: {
          type: {
            oneofKind: "timestampPb",
            timestampPb: {
              min: dateToTimestamp(min),
              max: dateToTimestamp(max),
            },
          },
        },
      },
      distribution: distribution && toProtoDistribution(distribution),
    });
  },

  // ratio of true values; unique = true => sequence [false, true]
  bool(ratio: number, unique = false): Rule {
    return rule({
      kind: { oneofKind: "boolRange", boolRange: { ratio } },
      unique: unique,
    });
  },

  uuid(): Rule {
    return rule({ kind: { oneofKind: "uuidRandom", uuidRandom: true } });
  },

  uuidSeeded(): Rule {
    return rule({ kind: { oneofKind: "uuidSeeded", uuidSeeded: true } });
  },

  group: group_internal,

  groups(
    groups: Record<string, Record<string, Generation_Rule>>,
  ): QueryParamGroup[] {
    return Object.entries(groups).map(([name, params]) =>
      QueryParamGroup.create({ name, params: group_internal(params) }),
    );
  },
};

interface SequenceGenerators {
  /** Unique string sequence (length, alphabet). */
  str(len: number, alphabet?: Alphabet): Rule;
  str(minLen: number, maxLen: number, alphabet?: Alphabet): Rule;

  /** Sequential 32-bit signed integer from min to max. */
  int32: (min: number, max: number) => Rule;
  /** Sequential 64-bit signed integer from min to max. Proto: int64 → string. */
  int64: (min: string | number | bigint, max: string | number | bigint) => Rule;
  /** Sequential 32-bit unsigned integer from min to max. */
  uint32: (min: number, max: number) => Rule;
  /** Sequential 64-bit unsigned integer from min to max. Proto: uint64 → string. */
  uint64: (min: string | number | bigint, max: string | number | bigint) => Rule;

  /** Sequential UUIDs from min to max (inclusive).
   *  min defaults to 00000000-0000-0000-0000-000000000000 if omitted. */
  uuid(max: string): Rule;
  uuid(min: string, max: string): Rule;
}

export const S: SequenceGenerators = {
  str(
    lenOrMin: number,
    alphabetOrMax?: Alphabet | number,
    alphabet: Alphabet = AB.en,
  ): Rule {
    const isRange = typeof alphabetOrMax === "number";
    const minLen = lenOrMin;
    const maxLen = isRange ? alphabetOrMax : lenOrMin;
    const alph = isRange ? alphabet : (alphabetOrMax ?? AB.en);

    return rule({
      kind: {
        oneofKind: "stringRange",
        stringRange: {
          minLen: minLen.toString(),
          maxLen: maxLen.toString(),
          alphabet: { ranges: alph },
        },
      },
      unique: true,
    });
  },

  int32(min: number, max: number): Rule {
    return rule({
      kind: { oneofKind: "int32Range", int32Range: { min, max } },
      unique: true,
    });
  },

  int64(min: string | number | bigint, max: string | number | bigint): Rule {
    return rule({
      kind: { oneofKind: "int64Range", int64Range: { min: String(min), max: String(max) } },
      unique: true,
    });
  },

  uint32(min: number, max: number): Rule {
    return rule({
      kind: { oneofKind: "uint32Range", uint32Range: { min, max } },
      unique: true,
    });
  },

  uint64(min: string | number | bigint, max: string | number | bigint): Rule {
    return rule({
      kind: { oneofKind: "uint64Range", uint64Range: { min: String(min), max: String(max) } },
      unique: true,
    });
  },

  uuid(minOrMax: string, max?: string): Rule {
    const resolvedMin = max !== undefined ? minOrMax : undefined;
    const resolvedMax = max !== undefined ? max : minOrMax;
    return rule({
      kind: {
        oneofKind: "uuidSeq",
        uuidSeq: {
          max: { value: resolvedMax },
          ...(resolvedMin !== undefined ? { min: { value: resolvedMin } } : {}),
        },
      },
    });
  },
};

function group_internal(
  params: Record<string, Generation_Rule>,
): GroupRule {
  const descriptors = Object.entries(params).map(([name, generationRule]) =>
    QueryParamDescriptor.create({ name, generationRule }),
  );
  return Object.assign(descriptors, {
    gen(seed?: number): ReturnType<typeof NewGroupGeneratorByRulesBin> {
      return NewGroupGeneratorByRulesBin(
        seed ?? _seed,
        QueryParamGroup.toBinary(
          QueryParamGroup.create({ name: "", params: descriptors }),
        ),
      );
    },
  }) as GroupRule;
}

/** Wrap a function so it executes only once per VU.
 *  Call once() during init to capture the guard, then invoke the
 *  returned function during iterations — it only fires on the first call. */
export const once = Once;
