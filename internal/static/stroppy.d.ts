// Type declarations for k6/x/stroppy Go module
// These declarations provide TypeScript/LSP support for the k6 module API

import type {
  GlobalConfig,
  UnitDescriptor,
  DriverTransactionStat,
  InsertDescriptor,
  InsertSpec,
  DriverConfig,
  Generation_Rule,
  QueryParamGroup,
  DateTime,
} from "./stroppy.pb.js";

declare module "k6/x/stroppy" {
  // protobuf serialized messages - type-safe wrapper around Uint8Array
  export type BinMsg<_T extends any> = Uint8Array<ArrayBufferLike>;

  export interface GoDuration {
    /** Truncated to int64 milliseconds — sub-ms durations report as 0. */
    milliseconds(): number;
    /** Float seconds — multiply by 1000 for float-precision ms. */
    seconds(): number;
    /** Truncated to int64 microseconds. */
    microseconds(): number;
    /** Truncated to int64 nanoseconds. */
    nanoseconds(): number;
    string(): string;
  }

  export interface QueryStats {
    elapsed: GoDuration;
  }

  // Cursor-style row iteration over query results.
  // Auto-closes when next() returns false.
  export interface Rows {
    columns(): string[];
    next(): boolean;
    values(): any[];
    readAll(limit: number): any[][];
    err(): Error | null;
    close(): Error | null;
  }

  export interface QueryResult {
    stats: QueryStats;
    rows: Rows;
  }

  // Transaction interface - provides query execution within a transaction.
  // All methods throw on error (Go errors become JS exceptions via sobek).
  export interface Tx {
    /** @throws {Error} on query execution error */
    runQuery(sql: string, args: Record<string, any>): QueryResult;
    /** @throws {Error} on commit failure */
    commit(): void;
    /** @throws {Error} on rollback failure */
    rollback(): void;
  }

  // Driver interface - provides database operations.
  // All methods throw on error (Go errors become JS exceptions via sobek).
  export interface Driver {
    /** @throws {Error} on insert failure or protobuf unmarshal error */
    insertValuesBin(insert: BinMsg<InsertDescriptor>): QueryStats;
    /** Run a relational InsertSpec through the driver. The TS wrapper handles
     *  marshalling; JS code never constructs the binary directly.
     *  @throws {Error} on insert failure or protobuf unmarshal error */
    insertSpecBin(spec: BinMsg<InsertSpec>): QueryStats;
    /** @throws {Error} on query execution or argument processing error */
    runQuery(sql: string, args: Record<string, any>): QueryResult;
    /** Start a transaction with the given isolation level (proto TxIsolationLevel enum value).
     *  @throws {Error} if the driver does not support the requested isolation level */
    begin(isolationLevel: number): Tx;
    /** Store driver configuration. The driver is lazily dispatched on first use.
     *  If called at init phase (VU state nil): marks driver as shared.
     *  If called at iteration phase: marks driver as per-VU. */
    setup(configBin: BinMsg<DriverConfig>): void;
  }

  // Generator interface - provides data generation
  export interface Generator {
    next(): any;
  }

  // k6 module functions provided by Go module
  export declare function NotifyStep(name: String, status: number): void;
  export declare function Teardown(): Error;
  export declare function NewDriver(): Driver;
  export declare function NewGeneratorByRuleBin(
    seed: number,
    rule: BinMsg<Generation_Rule>,
  ): Generator;
  export declare function NewGroupGeneratorByRulesBin(
    seed: number,
    rule: BinMsg<QueryParamGroup>,
  ): Generator;

  export interface Picker {
    pick(array: any[]): any;
    pickWeighted(array: any[], weights: number[]): any;
  }
  export declare function NewPicker(seed: number): Picker;

  export declare function DeclareEnv(
    names: string[],
    default_: string,
    description: string,
  ): void;

  /** Wrap a function so it executes only once per VU.
   *  Call Once() during init, then invoke the returned function during iterations.
   *  The wrapped function caches and returns the result of the first invocation. */
  export declare function Once<F extends (...args: any[]) => any>(fn: F): F;
}
