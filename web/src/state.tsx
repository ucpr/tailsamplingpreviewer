import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useReducer,
  useRef,
  type ReactNode,
} from "react";
import { BrowserSocket, type WsStatus } from "./ws";
import type {
  ClientCmd,
  CollectorSummary,
  PolicyCfg,
  PolicyView,
  ServerMsg,
  SessionSummary,
  StatisticsView,
  TraceSummary,
} from "./types";

interface AppState {
  wsStatus: WsStatus;
  session: SessionSummary | null;
  collectors: Record<string, CollectorSummary>;
  traces: Record<string, TraceSummary>;
  policy: PolicyView | null;
  statistics: StatisticsView | null;
  notice: string | null;
  // Policies proposed by the AI policy assistant (see ai/), queued for
  // PolicyBuilder to merge into its draft — see queuePolicySuggestions.
  pendingPolicySuggestions: PolicyCfg[] | null;
}

const initialState: AppState = {
  wsStatus: "connecting",
  session: null,
  collectors: {},
  traces: {},
  policy: null,
  statistics: null,
  notice: null,
  pendingPolicySuggestions: null,
};

type Action =
  | { kind: "ws-status"; status: WsStatus }
  | { kind: "server-msg"; msg: ServerMsg }
  | { kind: "policy-set"; policy: PolicyView }
  | { kind: "session-set"; session: SessionSummary }
  | { kind: "notice"; message: string | null }
  | { kind: "policy-suggest"; policies: PolicyCfg[] }
  | { kind: "policy-suggest-clear" };

function reducer(state: AppState, action: Action): AppState {
  switch (action.kind) {
    case "ws-status":
      return { ...state, wsStatus: action.status };
    case "policy-set":
      return { ...state, policy: action.policy };
    case "session-set":
      return { ...state, session: action.session };
    case "notice":
      return { ...state, notice: action.message };
    case "policy-suggest":
      return { ...state, pendingPolicySuggestions: action.policies };
    case "policy-suggest-clear":
      return { ...state, pendingPolicySuggestions: null };
    case "server-msg":
      return applyServerMsg(state, action.msg);
    default:
      return state;
  }
}

function applyServerMsg(state: AppState, msg: ServerMsg): AppState {
  switch (msg.type) {
    case "snapshot": {
      const traces: Record<string, TraceSummary> = {};
      for (const t of msg.traces) traces[t.trace_id] = t;
      const collectors: Record<string, CollectorSummary> = {};
      for (const c of msg.collectors) collectors[c.id] = c;
      return {
        ...state,
        session: msg.session,
        collectors,
        traces,
        policy: msg.policy,
        statistics: msg.statistics,
      };
    }
    case "trace.updated":
      return {
        ...state,
        traces: { ...state.traces, [msg.trace.trace_id]: msg.trace },
      };
    case "trace.decided": {
      const existing = state.traces[msg.trace_id];
      if (!existing) return state;
      return {
        ...state,
        traces: {
          ...state.traces,
          [msg.trace_id]: {
            ...existing,
            decision: msg.decision,
            matched_policies: msg.matched_policies,
            state: "DECIDED",
          },
        },
      };
    }
    case "statistics.updated":
      return { ...state, statistics: msg.statistics };
    case "session.state":
      return { ...state, session: msg.session };
    case "collector.connected":
      return {
        ...state,
        collectors: { ...state.collectors, [msg.collector.id]: msg.collector },
      };
    case "collector.disconnected": {
      const existing = state.collectors[msg.collector_id];
      if (!existing) return state;
      return {
        ...state,
        collectors: {
          ...state.collectors,
          [msg.collector_id]: { ...existing, connected: false },
        },
      };
    }
    case "policy.updated":
      return { ...state, policy: msg.policy };
    case "error":
      return { ...state, notice: msg.message };
    default:
      return state;
  }
}

interface AppContextValue {
  state: AppState;
  sendCmd: (cmd: ClientCmd) => void;
  setPolicy: (policy: PolicyView) => void;
  setSession: (session: SessionSummary) => void;
  notify: (message: string | null) => void;
  queuePolicySuggestions: (policies: PolicyCfg[]) => void;
  clearPolicySuggestions: () => void;
}

const AppContext = createContext<AppContextValue | null>(null);

export function AppStateProvider({ children }: { children: ReactNode }) {
  const [state, dispatch] = useReducer(reducer, initialState);
  const socketRef = useRef<BrowserSocket | null>(null);

  useEffect(() => {
    const socket = new BrowserSocket(
      (msg) => dispatch({ kind: "server-msg", msg }),
      (status) => dispatch({ kind: "ws-status", status }),
    );
    socketRef.current = socket;
    socket.connect();
    return () => socket.close();
  }, []);

  const sendCmd = useCallback((cmd: ClientCmd) => socketRef.current?.send(cmd), []);
  const setPolicy = useCallback(
    (policy: PolicyView) => dispatch({ kind: "policy-set", policy }),
    [],
  );
  const setSession = useCallback(
    (session: SessionSummary) => dispatch({ kind: "session-set", session }),
    [],
  );
  const notify = useCallback(
    (message: string | null) => dispatch({ kind: "notice", message }),
    [],
  );
  const queuePolicySuggestions = useCallback(
    (policies: PolicyCfg[]) => dispatch({ kind: "policy-suggest", policies }),
    [],
  );
  const clearPolicySuggestions = useCallback(() => dispatch({ kind: "policy-suggest-clear" }), []);

  const value = useMemo(
    () => ({ state, sendCmd, setPolicy, setSession, notify, queuePolicySuggestions, clearPolicySuggestions }),
    [state, sendCmd, setPolicy, setSession, notify, queuePolicySuggestions, clearPolicySuggestions],
  );

  return <AppContext.Provider value={value}>{children}</AppContext.Provider>;
}

export function useAppState(): AppContextValue {
  const ctx = useContext(AppContext);
  if (!ctx) throw new Error("useAppState must be used within AppStateProvider");
  return ctx;
}
