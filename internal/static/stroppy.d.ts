// Type declarations for k6/x/stroppy Go module
// These declarations provide TypeScript/LSP support for the k6 module API

import type {
  GlobalConfig,
  UnitDescriptor,
  DriverTransactionStat,
  InsertDescriptor,
  Generation_Rule,
  QueryParamGroup,
  DateTime,
} from "./stroppy.pb.js";

declare module "k6/x/stroppy" {
  // protobuf serialized messages - type-safe wrapper around Uint8Array
  export type BinMsg<_T extends any> = Uint8Array<ArrayBufferLike>;

  export interface GoDuration {
    milliseconds(): number;
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

  // Driver interface - provides database operations.
  // All methods throw on error (Go errors become JS exceptions via sobek).
  export interface Driver {
    /** @throws {Error} on insert failure or protobuf unmarshal error */
    insertValuesBin(insert: BinMsg<InsertDescriptor>): QueryStats;
    /** @throws {Error} on query execution or argument processing error */
    runQuery(sql: string, args: Record<string, any>): QueryResult;
    /* Per VU setup. Runs lambda once at first iteration and never after it.  */
    setup(lambda: () => void);
  }

  // Generator interface - provides data generation
  export interface Generator {
    next(): any;
  }

  // k6 module functions provided by Go module
  export declare function NotifyStep(name: String, status: number): void;
  export declare function Teardown(): Error;
  export declare function NewDriverByConfigBin(
    configBin: BinMsg<GlobalConfig>,
  ): Driver;
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
}
