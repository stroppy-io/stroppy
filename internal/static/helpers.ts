import { Counter, Rate, Trend } from "k6/metrics";
import encoding from "k6/x/encoding";
globalThis.TextEncoder = encoding.TextEncoder;
globalThis.TextDecoder = encoding.TextDecoder;

import {
  NewDriverByConfigBin,
  NewGeneratorByRuleBin,
  NewGroupGeneratorByRulesBin,
  NotifyStep,
  Driver,
  QueryStats,
} from "k6/x/stroppy";
import {
  Generation_Rule,
  Generation_Distribution,
  Generation_Distribution_DistributionType,
  QueryParamGroup,
  GlobalConfig,
  QueryParamDescriptor,
  InsertDescriptor,
  InsertMethod,
  Status,
  Timestamp,
} from "./stroppy.pb.js";
import { ParsedQuery } from "./parse_sql.js";


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

export type InsertMethodName = "plain_query" | "copy_from";

const insertMethodMap: Record<InsertMethodName, InsertMethod> = {
  plain_query: InsertMethod.PLAIN_QUERY,
  copy_from: InsertMethod.COPY_FROM,
};

interface InsertDescriptorX {
  method?: InsertMethodName;
  seed?: number;
  params?: Record<string, Generation_Rule>;
  groups?: Record<string, Record<string, Generation_Rule>>;
}

const insertMetric = new Trend("insert_duration", true);
const insertErrRateMetric = new Rate("insert_error_rate");
const runQueryMetric = new Trend("run_query_duration", true);
const runQueryCounterMetric = new Counter("run_query_count");
const runQueryErrRateMetric = new Rate("run_query_error_rate");

function isQueryStats(obj: any): obj is QueryStats {
  return typeof obj.elapsed !== "undefined"
}

export class DriverX {
  private driver: Driver;

  constructor(driver: Driver) {
    this.driver = driver;
  }

  static fromConfig(config: Partial<GlobalConfig>): DriverX {
    if (config.seed) setSeed(Number(config.seed));
    const driver = NewDriverByConfigBin(
      GlobalConfig.toBinary(GlobalConfig.create(config)),
    );
    return new DriverX(driver);
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
          method: insert?.method ? insertMethodMap[insert.method] : undefined,
          seed: insert?.seed ?? _seed,
          params: R.group(insert?.params ?? {}),
          groups: R.groups(insert?.groups ?? {}),
          count,
        }
      : insertOrTableName;

    console.log(
      `Insertion into '${descriptor.tableName}' of ${descriptor.count} values starting...`,
    );

    const results = this.driver.insertValuesBin(
      InsertDescriptor.toBinary(InsertDescriptor.create(descriptor)),
    );

    const tags = { table_name: descriptor.tableName ?? "unknown" };
    if (!results) return 
    if (isQueryStats(results)) {
      insertErrRateMetric.add(0, tags);
      insertMetric.add(results.elapsed.milliseconds(), tags);
    } else {
      insertErrRateMetric.add(1, tags);
    }

    console.log(`Insertion into '${descriptor.tableName}' ended`);
  }

  runQuery(sql: string, args: Record<string, any>): void;
  runQuery(query: ParsedQuery, args: Record<string, any>): void;
  runQuery(sqlOrQuery: string | ParsedQuery, args: Record<string, any>): void {
    const isSql = typeof sqlOrQuery === "string";
    const result = this.driver.runQuery(
      isSql ? sqlOrQuery : sqlOrQuery.sql,
      args,
    );

    const tags = isSql
      ? undefined
      : { name: sqlOrQuery.name, type: sqlOrQuery.type };

    if (!result) return 
    if (isQueryStats(result)) {
      runQueryMetric.add(result.elapsed.milliseconds(), tags);
      runQueryErrRateMetric.add(0, tags);
      runQueryCounterMetric.add(1, tags);
    } else {
      runQueryErrRateMetric.add(1, tags);
    }
  }

  // Expose the underlying driver if needed for advanced usage
  getDriver(): Driver {
    return this.driver;
  }
}

export function NewDriverByConfig(config: Partial<GlobalConfig>): Driver {
  return NewDriverByConfigBin(
    GlobalConfig.toBinary(GlobalConfig.create(config)),
  );
}

export const Step = Object.assign(
  (name: string, step: () => void): void => {
    Step.begin(name);
    step();
    Step.end(name);
  },
  {
    begin: (name: string): void => {
      NotifyStep(name, Status.STATUS_RUNNING);
      console.log(`Start of '${name}' step`);
    },
    end: (name: string): void => {
      console.log(`End of '${name}' step`);
      NotifyStep(name, Status.STATUS_COMPLETED);
    },
  }
);

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
