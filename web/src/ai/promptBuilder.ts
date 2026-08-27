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
  and: "and: matches only if every sub-policy in and_sub_policy matches (AND). Sub-policies cannot themselves be type \"and\".",
};

function buildSystemPrompt(): string {
  const typeDocs = POLICY_TYPES.map((t) => `- ${POLICY_TYPE_DOCS[t]}`).join("\n");
  return [
    "You are an assistant that turns a natural-language description into OpenTelemetry Collector tail-sampling policy configuration for a Tail Sampling Preview tool.",
    "Output must strictly follow the given JSON schema: an object with a `policies` array.",
    "Each policy has a `name` (short kebab-case, e.g. \"checkout-errors\") and a `type`, and only the fields relevant to that `type` should be set. Available policy types:",
    typeDocs,
    "Attribute keys given to you are already prefixed to indicate their source: `resource.<key>` for resource attributes (e.g. resource.service.name), or a bare key for span attributes (e.g. http.status_code). Use the exact key text the user selected, prefix included, as the `key` field for attribute-based policy types.",
    "Prefer the simplest policy type that satisfies the request; use `and` only when multiple independent conditions must all hold simultaneously.",
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
