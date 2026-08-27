// TypeScript port of the Live Tail filter grammar (spec.md ss27,
// internal/query/filter.go). Whitespace-separated terms are ANDed. A term
// is either free text (substring match on service_name / root_span_name)
// or key:value / key:op value (op in > >= < <=, meaningful for duration).
//
// This mirrors the backend's behavior for the keys the browser can
// evaluate against a TraceSummary (status, duration, service.name/service,
// free text). TraceSummary carries only the root span's name, not every
// span name, and no attributes, so free-text and arbitrary attribute keys
// are necessarily an approximation of the server-side match — a soft
// no-op for keys we can't evaluate client-side, matching the grammar's
// "never fail, never crash" contract.
import type { TraceSummary } from "./types";

interface Term {
  key: string;
  op: "" | ":" | ">" | ">=" | "<" | "<=";
  text: string;
}

export type Filter = Term[];

const DURATION_OPS: Array<Term["op"]> = [">=", "<=", ">", "<"];

export function parseFilter(query: string): Filter {
  return query
    .split(/\s+/)
    .filter((s) => s.length > 0)
    .map(parseTerm);
}

function parseTerm(raw: string): Term {
  const idx = raw.indexOf(":");
  if (idx < 0) return { key: "", op: "", text: raw.toLowerCase() };

  const key = raw.slice(0, idx).toLowerCase();
  let val = raw.slice(idx + 1);
  let op: Term["op"] = ":";
  for (const candidate of DURATION_OPS) {
    if (val.startsWith(candidate)) {
      op = candidate;
      val = val.slice(candidate.length);
      break;
    }
  }
  return { key, op, text: val };
}

export function matchesFilter(t: TraceSummary, filter: Filter): boolean {
  return filter.every((term) => matchTerm(t, term));
}

function matchTerm(t: TraceSummary, term: Term): boolean {
  if (term.key === "") return matchFreeText(t, term.text);

  switch (term.key) {
    case "status":
      return t.has_error === (term.text.toLowerCase() === "error");
    case "duration":
      return matchDuration(t, term.op, term.text);
    case "service.name":
    case "service":
      return t.service_name.toLowerCase().includes(term.text.toLowerCase());
    default:
      // Unrecognized / attribute keys: TraceSummary carries no span
      // attributes, so treat as a no-op (spec.md ss27).
      return true;
  }
}

function matchFreeText(t: TraceSummary, text: string): boolean {
  return (
    t.service_name.toLowerCase().includes(text) ||
    t.root_span_name.toLowerCase().includes(text)
  );
}

// parseGoDuration parses a Go-style duration string ("1s", "500ms", "1.5m")
// into milliseconds, matching time.ParseDuration's units well enough for
// the durations traces realistically span.
function parseGoDuration(text: string): number | null {
  const match = /^([0-9]*\.?[0-9]+)(ns|us|µs|ms|s|m|h)$/.exec(text.trim());
  if (!match) return null;
  const value = parseFloat(match[1]);
  const unit = match[2];
  const perMs: Record<string, number> = {
    ns: 1e-6,
    us: 1e-3,
    "µs": 1e-3,
    ms: 1,
    s: 1000,
    m: 60_000,
    h: 3_600_000,
  };
  return value * perMs[unit];
}

function matchDuration(t: TraceSummary, op: Term["op"], text: string): boolean {
  const want = parseGoDuration(text);
  if (want === null) return false;
  const got = t.duration_ms;
  switch (op) {
    case ">":
      return got > want;
    case ">=":
      return got >= want;
    case "<":
      return got < want;
    case "<=":
      return got <= want;
    default:
      return got === want;
  }
}
