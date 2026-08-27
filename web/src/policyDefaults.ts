// Default values and field-reset helpers for the Policy Builder. Exactly
// one of PolicyCfg's type-specific sub-objects should be set at a time
// (matching `type`), so switching a policy's type must clear every other
// sub-object while filling in a sensible default for the new one.
import type {
  AndSubPolicyCfg,
  BooleanAttributeCfg,
  LatencyCfg,
  NumericAttributeCfg,
  OTTLConditionCfg,
  PolicyType,
  ProbabilisticCfg,
  RateLimitingCfg,
  SpanCountCfg,
  StatusCodeCfg,
  StringAttributeCfg,
  TraceStateCfg,
} from "./types";

export interface CommonFields {
  latency?: LatencyCfg;
  numeric_attribute?: NumericAttributeCfg;
  probabilistic?: ProbabilisticCfg;
  status_code?: StatusCodeCfg;
  string_attribute?: StringAttributeCfg;
  rate_limiting?: RateLimitingCfg;
  boolean_attribute?: BooleanAttributeCfg;
  span_count?: SpanCountCfg;
  trace_state?: TraceStateCfg;
  ottl_condition?: OTTLConditionCfg;
}

export const EMPTY_COMMON_FIELDS: Required<CommonFields> = {
  latency: undefined as unknown as LatencyCfg,
  numeric_attribute: undefined as unknown as NumericAttributeCfg,
  probabilistic: undefined as unknown as ProbabilisticCfg,
  status_code: undefined as unknown as StatusCodeCfg,
  string_attribute: undefined as unknown as StringAttributeCfg,
  rate_limiting: undefined as unknown as RateLimitingCfg,
  boolean_attribute: undefined as unknown as BooleanAttributeCfg,
  span_count: undefined as unknown as SpanCountCfg,
  trace_state: undefined as unknown as TraceStateCfg,
  ottl_condition: undefined as unknown as OTTLConditionCfg,
};

export function commonDefaultsForType(type: PolicyType): CommonFields {
  switch (type) {
    case "latency":
      return { latency: { threshold_ms: 1000 } };
    case "numeric_attribute":
      return { numeric_attribute: { key: "", min_value: 0, max_value: 0 } };
    case "probabilistic":
      return { probabilistic: { sampling_percentage: 10 } };
    case "status_code":
      return { status_code: { status_codes: ["ERROR"] } };
    case "string_attribute":
      return { string_attribute: { key: "", values: [] } };
    case "rate_limiting":
      return { rate_limiting: { spans_per_second: 100 } };
    case "boolean_attribute":
      return { boolean_attribute: { key: "", value: true } };
    case "span_count":
      return { span_count: { min_spans: 1 } };
    case "trace_state":
      return { trace_state: { key: "", values: [] } };
    case "ottl_condition":
      return { ottl_condition: { error_mode: "ignore", span: [] } };
    default:
      return {};
  }
}

// fieldsForType returns the full sub-object patch (every other field
// cleared) for a top-level policy of the given type.
export function fieldsForType(type: PolicyType): CommonFields & { and?: { and_sub_policy: AndSubPolicyCfg[] } } {
  if (type === "and") {
    return { ...EMPTY_COMMON_FIELDS, and: { and_sub_policy: [] } };
  }
  return { ...EMPTY_COMMON_FIELDS, and: undefined, ...commonDefaultsForType(type) };
}

// subFieldsForType is the same reset for an "and" sub-policy, which can
// never itself be type "and".
export function subFieldsForType(type: AndSubPolicyCfg["type"]): CommonFields {
  return { ...EMPTY_COMMON_FIELDS, ...commonDefaultsForType(type) };
}

export const POLICY_TYPES: PolicyType[] = [
  "always_sample",
  "latency",
  "status_code",
  "numeric_attribute",
  "string_attribute",
  "boolean_attribute",
  "probabilistic",
  "rate_limiting",
  "span_count",
  "trace_state",
  "ottl_condition",
  "and",
];

export const SUB_POLICY_TYPES: Array<AndSubPolicyCfg["type"]> = POLICY_TYPES.filter(
  (t): t is AndSubPolicyCfg["type"] => t !== "and",
);

export function splitList(text: string): string[] {
  return text
    .split(",")
    .map((s) => s.trim())
    .filter((s) => s.length > 0);
}

export function splitLines(text: string): string[] {
  return text
    .split("\n")
    .map((s) => s.trim())
    .filter((s) => s.length > 0);
}
