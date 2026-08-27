import { useAppState } from "../state";
import { formatClock, formatCount } from "../format";
import type { WsStatus } from "../ws";

function wsLabel(status: WsStatus): string {
  switch (status) {
    case "open":
      return "Connected";
    case "connecting":
      return "Connecting…";
    case "closed":
      return "Disconnected";
  }
}

export function Header() {
  const { state } = useAppState();
  const collectors = Object.values(state.collectors).sort((a, b) =>
    a.id.localeCompare(b.id),
  );

  return (
    <header className="app-header">
      <div className="app-header__brand">
        <span className="app-header__title">Tail Sampling Preview</span>
        <span className={`ws-dot ws-dot--${state.wsStatus}`} title={wsLabel(state.wsStatus)} />
        <span className="app-header__ws-label">{wsLabel(state.wsStatus)}</span>
      </div>

      <div className="app-header__collectors">
        <span className="app-header__collectors-label">Collectors</span>
        {collectors.length === 0 ? (
          <span className="app-header__empty">none connected</span>
        ) : (
          collectors.map((c) => (
            <span key={c.id} className="collector-chip" title={`last seen ${formatClock(c.last_seen)}`}>
              <span className={`status-dot ${c.connected ? "status-dot--up" : "status-dot--down"}`} />
              {c.id}
              <span className="collector-chip__meta">v{c.version}</span>
              <span className="collector-chip__meta">{formatCount(c.spans_received)} spans</span>
            </span>
          ))
        )}
      </div>
    </header>
  );
}
