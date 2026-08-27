package server

import (
	"time"

	"github.com/ucpr/tailsamplingpreviewer/internal/sampling"
	"github.com/ucpr/tailsamplingpreviewer/internal/session"
	"github.com/ucpr/tailsamplingpreviewer/internal/statistics"
)

// Envelope-typed events pushed from the Preview Server to the Browser
// (spec.md ss33). Every Browser<->Server message is JSON over a WebSocket
// Text frame; unlike the Collector protocol there is no binary plane.
const (
	evtSnapshot            = "snapshot"
	evtTraceUpdated        = "trace.updated"
	evtTraceDecided        = "trace.decided"
	evtStatisticsUpdated   = "statistics.updated"
	evtSessionState        = "session.state"
	evtCollectorConnected  = "collector.connected"
	evtCollectorDisconnect = "collector.disconnected"
	evtPolicyUpdated       = "policy.updated"
	evtError               = "error"

	cmdSessionStart  = "session.start"
	cmdSessionPause  = "session.pause"
	cmdSessionResume = "session.resume"
	cmdSessionStop   = "session.stop"
	cmdPolicyImport  = "policy.import"
)

type envelope struct {
	Type string `json:"type"`
}

// TraceSummary is the Live Tail row projection (spec.md ss26, ss28): only
// what the UI needs, never the raw OTLP payload (spec.md ss33).
type TraceSummary struct {
	TraceID         string    `json:"trace_id"`
	ServiceName     string    `json:"service_name"`
	RootSpanName    string    `json:"root_span_name"`
	DurationMs      int64     `json:"duration_ms"`
	HasError        bool      `json:"has_error"`
	SpanCount       int       `json:"span_count"`
	State           string    `json:"state"`
	Decision        string    `json:"decision"`
	MatchedPolicies []string  `json:"matched_policies"`
	FirstSeen       time.Time `json:"first_seen"`
	LastSeen        time.Time `json:"last_seen"`
}

// SpanSummary is one row of the Trace Detail waterfall (spec.md ss28). The
// Attributes map lets the UI show real attribute keys/values so a user can
// reference them while writing Policy Builder predicates (spec.md ss25).
type SpanSummary struct {
	SpanID       string         `json:"span_id"`
	ParentSpanID string         `json:"parent_span_id"`
	Name         string         `json:"name"`
	ServiceName  string         `json:"service_name"`
	StartTime    time.Time      `json:"start_time"`
	EndTime      time.Time      `json:"end_time"`
	DurationMs   int64          `json:"duration_ms"`
	StatusCode   string         `json:"status_code"`
	Attributes   map[string]any `json:"attributes"`
}

// TraceDetail extends TraceSummary with every span, for the Trace Detail
// view (spec.md ss28).
type TraceDetail struct {
	TraceSummary
	Spans []SpanSummary `json:"spans"`
}

// CollectorSummary is one row of the "Collectors" panel (spec.md ss17, ss18).
type CollectorSummary struct {
	ID        string    `json:"id"`
	Version   string    `json:"version"`
	Connected bool      `json:"connected"`
	SpansSent uint64    `json:"spans_received"`
	LastSeen  time.Time `json:"last_seen"`
}

// SessionSummary mirrors the "Preview Session" UI panel (spec.md ss9).
type SessionSummary struct {
	State     string `json:"state"`
	SessionID string `json:"session_id"`
	StartedAt string `json:"started_at,omitempty"`
	Spans     uint64 `json:"spans"`
	Traces    uint64 `json:"traces"`
}

func sessionSummary(snap session.Snapshot, spans, traces uint64) SessionSummary {
	s := SessionSummary{State: snap.State.String(), SessionID: snap.SessionID, Spans: spans, Traces: traces}
	if !snap.StartedAt.IsZero() {
		s.StartedAt = snap.StartedAt.Format(time.RFC3339)
	}
	return s
}

// PolicyView is the Config re-exported with JSON tags for the Policy
// Builder UI and REST API (spec.md ss25).
type PolicyView = sampling.Config

// StatisticsView re-exports statistics.Snapshot for the REST API / WS
// events (spec.md ss29).
type StatisticsView = statistics.Snapshot

// Every outbound message embeds a payload struct with an explicit "type"
// field so json.Marshal flattens it to a single top-level object, e.g.
// {"type":"snapshot","session":{...},...}.

type snapshotMsg struct {
	Type       string             `json:"type"`
	Session    SessionSummary     `json:"session"`
	Collectors []CollectorSummary `json:"collectors"`
	Traces     []TraceSummary     `json:"traces"`
	Policy     PolicyView         `json:"policy"`
	Statistics StatisticsView     `json:"statistics"`
}

type traceUpdatedMsg struct {
	Type  string       `json:"type"`
	Trace TraceSummary `json:"trace"`
}

type traceDecidedMsg struct {
	Type            string   `json:"type"`
	TraceID         string   `json:"trace_id"`
	Decision        string   `json:"decision"`
	MatchedPolicies []string `json:"matched_policies"`
}

type statisticsUpdatedMsg struct {
	Type       string         `json:"type"`
	Statistics StatisticsView `json:"statistics"`
}

type sessionStateMsg struct {
	Type    string         `json:"type"`
	Session SessionSummary `json:"session"`
}

type collectorConnectedMsg struct {
	Type      string           `json:"type"`
	Collector CollectorSummary `json:"collector"`
}

type collectorDisconnectedMsg struct {
	Type        string `json:"type"`
	CollectorID string `json:"collector_id"`
}

type policyUpdatedMsg struct {
	Type   string     `json:"type"`
	Policy PolicyView `json:"policy"`
}

type errorMsg struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}
