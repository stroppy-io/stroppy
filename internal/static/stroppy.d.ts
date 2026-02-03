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

  // Driver interface - provides database operations
  export interface Driver {
    insertValuesBin(insert: BinMsg<InsertDescriptor>): Error | QueryStats;
    runQuery(sql: string, args: Record<string, any>): Error | QueryStats;
  }

  // Generator interface - provides data generation
  export interface Generator {
    next(): any;
  }

  // k6 module functions provided by Go module
  export declare function NotifyStep(name: String, status: Number): void;
  export declare function Teardown(): Error;
  export declare function NewDriverByConfigBin(
    configBin: BinMsg<GlobalConfig>,
  ): Driver;
  export declare function NewGeneratorByRuleBin(
    seed: Number,
    rule: BinMsg<Generation_Rule>,
  ): Generator;
  export declare function NewGroupGeneratorByRulesBin(
    seed: Number,
    rule: BinMsg<QueryParamGroup>,
  ): Generator;
}
