import { useMemo, useState } from "react";
import { useAppState } from "../state";
import { matchesFilter, parseFilter } from "../filter";
import { formatClock, formatDurationMs } from "../format";
import type { TraceSummary } from "../types";

interface Props {
  selectedTraceId: string | null;
  onSelect: (traceId: string) => void;
}

function decisionBadge(t: TraceSummary): { label: string; className: string; title: string } {
  if (t.state === "DECIDED") {
    return t.decision === "KEEP"
      ? { label: "KEEP", className: "badge badge--keep", title: "Final decision" }
      : { label: "DROP", className: "badge badge--drop", title: "Final decision" };
  }
  if (t.decision === "KEEP") {
    return { label: "LIVE", className: "badge badge--live-keep", title: "Provisional: would KEEP" };
  }
  if (t.decision === "DROP") {
    return { label: "LIVE", className: "badge badge--live-drop", title: "Provisional: would DROP" };
  }
  return { label: "LIVE", className: "badge badge--live", title: "Decision pending" };
}

export function LiveTail({ selectedTraceId, onSelect }: Props) {
  const { state } = useAppState();
  const [query, setQuery] = useState("");

  const filter = useMemo(() => parseFilter(query), [query]);

  const rows = useMemo(() => {
    const all = Object.values(state.traces);
    const filtered = filter.length === 0 ? all : all.filter((t) => matchesFilter(t, filter));
    return filtered.sort((a, b) => (a.last_seen < b.last_seen ? 1 : -1));
  }, [state.traces, filter]);

  return (
    <section className="panel panel--live-tail">
      <div className="panel__header-row">
        <h2 className="panel__title">Live Tail</h2>
        <span className="live-tail__count">{rows.length} traces</span>
      </div>

      <input
        className="live-tail__query"
        type="text"
        placeholder="service.name:payment status:error duration:>1s"
        value={query}
        onChange={(e) => setQuery(e.target.value)}
        spellCheck={false}
      />

      <div className="live-tail__table-wrap">
        <table className="live-tail__table">
          <thead>
            <tr>
              <th>Time</th>
              <th>Decision</th>
              <th>Service</th>
              <th>Duration</th>
              <th>Status</th>
              <th>Spans</th>
            </tr>
          </thead>
          <tbody>
            {rows.map((t) => {
              const badge = decisionBadge(t);
              return (
                <tr
                  key={t.trace_id}
                  className={t.trace_id === selectedTraceId ? "is-selected" : undefined}
                  onClick={() => onSelect(t.trace_id)}
                >
                  <td className="mono">{formatClock(t.last_seen)}</td>
                  <td>
                    <span className={badge.className} title={badge.title}>
                      {badge.label}
                    </span>
                  </td>
                  <td>{t.service_name || "unknown"}</td>
                  <td className="mono">{formatDurationMs(t.duration_ms)}</td>
                  <td>
                    <span className={t.has_error ? "badge badge--error" : "badge badge--ok"}>
                      {t.has_error ? "ERROR" : "OK"}
                    </span>
                  </td>
                  <td className="mono">{t.span_count}</td>
                </tr>
              );
            })}
            {rows.length === 0 && (
              <tr>
                <td colSpan={6} className="live-tail__empty">
                  No traces yet. Start a Preview Session to begin receiving Shadow Traffic.
                </td>
              </tr>
            )}
          </tbody>
        </table>
      </div>
    </section>
  );
}
