import { POLICY_TYPES } from "../policyDefaults";
import type { PolicyGenerationContext } from "./types";

// Short, model-facing description of each policy type and its fields.
// Keys match PolicyType exactly (POLICY_TYPES) so this can't drift out of
// sync silently — see the exhaustiveness check below.
const POLICY_TYPE_DOCS: Record<(typeof POLICY_TYPES)[number], string> = {
  always_sample: "always_sample: matches every trace unconditionally. No fields.",
  latency: "latency: matches traces whose duration exceeds threshold_ms (optionally below upper_threshold_ms).",
  status_code: "status_code: matches traces containing a span with status_codes (subset of OK, ERROR, UNSET).",
  numeric_attribute:
    "numeric_attribute: matches traces with a span attribute `key` whose numeric value is between min_value and max_value (inclusive). invert_match negates the match.",
  string_attribute:
    "string_attribute: matches traces with a span attribute `key` whose string value is one of `values`. enabled_regex_matching treats `values` as regexes. invert_match negates the match.",
  boolean_attribute:
    "boolean_attribute: matches traces with a span attribute `key` whose boolean value equals `value`. invert_match negates the match.",
  probabilistic: "probabilistic: randomly samples sampling_percentage percent of traces.",
  rate_limiting: "rate_limiting: caps kept traces to spans_per_second.",
  span_count: "span_count: matches traces with span count between min_spans and max_spans (inclusive).",
  trace_state: "trace_state: matches traces with a tracestate entry `key` whose value is one of `values`.",
  ottl_condition:
    'ottl_condition: matches if any OTTL boolean expression in `span` evaluates to true. error_mode is one of "ignore"/"propagate"/"silent" (default to "ignore" unless the user asks otherwise). Use real OTTL syntax here, NOT the flat attribute-key convention used by the other attribute-based types — see the OTTL syntax rules below. Prefer this type only when the request needs something the other, simpler types cannot express (e.g. a numeric comparison against a non-numeric_attribute field, or a combination that would otherwise need an `and`).',
  and: "and: matches only if every sub-policy in and_sub_policy matches (AND). Sub-policies cannot themselves be type \"and\".",
};

function buildSystemPrompt(): string {
  const typeDocs = POLICY_TYPES.map((t) => `- ${POLICY_TYPE_DOCS[t]}`).join("\n");
  return [
    "You are an assistant that turns a natural-language description into OpenTelemetry Collector tail-sampling policy configuration for a Tail Sampling Preview tool.",
    "Output must strictly follow the given JSON schema: an object with a `policies` array.",
    "Each policy has a `name` (short kebab-case, e.g. \"checkout-errors\") and a `type`, and only the fields relevant to that `type` should be set. Available policy types:",
    typeDocs,
    "Attribute keys given to you are already prefixed to indicate their source: `resource.<key>` for resource attributes (e.g. resource.service.name), or `attributes.<key>` for span attributes (e.g. attributes.http.status_code). For numeric_attribute/string_attribute/boolean_attribute/trace_state, use the exact key text the user selected verbatim, prefix included, as the `key` field.",
    'For ottl_condition\'s `span` expressions ONLY, translate the same selected keys into real OTTL path syntax instead: `resource.<key>` becomes `resource.attributes["<key>"]`, and `attributes.<key>` becomes `attributes["<key>"]` (drop the "attributes." prefix, it is not part of the OTTL path). Example: selected key `attributes.http.status_code` with value 500 -> `attributes["http.status_code"] == 500`. Do not use the flat `resource.foo`/`attributes.foo` strings directly inside an OTTL expression.',
    "Prefer the simplest policy type that satisfies the request; use `and` only when multiple independent conditions must all hold simultaneously, and use `ottl_condition` only when no other type can express the request.",
    "Do not invent attribute keys that were not given to you or mentioned by the user.",
  ].join("\n");
}

function buildUserPrompt(ctx: PolicyGenerationContext): string {
  const lines = [`Request: ${ctx.description.trim()}`];
  if (ctx.attributes.length > 0) {
    lines.push("Selected attributes for context:");
    for (const attr of ctx.attributes) {
      const label = attr.source === "resource" ? "resource attribute" : "span attribute";
      lines.push(`- ${label} ${attr.key} = ${attr.value}`);
    }
  }
  return lines.join("\n");
}

export function buildPrompts(ctx: PolicyGenerationContext): { system: string; user: string } {
  return { system: buildSystemPrompt(), user: buildUserPrompt(ctx) };
}
