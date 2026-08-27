import { useState } from "react";
import { useAppState } from "../state";
import { pauseSession, resumeSession, startSession, stopSession, ApiError } from "../api";
import { formatCount, formatElapsed } from "../format";
import { useNow } from "../useNow";

export function SessionPanel() {
  const { state, setSession, notify } = useAppState();
  const [busy, setBusy] = useState(false);
  const now = useNow();

  const session = state.session;
  const sessionState = session?.state ?? "IDLE";
  // started_at can carry a stale value from a session that already ended
  // (the server doesn't clear it on stop) — only trust it while a session
  // is actually running.
  const isActive = sessionState === "STREAMING" || sessionState === "PAUSED";

  const run = async (action: () => Promise<typeof session>) => {
    setBusy(true);
    try {
      const next = await action();
      if (next) setSession(next);
    } catch (err) {
      notify(err instanceof ApiError ? err.message : "Request failed");
    } finally {
      setBusy(false);
    }
  };

  return (
    <section className="panel">
      <h2 className="panel__title">Preview Session</h2>
      <div className="session-state">
        <span className={`status-dot status-dot--${sessionState.toLowerCase()}`} />
        <span className="session-state__label">{sessionState}</span>
      </div>

      <dl className="stat-grid">
        <div>
          <dt>Duration</dt>
          <dd>{isActive ? formatElapsed(session?.started_at, now) : "00:00"}</dd>
        </div>
        <div>
          <dt>Spans</dt>
          <dd>{formatCount(session?.spans ?? 0)}</dd>
        </div>
        <div>
          <dt>Traces</dt>
          <dd>{formatCount(session?.traces ?? 0)}</dd>
        </div>
      </dl>

      <div className="button-row">
        <button
          type="button"
          disabled={busy || sessionState !== "IDLE"}
          onClick={() => run(startSession)}
        >
          Start
        </button>
        <button
          type="button"
          disabled={busy || sessionState !== "STREAMING"}
          onClick={() => run(pauseSession)}
        >
          Pause
        </button>
        <button
          type="button"
          disabled={busy || sessionState !== "PAUSED"}
          onClick={() => run(resumeSession)}
        >
          Resume
        </button>
        <button
          type="button"
          disabled={busy || sessionState === "IDLE"}
          onClick={() => run(stopSession)}
        >
          Stop
        </button>
      </div>
    </section>
  );
}
