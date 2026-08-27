// JSON Schema for the Chrome Prompt API's `responseConstraint` option
// (developer.chrome.com/docs/ai/structured-output-for-prompt-api). Chrome's
// documented examples only use plain object/array/string/number/boolean/enum
// with `required` and `additionalProperties: false` — no `oneOf`/`anyOf`/
// `$ref`. So instead of a discriminated union keyed by `type`, every
// type-specific sub-object is listed as an optional property, mirroring how
// `PolicyCfg` itself is shaped (see policyDefaults.ts's fieldsForType).
import { POLICY_TYPES, SUB_POLICY_TYPES } from "../policyDefaults";

const STATUS_CODE_ENUM = ["OK", "ERROR", "UNSET"];

const COMMON_FIELD_PROPERTIES = {
  latency: {
    type: "object",
    properties: {
      threshold_ms: { type: "number" },
      upper_threshold_ms: { type: "number" },
    },
    required: ["threshold_ms"],
  },
  numeric_attribute: {
    type: "object",
    properties: {
      key: { type: "string" },
      min_value: { type: "number" },
      max_value: { type: "number" },
      invert_match: { type: "boolean" },
    },
    required: ["key", "min_value", "max_value"],
  },
  probabilistic: {
    type: "object",
    properties: {
      sampling_percentage: { type: "number" },
    },
    required: ["sampling_percentage"],
  },
  status_code: {
    type: "object",
    properties: {
      status_codes: { type: "array", items: { type: "string", enum: STATUS_CODE_ENUM } },
    },
    required: ["status_codes"],
  },
  string_attribute: {
    type: "object",
    properties: {
      key: { type: "string" },
      values: { type: "array", items: { type: "string" } },
      enabled_regex_matching: { type: "boolean" },
      invert_match: { type: "boolean" },
    },
    required: ["key", "values"],
  },
  rate_limiting: {
    type: "object",
    properties: {
      spans_per_second: { type: "number" },
    },
    required: ["spans_per_second"],
  },
  boolean_attribute: {
    type: "object",
    properties: {
      key: { type: "string" },
      value: { type: "boolean" },
      invert_match: { type: "boolean" },
    },
    required: ["key", "value"],
  },
  span_count: {
    type: "object",
    properties: {
      min_spans: { type: "number" },
      max_spans: { type: "number" },
    },
    required: ["min_spans"],
  },
  trace_state: {
    type: "object",
    properties: {
      key: { type: "string" },
      values: { type: "array", items: { type: "string" } },
    },
    required: ["key", "values"],
  },
  // spanevent is deliberately omitted: the preview server rejects it (it
  // doesn't retain span event data), so the model should never propose it.
  ottl_condition: {
    type: "object",
    properties: {
      error_mode: { type: "string", enum: ["ignore", "propagate", "silent"] },
      span: { type: "array", items: { type: "string" } },
    },
    required: ["error_mode", "span"],
  },
} as const;

// Sub-policies inside an "and" can be any policy type except "and" itself —
// matches the existing UI constraint (AndSubPolicyList / SUB_POLICY_TYPES).
const SUB_POLICY_SCHEMA = {
  type: "object",
  properties: {
    name: { type: "string" },
    type: { type: "string", enum: [...SUB_POLICY_TYPES] },
    ...COMMON_FIELD_PROPERTIES,
  },
  required: ["name", "type"],
  additionalProperties: false,
};

const POLICY_SCHEMA = {
  type: "object",
  properties: {
    name: { type: "string" },
    type: { type: "string", enum: [...POLICY_TYPES] },
    ...COMMON_FIELD_PROPERTIES,
    and: {
      type: "object",
      properties: {
        and_sub_policy: { type: "array", items: SUB_POLICY_SCHEMA },
      },
      required: ["and_sub_policy"],
    },
  },
  required: ["name", "type"],
  additionalProperties: false,
};

export const POLICY_GENERATION_SCHEMA = {
  type: "object",
  properties: {
    policies: { type: "array", items: POLICY_SCHEMA },
    rationale: { type: "string" },
  },
  required: ["policies"],
  additionalProperties: false,
};
