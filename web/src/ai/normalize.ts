// Normalizes the LLM's raw (untrusted) JSON output into valid PolicyCfg[].
// Reuses commonDefaultsForType (policyDefaults.ts) as the fallback for any
// missing/malformed field, and always rebuilds each policy as a fresh
// object with exactly one type-specific sub-object set — the same
// invariant fieldsForType enforces for user-driven edits.
import { POLICY_TYPES, SUB_POLICY_TYPES, commonDefaultsForType } from "../policyDefaults";
import type { AndSubPolicyCfg, PolicyCfg, PolicyType, StatusCode } from "../types";

const STATUS_CODES: readonly StatusCode[] = ["OK", "ERROR", "UNSET"];
const OTTL_ERROR_MODES = ["ignore", "propagate", "silent"] as const;

function isPolicyType(v: unknown): v is PolicyType {
  return typeof v === "string" && (POLICY_TYPES as readonly string[]).includes(v);
}

function isSubPolicyType(v: unknown): v is AndSubPolicyCfg["type"] {
  return typeof v === "string" && (SUB_POLICY_TYPES as readonly string[]).includes(v);
}

function rawObj(v: unknown): Record<string, unknown> {
  return v && typeof v === "object" ? (v as Record<string, unknown>) : {};
}

function num(v: unknown, fallback: number): number {
  const n = Number(v);
  return Number.isFinite(n) ? n : fallback;
}

function numOrUndef(v: unknown): number | undefined {
  if (v === undefined || v === null || v === "") return undefined;
  const n = Number(v);
  return Number.isFinite(n) ? n : undefined;
}

function boolOrUndef(v: unknown): boolean | undefined {
  return typeof v === "boolean" ? v : undefined;
}

function str(v: unknown, fallback: string): string {
  return typeof v === "string" ? v : fallback;
}

function strArray(v: unknown): string[] {
  return Array.isArray(v) ? v.filter((x): x is string => typeof x === "string") : [];
}

function statusCodeArray(v: unknown, fallback: StatusCode[]): StatusCode[] {
  if (!Array.isArray(v)) return fallback;
  const filtered = v.filter((x): x is StatusCode => STATUS_CODES.includes(x as StatusCode));
  return filtered.length > 0 ? filtered : fallback;
}

// field key on the raw object always equals the policy type's name (e.g.
// type "latency" reads raw.latency) — same mapping as PolicyCfg itself.
function normalizeCommonFields(type: PolicyType | AndSubPolicyCfg["type"], raw: Record<string, unknown>) {
  const defaults = commonDefaultsForType(type);
  const r = rawObj(raw[type]);

  switch (type) {
    case "latency":
      return {
        latency: {
          threshold_ms: num(r.threshold_ms, defaults.latency!.threshold_ms),
          upper_threshold_ms: numOrUndef(r.upper_threshold_ms),
        },
      };
    case "numeric_attribute":
      return {
        numeric_attribute: {
          key: str(r.key, defaults.numeric_attribute!.key),
          min_value: num(r.min_value, defaults.numeric_attribute!.min_value),
          max_value: num(r.max_value, defaults.numeric_attribute!.max_value),
          invert_match: boolOrUndef(r.invert_match),
        },
      };
    case "probabilistic":
      return {
        probabilistic: {
          sampling_percentage: num(r.sampling_percentage, defaults.probabilistic!.sampling_percentage),
          hash_salt: typeof r.hash_salt === "string" ? r.hash_salt : undefined,
        },
      };
    case "status_code":
      return { status_code: { status_codes: statusCodeArray(r.status_codes, defaults.status_code!.status_codes) } };
    case "string_attribute":
      return {
        string_attribute: {
          key: str(r.key, defaults.string_attribute!.key),
          values: strArray(r.values),
          enabled_regex_matching: boolOrUndef(r.enabled_regex_matching),
          invert_match: boolOrUndef(r.invert_match),
        },
      };
    case "rate_limiting":
      return {
        rate_limiting: { spans_per_second: num(r.spans_per_second, defaults.rate_limiting!.spans_per_second) },
      };
    case "boolean_attribute":
      return {
        boolean_attribute: {
          key: str(r.key, defaults.boolean_attribute!.key),
          value: typeof r.value === "boolean" ? r.value : defaults.boolean_attribute!.value,
          invert_match: boolOrUndef(r.invert_match),
        },
      };
    case "span_count":
      return {
        span_count: {
          min_spans: num(r.min_spans, defaults.span_count!.min_spans),
          max_spans: numOrUndef(r.max_spans),
        },
      };
    case "trace_state":
      return { trace_state: { key: str(r.key, defaults.trace_state!.key), values: strArray(r.values) } };
    case "ottl_condition": {
      const errorMode = OTTL_ERROR_MODES.includes(r.error_mode as (typeof OTTL_ERROR_MODES)[number])
        ? (r.error_mode as (typeof OTTL_ERROR_MODES)[number])
        : defaults.ottl_condition!.error_mode;
      return { ottl_condition: { error_mode: errorMode, span: strArray(r.span) } };
    }
    default:
      return {};
  }
}

function normalizeSubPolicy(raw: unknown, index: number): AndSubPolicyCfg {
  const r = rawObj(raw);
  const type = isSubPolicyType(r.type) ? r.type : "always_sample";
  return {
    name: str(r.name, `condition-${index + 1}`),
    type,
    ...normalizeCommonFields(type, r),
  };
}

function normalizePolicy(raw: unknown, index: number): PolicyCfg {
  const r = rawObj(raw);
  const type = isPolicyType(r.type) ? r.type : "always_sample";
  const name = str(r.name, `ai-policy-${index + 1}`);

  if (type === "and") {
    const subRaw = rawObj(r.and).and_sub_policy;
    const and_sub_policy = (Array.isArray(subRaw) ? subRaw : []).map((s, i) => normalizeSubPolicy(s, i));
    return { name, type, and: { and_sub_policy } };
  }

  return { name, type, ...normalizeCommonFields(type, r) };
}

export function normalizePolicies(raw: unknown): PolicyCfg[] {
  const obj = rawObj(raw);
  const arr = Array.isArray(obj.policies) ? obj.policies : Array.isArray(raw) ? raw : [];
  return arr.map((p, i) => normalizePolicy(p, i));
}

export function extractRationale(raw: unknown): string | undefined {
  const r = rawObj(raw).rationale;
  return typeof r === "string" && r.trim() !== "" ? r : undefined;
}
