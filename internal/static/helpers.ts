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
  Driver,
  Generator,
  NotifyStep,
} from "k6/x/stroppy";

interface InsertDescriptorX {
  method: InsertMethod;
  params?: Record<string, Generation_Rule>;
  groups?: Record<string, QueryParamDescriptor[]>;
}

export function InsertValues(
  driver: Driver,
  insert: Partial<InsertDescriptor>,
): void;

export function InsertValues(
  driver: Driver,
  tableName: string,
  count: number,
  insert: InsertDescriptorX,
): void;

export function InsertValues(
  driver: Driver,
  insertOrTableName: string | Partial<InsertDescriptor>,
  count?: number,
  insert?: InsertDescriptorX,
) {
  const isName = typeof insertOrTableName === "string";
  const descriptor = isName
    ? {
        tableName: insertOrTableName,
        method: insert?.method,
        params: G.params(insert?.params ?? {}),
        groups: G.groups(insert?.groups ?? {}),
        count,
      }
    : insertOrTableName;
  console.log(
    `Insertion into '${descriptor.tableName}' of ${descriptor.count} values starting...`,
  );
  const err = driver.insertValuesBin(
    InsertDescriptor.toBinary(InsertDescriptor.create(descriptor)),
  );
  if (err) {
    throw err;
  }
  console.log(`Insertion into '${descriptor.tableName}' ended`);
}

export function StepBlock(name: string, block: () => void): void {
  NotifyStep(name, Status.STATUS_RUNNING);
  console.log(`Start of '${name}' block`);
  block();
  console.log(`End of '${name}' block`);
  NotifyStep(name, Status.STATUS_COMPLETED);
}

export function NewDriverByConfig(config: Partial<GlobalConfig>): Driver {
  return NewDriverByConfigBin(
    GlobalConfig.toBinary(GlobalConfig.create(config)),
  );
}
// Generator wrapper functions - provide convenient protobuf-based API
export function NewGeneratorByRule(
  seed: Number,
  rule: Partial<Generation_Rule>,
): Generator {
  return NewGeneratorByRuleBin(
    seed,
    Generation_Rule.toBinary(Generation_Rule.create(rule)),
  );
}

export function NewGroupGeneratorByRules(
  seed: Number,
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
interface GHelper {
  // Overloaded str function
  str(val: string): Generation_Rule;
  str(len: number, alphabet?: Alphabet): Generation_Rule;
  str(minLen: number, maxLen: number, alphabet?: Alphabet): Generation_Rule;

  // String generators
  strSeq(len: number, alphabet?: Alphabet): Generation_Rule;
  strSeq(minLen: number, maxLen: number, alphabet?: Alphabet): Generation_Rule;

  // Integer generators
  int32(val: number): Generation_Rule;
  int32(min: number, max: number): Generation_Rule;
  int32Seq: (min: number, max: number) => Generation_Rule;

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
  groups: (groups: Record<string, QueryParamDescriptor[]>) => QueryParamGroup[];
}

export const G: GHelper = {
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

  strSeq(
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

  int32(valOrMin: number, max?: number): Generation_Rule {
    if (max === undefined) {
      return { kind: { oneofKind: "int32Const", int32Const: valOrMin } };
    }
    return {
      kind: { oneofKind: "int32Range", int32Range: { min: valOrMin, max } },
    };
  },

  int32Seq(min: number, max: number): Generation_Rule {
    return {
      kind: { oneofKind: "int32Range", int32Range: { min, max } },
      unique: true,
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

  params(params: Record<string, Generation_Rule>): QueryParamDescriptor[] {
    return Object.entries(params).map(([name, generationRule]) =>
      QueryParamDescriptor.create({ name, generationRule }),
    );
  },

  groups(groups: Record<string, QueryParamDescriptor[]>): QueryParamGroup[] {
    return Object.entries(groups).map(([name, params]) =>
      QueryParamGroup.create({ name, params }),
    );
  },
};
