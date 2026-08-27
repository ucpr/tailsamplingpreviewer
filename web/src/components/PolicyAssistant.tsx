import { useEffect, useState } from "react";
import { getDefaultProvider } from "../ai/registry";
import type { PolicyGenerationResult, ProviderAvailability, SelectedAttribute } from "../ai/types";
import { useAppState } from "../state";
import type { PolicyCfg } from "../types";

interface Props {
  selectedAttributes: SelectedAttribute[];
  onRemoveAttribute: (key: string) => void;
}

function summarizePolicy(policy: PolicyCfg): string {
  switch (policy.type) {
    case "always_sample":
      return "matches every trace";
    case "latency":
      return `duration > ${policy.latency?.threshold_ms}ms`;
    case "status_code":
      return `status in [${policy.status_code?.status_codes.join(", ")}]`;
    case "numeric_attribute":
      return `${policy.numeric_attribute?.key} in [${policy.numeric_attribute?.min_value}, ${policy.numeric_attribute?.max_value}]`;
    case "string_attribute":
      return `${policy.string_attribute?.key} in [${policy.string_attribute?.values.join(", ")}]`;
    case "boolean_attribute":
      return `${policy.boolean_attribute?.key} = ${policy.boolean_attribute?.value}`;
    case "probabilistic":
      return `${policy.probabilistic?.sampling_percentage}% sampled`;
    case "rate_limiting":
      return `≤ ${policy.rate_limiting?.spans_per_second} spans/sec`;
    case "span_count":
      return `span count ≥ ${policy.span_count?.min_spans}`;
    case "trace_state":
      return `tracestate ${policy.trace_state?.key} in [${policy.trace_state?.values.join(", ")}]`;
    case "and":
      return `ALL of ${policy.and?.and_sub_policy.length ?? 0} conditions`;
    default:
      return "";
  }
}

export function PolicyAssistant({ selectedAttributes, onRemoveAttribute }: Props) {
  const { queuePolicySuggestions } = useAppState();
  const [availability, setAvailability] = useState<ProviderAvailability | "checking">("checking");
  const [description, setDescription] = useState("");
  const [generating, setGenerating] = useState(false);
  const [downloadPct, setDownloadPct] = useState<number | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [result, setResult] = useState<PolicyGenerationResult | null>(null);

  const provider = getDefaultProvider();

  useEffect(() => {
    let cancelled = false;
    provider.checkAvailability().then((a) => {
      if (!cancelled) setAvailability(a);
    });
    return () => {
      cancelled = true;
    };
  }, [provider]);

  const handleGenerate = async () => {
    if (!description.trim()) return;
    setGenerating(true);
    setError(null);
    setResult(null);
    setDownloadPct(null);
    try {
      const res = await provider.generate(
        { description, attributes: selectedAttributes },
        { onDownloadProgress: setDownloadPct },
      );
      setResult(res);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to generate policy suggestion");
    } finally {
      setGenerating(false);
      setDownloadPct(null);
    }
  };

  const handleAdd = () => {
    if (!result || result.policies.length === 0) return;
    queuePolicySuggestions(result.policies);
    setResult(null);
    setDescription("");
  };

  return (
    <section className="policy-assistant">
      <h3 className="panel__subtitle">AI Policy Assistant</h3>

      {availability === "unavailable" && (
        <p className="notice notice--error">
          Chrome's built-in AI (Prompt API) isn't available in this browser. It requires a recent desktop Chrome with
          the Prompt API enabled (see chrome://flags and chrome://components).
        </p>
      )}
      {availability === "downloadable" && (
        <p className="field-hint">
          The on-device model may need to download on first use — this can take a while.
          {downloadPct !== null && ` Downloading… ${downloadPct}%`}
        </p>
      )}

      {selectedAttributes.length > 0 && (
        <div className="policy-assistant__chips">
          {selectedAttributes.map((attr) => (
            <button
              type="button"
              key={attr.key}
              className="policy-assistant__chip"
              onClick={() => onRemoveAttribute(attr.key)}
              title="Remove from context"
            >
              {attr.key} = {attr.value} ✕
            </button>
          ))}
        </div>
      )}

      <textarea
        className="policy-assistant__input"
        placeholder='Describe the policy you want, e.g. "keep all traces from the checkout service that have an error"'
        value={description}
        onChange={(e) => setDescription(e.target.value)}
        disabled={availability === "unavailable"}
        rows={3}
      />

      <button
        type="button"
        onClick={handleGenerate}
        disabled={generating || availability === "unavailable" || !description.trim()}
      >
        {generating ? "Generating…" : "Generate"}
      </button>

      {error && <p className="notice notice--error">{error}</p>}

      {result && (
        <div className="policy-assistant__result">
          {result.rationale && <p className="field-hint">{result.rationale}</p>}
          {result.policies.length === 0 ? (
            <p className="app-header__empty">No policy suggestion produced. Try rephrasing.</p>
          ) : (
            <>
              <ul className="policy-assistant__suggestions">
                {result.policies.map((p, i) => (
                  <li key={i}>
                    <strong>{p.name}</strong> <span className="mono">({p.type})</span> — {summarizePolicy(p)}
                  </li>
                ))}
              </ul>
              <button type="button" onClick={handleAdd}>
                Add {result.policies.length > 1 ? `${result.policies.length} policies` : "policy"} to Policy Builder
              </button>
            </>
          )}
        </div>
      )}
    </section>
  );
}
