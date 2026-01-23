import {
  Generation_Rule,
  QueryParamGroup,
  GlobalConfig,
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
