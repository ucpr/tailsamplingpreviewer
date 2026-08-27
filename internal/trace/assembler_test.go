package trace

import (
	"testing"
	"time"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

func TestAssemble_GroupsSpansByTraceID(t *testing.T) {
	td := ptrace.NewTraces()
	rs := td.ResourceSpans().AppendEmpty()
	rs.Resource().Attributes().PutStr("service.name", "payment-api")
	spans := rs.ScopeSpans().AppendEmpty().Spans()

	var trace1, trace2 pcommon.TraceID
	trace1[0] = 1
	trace2[0] = 2

	s1 := spans.AppendEmpty()
	s1.SetTraceID(trace1)
	s1.SetName("a")
	s2 := spans.AppendEmpty()
	s2.SetTraceID(trace1)
	s2.SetName("b")
	s3 := spans.AppendEmpty()
	s3.SetTraceID(trace2)
	s3.SetName("c")

	got := Assemble(td)

	if len(got) != 2 {
		t.Fatalf("expected 2 distinct trace ids, got %d: %+v", len(got), got)
	}
	if len(got[trace1]) != 2 {
		t.Fatalf("expected 2 spans for trace1, got %d", len(got[trace1]))
	}
	if len(got[trace2]) != 1 {
		t.Fatalf("expected 1 span for trace2, got %d", len(got[trace2]))
	}
}

func TestAssemble_SpanFieldMapping(t *testing.T) {
	tests := []struct {
		name           string
		statusCode     ptrace.StatusCode
		wantStatus     StatusCode
		wantStatusAttr string
	}{
		{"ok status", ptrace.StatusCodeOk, StatusCodeOK, "OK"},
		{"error status", ptrace.StatusCodeError, StatusCodeError, "ERROR"},
		{"unset status", ptrace.StatusCodeUnset, StatusCodeUnset, "UNSET"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			td := ptrace.NewTraces()
			rs := td.ResourceSpans().AppendEmpty()
			rs.Resource().Attributes().PutStr("service.name", "payment-api")
			rs.Resource().Attributes().PutStr("deployment.environment", "prod")

			span := rs.ScopeSpans().AppendEmpty().Spans().AppendEmpty()
			var id pcommon.TraceID
			id[0] = 7
			span.SetTraceID(id)
			span.SetName("handle-payment")
			span.Attributes().PutStr("http.method", "POST")
			start := time.Unix(1000, 0)
			span.SetStartTimestamp(pcommon.NewTimestampFromTime(start))
			span.SetEndTimestamp(pcommon.NewTimestampFromTime(start.Add(250 * time.Millisecond)))
			span.Status().SetCode(tt.statusCode)

			got := Assemble(td)
			spans := got[id]
			if len(spans) != 1 {
				t.Fatalf("expected exactly 1 span, got %d", len(spans))
			}
			s := spans[0]

			if s.ServiceName != "payment-api" {
				t.Errorf("ServiceName = %q, want %q", s.ServiceName, "payment-api")
			}
			if s.Name != "handle-payment" {
				t.Errorf("Name = %q, want %q", s.Name, "handle-payment")
			}
			if s.StatusCode != tt.wantStatus {
				t.Errorf("StatusCode = %v, want %v", s.StatusCode, tt.wantStatus)
			}
			if s.Duration() != 250*time.Millisecond {
				t.Errorf("Duration() = %v, want 250ms", s.Duration())
			}

			if got := s.Attributes["resource.deployment.environment"]; got != "prod" {
				t.Errorf(`Attributes["resource.deployment.environment"] = %v, want "prod"`, got)
			}
			if got := s.Attributes["attributes.http.method"]; got != "POST" {
				t.Errorf(`Attributes["attributes.http.method"] = %v, want "POST"`, got)
			}
			if got := s.Attributes["service.name"]; got != "payment-api" {
				t.Errorf(`Attributes["service.name"] = %v, want "payment-api"`, got)
			}
			if got := s.Attributes["status.code"]; got != tt.wantStatusAttr {
				t.Errorf(`Attributes["status.code"] = %v, want %q`, got, tt.wantStatusAttr)
			}
			if got := s.Attributes["duration_ms"]; got != float64(250) {
				t.Errorf(`Attributes["duration_ms"] = %v, want 250`, got)
			}
		})
	}
}
