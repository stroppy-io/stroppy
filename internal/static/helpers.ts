import {
  Generation_Rule,
  QueryParamGroup,
  GlobalConfig,
  QueryParamDescriptor,
  InsertDescriptor,
  InsertMethod,
  Status,
} from "./stroppy.pb.js";

import {
  NewDriverByConfigBin,
  NewGeneratorByRuleBin,
  NewGroupGeneratorByRulesBin,
  Generator,
  NotifyStep,
  Driver,
  QueryStats,
} from "k6/x/stroppy";

import { Counter, Rate, Trend } from "k6/metrics";
import { ParsedQuery } from "./parse_sql.js";

interface InsertDescriptorX {
  method: InsertMethod;
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


export function Step(name: string, block: () => void): void {
  NotifyStep(name, Status.STATUS_RUNNING);
  console.log(`Start of '${name}' block`);
  block();
  console.log(`End of '${name}' block`);
  NotifyStep(name, Status.STATUS_COMPLETED);
}

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
  seed: number,
  rules: Partial<QueryParamGroup>,
): Generator {
  return NewGroupGeneratorByRulesBin(
    seed,
    QueryParamGroup.toBinary(QueryParamGroup.create(rules)),
  );
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
  // Overloaded str function
  str(val: string): Generation_Rule;
  str(len: number, alphabet?: Alphabet): Generation_Rule;
  str(minLen: number, maxLen: number, alphabet?: Alphabet): Generation_Rule;

  // Integer generators
  int32(val: number): Generation_Rule;
  int32(min: number, max: number): Generation_Rule;

  // Float/Double generators
  float(val: number): Generation_Rule;
  float(min: number, max: number): Generation_Rule;

  double(val: number): Generation_Rule;
  double(min: number, max: number): Generation_Rule;

  // Datetime generator
  datetimeConst: (val: Date) => Generation_Rule;

  // Boolean generator
  bool: (ratio: number, unique?: boolean) => Generation_Rule;

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
  ): Generation_Rule {
    if (typeof valOrMin === "string") {
      return { kind: { oneofKind: "stringConst", stringConst: valOrMin } };
    }

    const isRange = typeof alphabetOrMax === "number";
    const minLen = valOrMin;
    const maxLen = isRange ? alphabetOrMax : valOrMin;
    const alph = isRange ? alphabet : (alphabetOrMax ?? AB.en);

    return {
      kind: {
        oneofKind: "stringRange",
        stringRange: {
          minLen: minLen.toString(),
          maxLen: maxLen.toString(),
          alphabet: { ranges: alph },
        },
      },
    };
  },

  int32(valOrMin: number, max?: number): Generation_Rule {
    if (max === undefined) {
      return { kind: { oneofKind: "int32Const", int32Const: valOrMin } };
    }
    return {
      kind: { oneofKind: "int32Range", int32Range: { min: valOrMin, max } },
    };
  },

  float(valOrMin: number, max?: number): Generation_Rule {
    if (max === undefined) {
      return { kind: { oneofKind: "floatConst", floatConst: valOrMin } };
    }
    return {
      kind: { oneofKind: "floatRange", floatRange: { min: valOrMin, max } },
    };
  },

  double(valOrMin: number, max?: number): Generation_Rule {
    if (max === undefined) {
      return { kind: { oneofKind: "doubleConst", doubleConst: valOrMin } };
    }
    return {
      kind: { oneofKind: "doubleRange", doubleRange: { min: valOrMin, max } },
    };
  },

  datetimeConst(val: Date): Generation_Rule {
    return {
      kind: {
        oneofKind: "datetimeConst",
        datetimeConst: {
          value: {
            seconds: val.getSeconds().toString(),
            nanos: (val.getMilliseconds() % 1000) * 1000000,
          },
        },
      },
    };
  },

  // ratio of true values
  // unique = true => sequence [false, true]
  bool(ratio: number, unique = false): Generation_Rule {
    return {
      kind: { oneofKind: "boolRange", boolRange: { ratio } },
      unique: unique,
    };
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
  str(len: number, alphabet?: Alphabet): Generation_Rule;
  str(minLen: number, maxLen: number, alphabet?: Alphabet): Generation_Rule;

  int32: (min: number, max: number) => Generation_Rule;
}

export const S: SequenceGenerators = {
  str(
    lenOrMin: number,
    alphabetOrMax?: Alphabet | number,
    alphabet: Alphabet = AB.en,
  ): Generation_Rule {
    const isRange = typeof alphabetOrMax === "number";
    const minLen = lenOrMin;
    const maxLen = isRange ? alphabetOrMax : lenOrMin;
    const alph = isRange ? alphabet : (alphabetOrMax ?? AB.en);

    return {
      kind: {
        oneofKind: "stringRange",
        stringRange: {
          minLen: minLen.toString(),
          maxLen: maxLen.toString(),
          alphabet: { ranges: alph },
        },
      },
      unique: true,
    };
  },

  int32(min: number, max: number): Generation_Rule {
    return {
      kind: { oneofKind: "int32Range", int32Range: { min, max } },
      unique: true,
    };
  },
};

function params_internal(
  params: Record<string, Generation_Rule>,
): QueryParamDescriptor[] {
  return Object.entries(params).map(([name, generationRule]) =>
    QueryParamDescriptor.create({ name, generationRule }),
  );
}
