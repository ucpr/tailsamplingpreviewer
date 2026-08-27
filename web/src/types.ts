// Wire types for the Preview Server REST/WS API. Field names mirror the Go
// JSON tags exactly (see internal/server/events.go, internal/sampling/otel.go).

export type TraceState = "RECEIVING" | "READY" | "DECIDED" | "EXPIRED";
export type Decision = "PENDING" | "KEEP" | "DROP";
export type StatusCode = "OK" | "ERROR" | "UNSET";
export type SessionState = "IDLE" | "STREAMING" | "PAUSED";

export interface TraceSummary {
  trace_id: string;
  service_name: string;
  root_span_name: string;
  duration_ms: number;
  has_error: boolean;
  span_count: number;
  state: TraceState;
  decision: Decision;
  matched_policies: string[] | null;
  first_seen: string;
  last_seen: string;
}

export interface SpanSummary {
  span_id: string;
  parent_span_id: string;
  name: string;
  service_name: string;
  start_time: string;
  end_time: string;
  duration_ms: number;
  status_code: StatusCode;
}

export interface TraceDetail extends TraceSummary {
  spans: SpanSummary[];
}

export interface CollectorSummary {
  id: string;
  version: string;
  connected: boolean;
  spans_received: number;
  last_seen: string;
}

export interface SessionSummary {
  state: SessionState;
  session_id: string;
  started_at?: string;
  spans: number;
  traces: number;
}

export interface StatisticsView {
  observed: number;
  keep: number;
  drop: number;
  sampling_rate: number;
  policy_matches: Record<string, number>;
}

export type PolicyType =
  | "always_sample"
  | "latency"
  | "numeric_attribute"
  | "probabilistic"
  | "status_code"
  | "string_attribute"
  | "rate_limiting"
  | "boolean_attribute"
  | "span_count"
  | "trace_state"
  | "and";

export interface LatencyCfg {
  threshold_ms: number;
  upper_threshold_ms?: number;
}

export interface NumericAttributeCfg {
  key: string;
  min_value: number;
  max_value: number;
  invert_match?: boolean;
}

export interface ProbabilisticCfg {
  hash_salt?: string;
  sampling_percentage: number;
}

export interface StatusCodeCfg {
  status_codes: StatusCode[];
}

export interface StringAttributeCfg {
  key: string;
  values: string[];
  enabled_regex_matching?: boolean;
  invert_match?: boolean;
}

export interface RateLimitingCfg {
  spans_per_second: number;
}

export interface BooleanAttributeCfg {
  key: string;
  value: boolean;
  invert_match?: boolean;
}

export interface SpanCountCfg {
  min_spans: number;
  max_spans?: number;
}

export interface TraceStateCfg {
  key: string;
  values: string[];
}

export interface AndSubPolicyCfg {
  name: string;
  type: Exclude<PolicyType, "and">;
  latency?: LatencyCfg;
  numeric_attribute?: NumericAttributeCfg;
  probabilistic?: ProbabilisticCfg;
  status_code?: StatusCodeCfg;
  string_attribute?: StringAttributeCfg;
  rate_limiting?: RateLimitingCfg;
  boolean_attribute?: BooleanAttributeCfg;
  span_count?: SpanCountCfg;
  trace_state?: TraceStateCfg;
}

export interface AndCfg {
  and_sub_policy: AndSubPolicyCfg[];
}

export interface PolicyCfg {
  name: string;
  type: PolicyType;
  latency?: LatencyCfg;
  numeric_attribute?: NumericAttributeCfg;
  probabilistic?: ProbabilisticCfg;
  status_code?: StatusCodeCfg;
  string_attribute?: StringAttributeCfg;
  rate_limiting?: RateLimitingCfg;
  boolean_attribute?: BooleanAttributeCfg;
  span_count?: SpanCountCfg;
  trace_state?: TraceStateCfg;
  and?: AndCfg;
}

export interface PolicyView {
  decision_wait: string;
  num_traces?: number;
  policies: PolicyCfg[];
}

// Browser WebSocket protocol (spec.md ss33).

export interface SnapshotMsg {
  type: "snapshot";
  session: SessionSummary;
  collectors: CollectorSummary[];
  traces: TraceSummary[];
  policy: PolicyView;
  statistics: StatisticsView;
}

export interface TraceUpdatedMsg {
  type: "trace.updated";
  trace: TraceSummary;
}

export interface TraceDecidedMsg {
  type: "trace.decided";
  trace_id: string;
  decision: Decision;
  matched_policies: string[];
}

export interface StatisticsUpdatedMsg {
  type: "statistics.updated";
  statistics: StatisticsView;
}

export interface SessionStateMsg {
  type: "session.state";
  session: SessionSummary;
}

export interface CollectorConnectedMsg {
  type: "collector.connected";
  collector: CollectorSummary;
}

export interface CollectorDisconnectedMsg {
  type: "collector.disconnected";
  collector_id: string;
}

export interface PolicyUpdatedMsg {
  type: "policy.updated";
  policy: PolicyView;
}

export interface ErrorMsg {
  type: "error";
  message: string;
}

export type ServerMsg =
  | SnapshotMsg
  | TraceUpdatedMsg
  | TraceDecidedMsg
  | StatisticsUpdatedMsg
  | SessionStateMsg
  | CollectorConnectedMsg
  | CollectorDisconnectedMsg
  | PolicyUpdatedMsg
  | ErrorMsg;

export type ClientCmd =
  | { type: "session.start" }
  | { type: "session.pause" }
  | { type: "session.resume" }
  | { type: "session.stop" };
