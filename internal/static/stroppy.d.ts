// Type declarations for k6/x/stroppy Go module
// These declarations provide TypeScript/LSP support for the k6 module API

import type {
  GlobalConfig,
  UnitDescriptor,
  DriverTransactionStat,
  InsertSpec,
  DriverConfig,
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
    rows: number;
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
    /** Run a relational InsertSpec through the driver. The TS wrapper handles
     *  marshalling; JS code never constructs the binary directly.
     *  @throws {Error} on insert failure or protobuf unmarshal error */
    insertSpecBin(spec: BinMsg<InsertSpec>): QueryStats;
    /** @throws {Error} on query execution or argument processing error */
    runQuery(sql: string, args: Record<string, any>): QueryResult;
    /** Start a transaction with the given isolation level (proto TxIsolationLevel enum value).
     *  @throws {Error} if the driver does not support the requested isolation level */
    begin(isolationLevel: number, txName?: string): Tx;
    /** Store driver configuration. The driver is lazily dispatched on first use.
     *  If called at init phase (VU state nil): marks driver as shared.
     *  If called at iteration phase: marks driver as per-VU. */
    setup(configBin: BinMsg<DriverConfig>): void;
  }

  // k6 module functions provided by Go module
  export declare function NotifyStep(name: String, status: number): void;
  export declare function SetStepTag(name: string): void;
  export declare function ClearStepTag(name: string): void;
  export declare function CurrentStep(): string;
  export declare function Teardown(): Error;
  export declare function NewDriver(): Driver;

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

  // -------- Draw iter 2: sobek-bound Go structs per StreamDraw arm --------

  /** Concurrency: one Draw instance per VU. Cursors are not atomic. */
  export interface DrawX {
    /** Stateless sample at (seed, key); does not touch the cursor. */
    sample(seed: number, key: number): any;
    /** Value at current cursor; advances the cursor. */
    next(): any;
    /** Set cursor to `key` (absolute). */
    seek(key: number): void;
    /** Reset cursor to 0. */
    reset(): void;
  }

  // Handle registries. Called internally by datagen.ts DrawRT.* builders;
  // workload code should not touch these directly.
  export declare function RegisterDict(name: string, dictBin: Uint8Array): number;
  export declare function RegisterAlphabet(alphabetBin: Uint8Array): number;
  export declare function RegisterGrammar(grammarBin: Uint8Array): number;

  // Per-arm constructors. Errors surface to JS as thrown exceptions via
  // sobek's native error-to-throw conversion.
  export declare function NewDrawIntUniform(seed: number, lo: number, hi: number): DrawX;
  export declare function NewDrawFloatUniform(seed: number, lo: number, hi: number): DrawX;
  export declare function NewDrawNormal(
    seed: number,
    lo: number,
    hi: number,
    screw: number,
  ): DrawX;
  export declare function NewDrawZipf(
    seed: number,
    lo: number,
    hi: number,
    exponent: number,
  ): DrawX;
  export declare function NewDrawNURand(
    seed: number,
    a: number,
    x: number,
    y: number,
    cSalt: number,
  ): DrawX;
  export declare function NewDrawBernoulli(seed: number, p: number): DrawX;
  export declare function NewDrawDate(seed: number, loDays: number, hiDays: number): DrawX;
  export declare function NewDrawDecimal(
    seed: number,
    lo: number,
    hi: number,
    scale: number,
  ): DrawX;
  export declare function NewDrawASCII(
    seed: number,
    minLen: number,
    maxLen: number,
    alphabetHandle: number,
  ): DrawX;
  export declare function NewDrawDict(
    seed: number,
    dictHandle: number,
    weightSet: string,
  ): DrawX;
  export declare function NewDrawJoint(
    seed: number,
    dictHandle: number,
    column: string,
    weightSet: string,
  ): DrawX;
  export declare function NewDrawPhrase(
    seed: number,
    vocabHandle: number,
    minWords: number,
    maxWords: number,
    separator: string,
  ): DrawX;
  export declare function NewDrawGrammar(
    seed: number,
    grammarHandle: number,
    minLen: number,
    maxLen: number,
  ): DrawX;
}
