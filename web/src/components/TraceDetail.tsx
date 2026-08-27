import { useEffect, useMemo, useState } from "react";
import { getTrace } from "../api";
import { useAppState } from "../state";
import { formatDurationMs } from "../format";
import type { SpanSummary, TraceDetail as TraceDetailType } from "../types";

interface Props {
  traceId: string | null;
  onClose: () => void;
}

interface WaterfallRow {
  span: SpanSummary;
  leftPct: number;
  widthPct: number;
}

function buildWaterfall(spans: SpanSummary[]): WaterfallRow[] {
  if (spans.length === 0) return [];
  const starts = spans.map((s) => new Date(s.start_time).getTime());
  const ends = spans.map((s) => new Date(s.end_time).getTime());
  const overallStart = Math.min(...starts);
  const overallEnd = Math.max(...ends);
  const total = Math.max(overallEnd - overallStart, 1);

  return spans
    .map((span, i) => ({
      span,
      leftPct: ((starts[i] - overallStart) / total) * 100,
      widthPct: Math.max(((ends[i] - starts[i]) / total) * 100, 0.5),
    }))
    .sort((a, b) => a.leftPct - b.leftPct);
}

export function TraceDetail({ traceId, onClose }: Props) {
  const { state } = useAppState();
  const summary = traceId ? state.traces[traceId] : undefined;
  const [detail, setDetail] = useState<TraceDetailType | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [selectedSpanId, setSelectedSpanId] = useState<string | null>(null);

  useEffect(() => {
    setSelectedSpanId(null);
  }, [traceId]);

  useEffect(() => {
    if (!traceId) {
      setDetail(null);
      return;
    }
    let cancelled = false;
    setLoading(true);
    setError(null);
    getTrace(traceId)
      .then((d) => {
        if (cancelled) return;
        if (!d) setError("Trace no longer retained (evicted from ring buffer).");
        setDetail(d);
      })
      .catch((err: unknown) => {
        if (!cancelled) setError(err instanceof Error ? err.message : "Failed to load trace");
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [traceId, summary?.span_count, summary?.state, summary?.decision]);

  const waterfall = useMemo(() => (detail ? buildWaterfall(detail.spans) : []), [detail]);

  // Default-select the earliest-starting span (the root span, or the
  // earliest arrival if the true root hasn't arrived yet) once a trace's
  // spans load, so attributes show something immediately.
  useEffect(() => {
    if (selectedSpanId === null && waterfall.length > 0) {
      setSelectedSpanId(waterfall[0].span.span_id);
    }
  }, [waterfall, selectedSpanId]);

  if (!traceId) return null;

  const matched = new Set(detail?.matched_policies ?? []);
  const allPolicyNames = detail?.matched_policies ?? [];
  const selectedSpan = detail?.spans.find((s) => s.span_id === selectedSpanId) ?? null;
  const attributeEntries = selectedSpan
    ? Object.entries(selectedSpan.attributes ?? {}).sort(([a], [b]) => a.localeCompare(b))
    : [];

  return (
    <aside className="trace-detail">
      <div className="trace-detail__header">
        <h2 className="panel__title">Trace Detail</h2>
        <button type="button" className="trace-detail__close" onClick={onClose}>
          ✕
        </button>
      </div>

      {loading && <p className="app-header__empty">Loading…</p>}
      {error && <p className="notice notice--error">{error}</p>}

      {detail && (
        <>
          <div className="trace-detail__summary">
            <div className="trace-detail__name">{detail.service_name || "unknown"}</div>
            <div className="trace-detail__root-span">{detail.root_span_name}</div>
            <div className="trace-detail__meta">
              <span className="mono">{formatDurationMs(detail.duration_ms)}</span>
              <span className={detail.has_error ? "badge badge--error" : "badge badge--ok"}>
                {detail.has_error ? "ERROR" : "OK"}
              </span>
              <span>{detail.span_count} spans</span>
            </div>
            <div className="mono trace-detail__trace-id">{detail.trace_id}</div>
          </div>

          <h3 className="panel__subtitle">Decision</h3>
          <div>
            {detail.state === "DECIDED" ? (
              <span className={detail.decision === "KEEP" ? "badge badge--keep" : "badge badge--drop"}>
                {detail.decision}
              </span>
            ) : (
              <span className="badge badge--live">
                LIVE — decision in {detail.state === "RECEIVING" ? "progress" : "final wait"}
              </span>
            )}
          </div>

          <h3 className="panel__subtitle">Matched Policies</h3>
          {allPolicyNames.length === 0 ? (
            <p className="app-header__empty">No policy matched.</p>
          ) : (
            <ul className="matched-policy-list">
              {allPolicyNames.map((name) => (
                <li key={name} className={matched.has(name) ? "is-matched" : ""}>
                  {matched.has(name) ? "✓" : "×"} {name}
                </li>
              ))}
            </ul>
          )}

          <h3 className="panel__subtitle">Waterfall</h3>
          <div className="waterfall">
            {waterfall.map(({ span, leftPct, widthPct }) => (
              <div
                className={`waterfall__row ${span.span_id === selectedSpanId ? "is-selected" : ""}`}
                key={span.span_id}
                onClick={() => setSelectedSpanId(span.span_id)}
              >
                <div className="waterfall__label" title={`${span.service_name} · ${span.name}`}>
                  {span.name}
                </div>
                <div className="waterfall__track">
                  <div
                    className={`waterfall__bar ${span.status_code === "ERROR" ? "waterfall__bar--error" : ""}`}
                    style={{ left: `${leftPct}%`, width: `${widthPct}%` }}
                    title={formatDurationMs(span.duration_ms)}
                  />
                </div>
                <div className="waterfall__duration mono">{formatDurationMs(span.duration_ms)}</div>
              </div>
            ))}
          </div>

          <h3 className="panel__subtitle">
            Attributes{selectedSpan ? <span className="mono"> — {selectedSpan.name}</span> : null}
          </h3>
          {!selectedSpan ? (
            <p className="app-header__empty">Select a span to view its attributes.</p>
          ) : attributeEntries.length === 0 ? (
            <p className="app-header__empty">No attributes on this span.</p>
          ) : (
            <div className="attribute-list-wrap">
              <dl className="attribute-list">
                {attributeEntries.map(([key, value]) => (
                  <div className="attribute-list__row" key={key}>
                    <dt className="mono">{key}</dt>
                    <dd className="mono">{String(value)}</dd>
                  </div>
                ))}
              </dl>
            </div>
          )}
        </>
      )}
    </aside>
  );
}
