// REST client for the Preview Server API. All requests are same-origin
// (the backend has no CORS support); the Vite dev server proxies /api to
// the real backend (see vite.config.ts).
import type {
  CollectorSummary,
  CompareResult,
  PolicyView,
  SessionSummary,
  StatisticsView,
  TraceDetail,
  TraceSummary,
} from "./types";

export class ApiError extends Error {}

async function json<T>(res: Response): Promise<T> {
  if (!res.ok) {
    const text = await res.text();
    throw new ApiError(text || `${res.status} ${res.statusText}`);
  }
  return (await res.json()) as T;
}

export async function getSession(): Promise<SessionSummary> {
  return json<SessionSummary>(await fetch("/api/session"));
}

function sessionAction(path: string): () => Promise<SessionSummary> {
  return async () => json<SessionSummary>(await fetch(path, { method: "POST" }));
}

export const startSession = sessionAction("/api/session/start");
export const pauseSession = sessionAction("/api/session/pause");
export const resumeSession = sessionAction("/api/session/resume");
export const stopSession = sessionAction("/api/session/stop");

export async function getStatistics(): Promise<StatisticsView> {
  return json<StatisticsView>(await fetch("/api/statistics"));
}

export async function getCollectors(): Promise<CollectorSummary[]> {
  return json<CollectorSummary[]>(await fetch("/api/collectors"));
}

export async function getPolicy(): Promise<PolicyView> {
  return json<PolicyView>(await fetch("/api/policy"));
}

export async function putPolicy(policy: PolicyView): Promise<PolicyView> {
  const res = await fetch("/api/policy", {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(policy),
  });
  return json<PolicyView>(res);
}

export async function comparePolicy(candidate: PolicyView): Promise<CompareResult> {
  const res = await fetch("/api/policy/compare", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(candidate),
  });
  return json<CompareResult>(res);
}

export async function getPolicyYAML(): Promise<string> {
  const res = await fetch("/api/policy/yaml");
  if (!res.ok) throw new ApiError(await res.text());
  return res.text();
}

export async function putPolicyYAML(yamlText: string): Promise<PolicyView> {
  const res = await fetch("/api/policy/yaml", {
    method: "PUT",
    headers: { "Content-Type": "text/yaml" },
    body: yamlText,
  });
  return json<PolicyView>(res);
}

export async function getTraces(q?: string): Promise<TraceSummary[]> {
  const qs = q ? `?q=${encodeURIComponent(q)}` : "";
  const res = await fetch(`/api/traces${qs}`);
  return json<TraceSummary[]>(res);
}

export async function getTrace(id: string): Promise<TraceDetail | null> {
  const res = await fetch(`/api/traces/${id}`);
  if (res.status === 404) return null;
  return json<TraceDetail>(res);
}
