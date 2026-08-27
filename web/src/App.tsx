import { useState } from "react";
import { AppStateProvider, useAppState } from "./state";
import { Header } from "./components/Header";
import { SessionPanel } from "./components/SessionPanel";
import { StatisticsPanel } from "./components/StatisticsPanel";
import { LiveTail } from "./components/LiveTail";
import { SpanVolumeChart } from "./components/SpanVolumeChart";
import { TraceDetail } from "./components/TraceDetail";
import { PolicyBuilder } from "./components/PolicyBuilder";
import { YamlPanel } from "./components/YamlPanel";
import "./App.css";

function NoticeBanner() {
  const { state, notify } = useAppState();
  if (!state.notice) return null;
  return (
    <div className="notice-banner">
      <span>{state.notice}</span>
      <button type="button" onClick={() => notify(null)}>
        ✕
      </button>
    </div>
  );
}

function AppShell() {
  const [selectedTraceId, setSelectedTraceId] = useState<string | null>(null);

  return (
    <div className="app-shell">
      <Header />
      <NoticeBanner />
      <main className="app-main">
        <div className="app-main__left">
          <SessionPanel />
          <StatisticsPanel />
        </div>

        <div className="app-main__center">
          <section className="panel panel--span-volume">
            <SpanVolumeChart />
          </section>
          <LiveTail selectedTraceId={selectedTraceId} onSelect={setSelectedTraceId} />
          {selectedTraceId && (
            <TraceDetail traceId={selectedTraceId} onClose={() => setSelectedTraceId(null)} />
          )}
        </div>

        <div className="app-main__right">
          <PolicyBuilder />
          <YamlPanel />
        </div>
      </main>
    </div>
  );
}

export default function App() {
  return (
    <AppStateProvider>
      <AppShell />
    </AppStateProvider>
  );
}
