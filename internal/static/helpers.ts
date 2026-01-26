import {
  Generation_Rule,
  QueryParamGroup,
  GlobalConfig,
  QueryParamDescriptor,
} from "./stroppy.pb.js";
import type { Generator, Driver } from "./stroppy.d.ts";

import {
  NewDriverByConfigBin,
  NewGeneratorByRuleBin,
  NewGroupGeneratorByRulesBin,
} from "k6/x/stroppy";

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

export const G = {
  // String generators
  str: (len: number, alphabet = AB.en): Generation_Rule => ({
    kind: {
      oneofKind: "stringRange",
      stringRange: {
        minLen: len.toString(),
        maxLen: len.toString(),
        alphabet: { ranges: alphabet },
      },
    },
  }),

  strRange: (
    minLen: number,
    maxLen: number,
    alphabet = AB.en,
  ): Generation_Rule => ({
    kind: {
      oneofKind: "stringRange",
      stringRange: {
        minLen: minLen.toString(),
        maxLen: maxLen.toString(),
        alphabet: { ranges: alphabet },
      },
    },
  }),

  strSeq: (
    minLen: number,
    maxLen: number,
    alphabet = AB.en,
  ): Generation_Rule => ({
    kind: {
      oneofKind: "stringRange",
      stringRange: {
        minLen: minLen.toString(),
        maxLen: maxLen.toString(),
        alphabet: { ranges: alphabet },
      },
    },
    unique: true,
  }),

  strConst: (val: string): Generation_Rule => ({
    kind: { oneofKind: "stringConst", stringConst: val },
  }),

  // Integer generators
  int32: (min: number, max: number): Generation_Rule => ({
    kind: { oneofKind: "int32Range", int32Range: { min, max } },
  }),

  int32Seq: (min: number, max: number): Generation_Rule => ({
    kind: { oneofKind: "int32Range", int32Range: { min, max } },
    unique: true,
  }),

  int32Const: (val: number): Generation_Rule => ({
    kind: { oneofKind: "int32Const", int32Const: val },
  }),

  // Float/Double generators
  float: (min: number, max: number): Generation_Rule => ({
    kind: { oneofKind: "floatRange", floatRange: { min, max } },
  }),

  floatConst: (val: number): Generation_Rule => ({
    kind: { oneofKind: "floatConst", floatConst: val },
  }),

  double: (min: number, max: number): Generation_Rule => ({
    kind: { oneofKind: "doubleRange", doubleRange: { min, max } },
  }),

  // Datetime generator
  datetimeConst: (val: Date): Generation_Rule => ({
    kind: {
      oneofKind: "datetimeConst",
      datetimeConst: {
        value: {
          seconds: val.getSeconds().toString(),
          nanos: (val.getMilliseconds() % 1000) * 1000000,
        },
      },
    },
  }),
};

// Helper to convert params object to array
export const paramsG = (
  params: Record<string, Generation_Rule>,
): QueryParamDescriptor[] => {
  return Object.entries(params).map(([name, generationRule]) =>
    QueryParamDescriptor.create({ name, generationRule }),
  );
};
