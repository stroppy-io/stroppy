import { Counter, Rate, Trend } from "k6/metrics";
import encoding from "k6/x/encoding";
globalThis.TextEncoder = encoding.TextEncoder;
globalThis.TextDecoder = encoding.TextDecoder;

import {
  NewDriverByConfigBin,
  NewGeneratorByRuleBin,
  NewGroupGeneratorByRulesBin,
  Generator,
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

interface InsertDescriptorX {
  method?: InsertMethod;
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
          method: insert?.method,
          seed: insert?.seed ?? _seed,
          params: R.params(insert?.params ?? {}),
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

// Generator wrapper functions - provide convenient protobuf-based API
export function NewGen(
  seed: number,
  rule: Partial<Generation_Rule>,
): Generator {
  return NewGeneratorByRuleBin(
    seed,
    Generation_Rule.toBinary(Generation_Rule.create(rule)),
  );
}

export function NewGroupGen(
  rules: Partial<QueryParamGroup>,
  seed?: number,
): Generator {
  return NewGroupGeneratorByRulesBin(
    seed ?? _seed,
    QueryParamGroup.toBinary(QueryParamGroup.create(rules)),
  );
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
interface RandomRangeGenerators {
  // String
  str(val: string): Rule;
  str(len: number, alphabet?: Alphabet): Rule;
  str(minLen: number, maxLen: number, alphabet?: Alphabet): Rule;

  // Signed integers
  int32(val: number): Rule;
  int32(min: number, max: number, distribution?: Distribution): Rule;

  int64(val: string | number): Rule;
  int64(min: string | number, max: string | number, distribution?: Distribution): Rule;

  // Unsigned integers
  uint32(val: number): Rule;
  uint32(min: number, max: number, distribution?: Distribution): Rule;

  uint64(val: string | number): Rule;
  uint64(min: string | number, max: string | number, distribution?: Distribution): Rule;

  // Floating point
  float(val: number): Rule;
  float(min: number, max: number, distribution?: Distribution): Rule;

  double(val: number): Rule;
  double(min: number, max: number, distribution?: Distribution): Rule;

  // Decimal (arbitrary precision)
  decimal(val: string): Rule;
  decimal(min: number, max: number, distribution?: Distribution): Rule;

  // Datetime
  datetimeConst: (val: Date) => Rule;
  datetime(min: Date, max: Date, distribution?: Distribution): Rule;

  // Boolean
  bool: (ratio: number, unique?: boolean) => Rule;
  boolConst: (val: boolean) => Rule;

  // UUID
  uuid: () => Rule;
  uuidSeeded: () => Rule;
  uuidConst: (val: string) => Rule;

  // Helpers
  params: (params: Record<string, Generation_Rule>) => QueryParamDescriptor[];
  groups: (
    groups: Record<string, Record<string, Generation_Rule>>,
  ) => QueryParamGroup[];
}

export const R: RandomRangeGenerators = {
  str(
    valOrMin: string | number,
    alphabetOrMax?: Alphabet | number,
    alphabet: Alphabet = AB.en,
  ): Rule {
    if (typeof valOrMin === "string") {
      return rule({ kind: { oneofKind: "stringConst", stringConst: valOrMin } });
    }

    const isRange = typeof alphabetOrMax === "number";
    const minLen = valOrMin;
    const maxLen = isRange ? alphabetOrMax : valOrMin;
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

  int32(valOrMin: number, max?: number, distribution?: Distribution): Rule {
    if (max === undefined) {
      return rule({ kind: { oneofKind: "int32Const", int32Const: valOrMin } });
    }
    return rule({
      kind: { oneofKind: "int32Range", int32Range: { min: valOrMin, max } },
      distribution: distribution && toProtoDistribution(distribution),
    });
  },

  float(valOrMin: number, max?: number, distribution?: Distribution): Rule {
    if (max === undefined) {
      return rule({ kind: { oneofKind: "floatConst", floatConst: valOrMin } });
    }
    return rule({
      kind: { oneofKind: "floatRange", floatRange: { min: valOrMin, max } },
      distribution: distribution && toProtoDistribution(distribution),
    });
  },

  double(valOrMin: number, max?: number, distribution?: Distribution): Rule {
    if (max === undefined) {
      return rule({ kind: { oneofKind: "doubleConst", doubleConst: valOrMin } });
    }
    return rule({
      kind: { oneofKind: "doubleRange", doubleRange: { min: valOrMin, max } },
      distribution: distribution && toProtoDistribution(distribution),
    });
  },

  int64(valOrMin: string | number, max?: string | number, distribution?: Distribution): Rule {
    if (max === undefined) {
      return rule({ kind: { oneofKind: "int64Const", int64Const: String(valOrMin) } });
    }
    return rule({
      kind: { oneofKind: "int64Range", int64Range: { min: String(valOrMin), max: String(max) } },
      distribution: distribution && toProtoDistribution(distribution),
    });
  },

  uint32(valOrMin: number, max?: number, distribution?: Distribution): Rule {
    if (max === undefined) {
      return rule({ kind: { oneofKind: "uint32Const", uint32Const: valOrMin } });
    }
    return rule({
      kind: { oneofKind: "uint32Range", uint32Range: { min: valOrMin, max } },
      distribution: distribution && toProtoDistribution(distribution),
    });
  },

  uint64(valOrMin: string | number, max?: string | number, distribution?: Distribution): Rule {
    if (max === undefined) {
      return rule({ kind: { oneofKind: "uint64Const", uint64Const: String(valOrMin) } });
    }
    return rule({
      kind: { oneofKind: "uint64Range", uint64Range: { min: String(valOrMin), max: String(max) } },
      distribution: distribution && toProtoDistribution(distribution),
    });
  },

  decimal(valOrMin: string | number, max?: number, distribution?: Distribution): Rule {
    if (max === undefined) {
      return rule({ kind: { oneofKind: "decimalConst", decimalConst: { value: String(valOrMin) } } });
    }
    return rule({
      kind: {
        oneofKind: "decimalRange",
        decimalRange: { type: { oneofKind: "double", double: { min: valOrMin as number, max } } },
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

  boolConst(val: boolean): Rule {
    return rule({ kind: { oneofKind: "boolConst", boolConst: val } });
  },

  datetimeConst(val: Date): Rule {
    return rule({
      kind: {
        oneofKind: "datetimeConst",
        datetimeConst: {
          value: {
            seconds: val.getSeconds().toString(),
            nanos: (val.getMilliseconds() % 1000) * 1000000,
          },
        },
      },
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

  uuidConst(val: string): Rule {
    return rule({ kind: { oneofKind: "uuidConst", uuidConst: { value: val } } });
  },

  params: params_internal,

  groups(
    groups: Record<string, Record<string, Generation_Rule>>,
  ): QueryParamGroup[] {
    return Object.entries(groups).map(([name, params]) =>
      QueryParamGroup.create({ name, params: params_internal(params) }),
    );
  },
};

interface SequenceGenerators {
  // String generators
  str(len: number, alphabet?: Alphabet): Rule;
  str(minLen: number, maxLen: number, alphabet?: Alphabet): Rule;

  int32: (min: number, max: number) => Rule;
  int64: (min: string | number, max: string | number) => Rule;
  uint32: (min: number, max: number) => Rule;
  uint64: (min: string | number, max: string | number) => Rule;

  // Sequential UUID: counts from min up to max (inclusive).
  // min defaults to 00000000-0000-0000-0000-000000000000 if omitted.
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

  int64(min: string | number, max: string | number): Rule {
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

  uint64(min: string | number, max: string | number): Rule {
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

function params_internal(
  params: Record<string, Generation_Rule>,
): QueryParamDescriptor[] {
  return Object.entries(params).map(([name, generationRule]) =>
    QueryParamDescriptor.create({ name, generationRule }),
  );
}
