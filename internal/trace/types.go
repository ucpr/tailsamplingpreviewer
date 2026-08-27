// Package trace assembles OTLP spans received from Collectors into
// per-trace-id aggregates that the sampling and statistics engines can
// evaluate (spec.md ss20).
package trace

import (
	"time"

	"go.opentelemetry.io/collector/pdata/pcommon"
)

// StatusCode mirrors the OTLP span status code without requiring callers to
// depend on pdata/ptrace directly.
type StatusCode int32

const (
	StatusCodeUnset StatusCode = 0
	StatusCodeOK    StatusCode = 1
	StatusCodeError StatusCode = 2
)

// Span is a flattened, evaluation-friendly projection of an OTLP span plus
// the resource/scope it was reported under.
type Span struct {
	TraceID      pcommon.TraceID
	SpanID       pcommon.SpanID
	ParentSpanID pcommon.SpanID

	Name string
	Kind int32

	StartTime time.Time
	EndTime   time.Time

	StatusCode    StatusCode
	StatusMessage string

	TraceStateRaw string

	// Attributes merges span attributes ("attributes.foo"), resource
	// attributes ("resource.foo") and a handful of well-known top level
	// keys ("service.name", "duration_ms", "status.code", "name", "kind")
	// so that policy predicates can address them with a single flat key,
	// matching the dotted-key style used in spec.md ss25 / ss27.
	Attributes map[string]any

	ServiceName string

	EventCount int
	LinkCount  int
}

// Duration returns the span's wall-clock duration.
func (s Span) Duration() time.Duration {
	return s.EndTime.Sub(s.StartTime)
}

// State is the lifecycle of a Trace aggregate (spec.md ss20).
type State int

const (
	StateReceiving State = iota
	StateReady
	StateDecided
	StateExpired
)

func (s State) String() string {
	switch s {
	case StateReceiving:
		return "RECEIVING"
	case StateReady:
		return "READY"
	case StateDecided:
		return "DECIDED"
	case StateExpired:
		return "EXPIRED"
	default:
		return "UNKNOWN"
	}
}

// Decision is the KEEP/DROP outcome of policy evaluation for a Trace.
type Decision int

const (
	DecisionPending Decision = iota
	DecisionKeep
	DecisionDrop
)

func (d Decision) String() string {
	switch d {
	case DecisionKeep:
		return "KEEP"
	case DecisionDrop:
		return "DROP"
	default:
		return "PENDING"
	}
}

// Trace is the per-trace-id aggregate built by the Assembler (spec.md
// ss20, "TraceState" in the design doc; named Trace here to avoid clashing
// with the W3C tracestate span field).
type Trace struct {
	TraceID pcommon.TraceID

	FirstSeen time.Time
	LastSeen  time.Time

	Spans []Span

	SizeBytes uint64

	State State

	Decision        Decision
	MatchedPolicies []string
}

// RootSpan returns the span with no parent (or the earliest span if every
// span has a parent, e.g. because the root has not arrived yet).
func (t *Trace) RootSpan() (Span, bool) {
	if len(t.Spans) == 0 {
		return Span{}, false
	}
	var zero pcommon.SpanID
	for _, s := range t.Spans {
		if s.ParentSpanID == zero {
			return s, true
		}
	}
	root := t.Spans[0]
	for _, s := range t.Spans[1:] {
		if s.StartTime.Before(root.StartTime) {
			root = s
		}
	}
	return root, true
}

// RootServiceName returns the service.name of the root span, used as the
// trace's display name in Live Tail (spec.md ss26, ss28).
func (t *Trace) RootServiceName() string {
	if s, ok := t.RootSpan(); ok && s.ServiceName != "" {
		return s.ServiceName
	}
	if len(t.Spans) > 0 {
		return t.Spans[0].ServiceName
	}
	return ""
}

// Duration returns the trace's overall wall-clock duration, from the
// earliest span start to the latest span end.
func (t *Trace) Duration() time.Duration {
	if len(t.Spans) == 0 {
		return 0
	}
	start := t.Spans[0].StartTime
	end := t.Spans[0].EndTime
	for _, s := range t.Spans[1:] {
		if s.StartTime.Before(start) {
			start = s.StartTime
		}
		if s.EndTime.After(end) {
			end = s.EndTime
		}
	}
	return end.Sub(start)
}

// HasError reports whether any span in the trace has an error status,
// matching the "ERROR" badge shown in spec.md ss22/ss26/ss28.
func (t *Trace) HasError() bool {
	for _, s := range t.Spans {
		if s.StatusCode == StatusCodeError {
			return true
		}
	}
	return false
}
