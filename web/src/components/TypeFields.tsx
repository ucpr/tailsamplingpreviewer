import type { CommonFields } from "../policyDefaults";
import { splitList } from "../policyDefaults";
import type { PolicyType, StatusCode as StatusCodeValue } from "../types";

interface Props {
  type: PolicyType;
  cfg: CommonFields;
  onChange: (patch: CommonFields) => void;
}

const STATUS_CODES: StatusCodeValue[] = ["OK", "ERROR", "UNSET"];

function numberOrUndefined(raw: string): number | undefined {
  if (raw.trim() === "") return undefined;
  const n = Number(raw);
  return Number.isNaN(n) ? undefined : n;
}

// TypeFields renders the type-specific sub-form for one policy (or "and"
// sub-policy). "always_sample" and "and" have no fields of their own here:
// "and" is handled by the caller (a nested sub-policy list).
export function TypeFields({ type, cfg, onChange }: Props) {
  switch (type) {
    case "always_sample":
      return <p className="field-hint">Matches every trace unconditionally.</p>;

    case "latency": {
      const c = cfg.latency ?? { threshold_ms: 1000 };
      return (
        <div className="field-row">
          <label>
            Threshold (ms)
            <input
              type="number"
              min={0}
              value={c.threshold_ms}
              onChange={(e) =>
                onChange({ latency: { ...c, threshold_ms: Number(e.target.value) || 0 } })
              }
            />
          </label>
          <label>
            Upper threshold (ms, optional)
            <input
              type="number"
              min={0}
              value={c.upper_threshold_ms ?? ""}
              onChange={(e) =>
                onChange({ latency: { ...c, upper_threshold_ms: numberOrUndefined(e.target.value) } })
              }
            />
          </label>
        </div>
      );
    }

    case "status_code": {
      const selected = new Set(cfg.status_code?.status_codes ?? []);
      return (
        <div className="field-row">
          {STATUS_CODES.map((code) => (
            <label key={code} className="checkbox-label">
              <input
                type="checkbox"
                checked={selected.has(code)}
                onChange={(e) => {
                  const next = new Set(selected);
                  if (e.target.checked) next.add(code);
                  else next.delete(code);
                  onChange({ status_code: { status_codes: Array.from(next) } });
                }}
              />
              {code}
            </label>
          ))}
        </div>
      );
    }

    case "numeric_attribute": {
      const c = cfg.numeric_attribute ?? { key: "", min_value: 0, max_value: 0 };
      return (
        <div className="field-row">
          <label>
            Attribute key
            <input
              type="text"
              value={c.key}
              onChange={(e) => onChange({ numeric_attribute: { ...c, key: e.target.value } })}
            />
          </label>
          <label>
            Min value
            <input
              type="number"
              value={c.min_value}
              onChange={(e) =>
                onChange({ numeric_attribute: { ...c, min_value: Number(e.target.value) || 0 } })
              }
            />
          </label>
          <label>
            Max value
            <input
              type="number"
              value={c.max_value}
              onChange={(e) =>
                onChange({ numeric_attribute: { ...c, max_value: Number(e.target.value) || 0 } })
              }
            />
          </label>
          <label className="checkbox-label">
            <input
              type="checkbox"
              checked={c.invert_match ?? false}
              onChange={(e) => onChange({ numeric_attribute: { ...c, invert_match: e.target.checked } })}
            />
            Invert match
          </label>
        </div>
      );
    }

    case "string_attribute": {
      const c = cfg.string_attribute ?? { key: "", values: [] };
      return (
        <div className="field-row">
          <label>
            Attribute key
            <input
              type="text"
              value={c.key}
              onChange={(e) => onChange({ string_attribute: { ...c, key: e.target.value } })}
            />
          </label>
          <label>
            Values (comma-separated)
            <input
              type="text"
              value={c.values.join(", ")}
              onChange={(e) =>
                onChange({ string_attribute: { ...c, values: splitList(e.target.value) } })
              }
            />
          </label>
          <label className="checkbox-label">
            <input
              type="checkbox"
              checked={c.enabled_regex_matching ?? false}
              onChange={(e) =>
                onChange({ string_attribute: { ...c, enabled_regex_matching: e.target.checked } })
              }
            />
            Regex match
          </label>
          <label className="checkbox-label">
            <input
              type="checkbox"
              checked={c.invert_match ?? false}
              onChange={(e) => onChange({ string_attribute: { ...c, invert_match: e.target.checked } })}
            />
            Invert match
          </label>
        </div>
      );
    }

    case "boolean_attribute": {
      const c = cfg.boolean_attribute ?? { key: "", value: true };
      return (
        <div className="field-row">
          <label>
            Attribute key
            <input
              type="text"
              value={c.key}
              onChange={(e) => onChange({ boolean_attribute: { ...c, key: e.target.value } })}
            />
          </label>
          <label>
            Value
            <select
              value={String(c.value)}
              onChange={(e) => onChange({ boolean_attribute: { ...c, value: e.target.value === "true" } })}
            >
              <option value="true">true</option>
              <option value="false">false</option>
            </select>
          </label>
          <label className="checkbox-label">
            <input
              type="checkbox"
              checked={c.invert_match ?? false}
              onChange={(e) => onChange({ boolean_attribute: { ...c, invert_match: e.target.checked } })}
            />
            Invert match
          </label>
        </div>
      );
    }

    case "probabilistic": {
      const c = cfg.probabilistic ?? { sampling_percentage: 10 };
      return (
        <div className="field-row">
          <label>
            Sampling percentage
            <input
              type="number"
              min={0}
              max={100}
              step={0.1}
              value={c.sampling_percentage}
              onChange={(e) =>
                onChange({ probabilistic: { ...c, sampling_percentage: Number(e.target.value) || 0 } })
              }
            />
          </label>
          <label>
            Hash salt (optional)
            <input
              type="text"
              value={c.hash_salt ?? ""}
              onChange={(e) => onChange({ probabilistic: { ...c, hash_salt: e.target.value } })}
            />
          </label>
        </div>
      );
    }

    case "rate_limiting": {
      const c = cfg.rate_limiting ?? { spans_per_second: 100 };
      return (
        <div className="field-row">
          <label>
            Spans per second
            <input
              type="number"
              min={0}
              value={c.spans_per_second}
              onChange={(e) =>
                onChange({ rate_limiting: { spans_per_second: Number(e.target.value) || 0 } })
              }
            />
          </label>
        </div>
      );
    }

    case "span_count": {
      const c = cfg.span_count ?? { min_spans: 1 };
      return (
        <div className="field-row">
          <label>
            Min spans
            <input
              type="number"
              min={0}
              value={c.min_spans}
              onChange={(e) => onChange({ span_count: { ...c, min_spans: Number(e.target.value) || 0 } })}
            />
          </label>
          <label>
            Max spans (optional)
            <input
              type="number"
              min={0}
              value={c.max_spans ?? ""}
              onChange={(e) =>
                onChange({ span_count: { ...c, max_spans: numberOrUndefined(e.target.value) } })
              }
            />
          </label>
        </div>
      );
    }

    case "trace_state": {
      const c = cfg.trace_state ?? { key: "", values: [] };
      return (
        <div className="field-row">
          <label>
            tracestate key
            <input
              type="text"
              value={c.key}
              onChange={(e) => onChange({ trace_state: { ...c, key: e.target.value } })}
            />
          </label>
          <label>
            Values (comma-separated)
            <input
              type="text"
              value={c.values.join(", ")}
              onChange={(e) => onChange({ trace_state: { ...c, values: splitList(e.target.value) } })}
            />
          </label>
        </div>
      );
    }

    case "and":
      return null;
  }
}
