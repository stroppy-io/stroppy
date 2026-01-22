// Type declarations for k6/x/stroppy Go module
// These declarations provide TypeScript/LSP support for the k6 module API

import type {
  UnitDescriptor,
  DriverTransactionStat,
  InsertDescriptor,
} from "./stroppy.pb.js";

// protobuf serialized messages - type-safe wrapper around Uint8Array
export type BinMsg<_T extends any> = Uint8Array;

// Driver interface - provides database operations
export interface Driver {
  runUnit(unit: BinMsg<UnitDescriptor>): BinMsg<DriverTransactionStat>;
  insertValues(
    insert: BinMsg<InsertDescriptor>,
    count: number,
  ): BinMsg<DriverTransactionStat>;
  runQuery(sql: string, args: Record<string, any>): void; // TODO: return value, is it posible to make it generic?
}

// Generator interface - provides data generation
export interface Generator {
  next(): any;
}

// k6 module functions provided by Go module
export declare function NewDriverByConfig(configBin: BinMsg<any>): Driver;
export declare function NotifyStep(name: String, status: Number): void;
export declare function Teardown(): Error;
export declare function NewGeneratorByRuleBin(
  seed: Number,
  rule: BinMsg<any>,
): Generator;
export declare function NewGroupGeneratorByRulesBin(
  seed: Number,
  rule: BinMsg<any>,
): Generator;

// k6 environment variables
export declare const __ENV: Record<string, string | undefined>;
export declare const __SQL_FILE: string;
