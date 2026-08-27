package query

import (
	"testing"
	"time"

	"go.opentelemetry.io/collector/pdata/pcommon"

	itrace "github.com/ucpr/tailsamplingpreviewer/internal/trace"
)

func mkTrace(svc string, statusErr bool, durationMs int64) *itrace.Trace {
	start := time.Unix(0, 0)
	return &itrace.Trace{
		TraceID: pcommon.TraceID{9},
		Spans: []itrace.Span{
			{
				Name:        "handle",
				ServiceName: svc,
				StartTime:   start,
				EndTime:     start.Add(time.Duration(durationMs) * time.Millisecond),
				StatusCode:  map[bool]itrace.StatusCode{true: itrace.StatusCodeError, false: itrace.StatusCodeOK}[statusErr],
			},
		},
	}
}

func TestFilter_Empty(t *testing.T) {
	if !Parse("").Empty() {
		t.Fatal("expected empty filter for empty query")
	}
}

func TestFilter_Status(t *testing.T) {
	f := Parse("status:error")
	if !f.Match(mkTrace("payment", true, 10)) {
		t.Fatal("expected error trace to match status:error")
	}
	if f.Match(mkTrace("payment", false, 10)) {
		t.Fatal("expected OK trace not to match status:error")
	}
}

func TestFilter_Duration(t *testing.T) {
	tests := []struct {
		name       string
		query      string
		durationMs int64
		want       bool
	}{
		{"greater-than, below threshold", "duration:>1s", 500, false},
		{"greater-than, above threshold", "duration:>1s", 1500, true},
		{"greater-than-or-equal, exactly at threshold", "duration:>=1s", 1000, true},
		{"less-than, below threshold", "duration:<1s", 500, true},
		{"less-than, above threshold", "duration:<1s", 1500, false},
		{"less-than-or-equal, exactly at threshold", "duration:<=1s", 1000, true},
		{"bare colon means exact equality", "duration:1s", 1000, true},
		{"bare colon, not equal", "duration:1s", 1500, false},
		{"unparsable duration never matches", "duration:not-a-duration", 1000, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := Parse(tt.query)
			if got := f.Match(mkTrace("payment", false, tt.durationMs)); got != tt.want {
				t.Fatalf("Match() = %v, want %v (query=%q, durationMs=%d)", got, tt.want, tt.query, tt.durationMs)
			}
		})
	}
}

func TestFilter_ServiceNameAndCombination(t *testing.T) {
	f := Parse("service.name:payment status:error")
	if !f.Match(mkTrace("payment-api", true, 10)) {
		t.Fatal("expected substring service.name match combined with status:error to match")
	}
	if f.Match(mkTrace("catalog", true, 10)) {
		t.Fatal("expected non-matching service.name to fail the AND")
	}
	if f.Match(mkTrace("payment-api", false, 10)) {
		t.Fatal("expected non-matching status to fail the AND")
	}
}

func mkTraceWithAttrs(attrs map[string]any) *itrace.Trace {
	return &itrace.Trace{
		TraceID: pcommon.TraceID{9},
		Spans: []itrace.Span{
			{Name: "handle", Attributes: attrs},
		},
	}
}

func TestFilter_GenericAttribute(t *testing.T) {
	tests := []struct {
		name  string
		query string
		attrs map[string]any
		want  bool
	}{
		{"string attribute exact match", "attributes.route:/pay", map[string]any{"attributes.route": "/pay"}, true},
		{"string attribute case-insensitive", "attributes.route:/PAY", map[string]any{"attributes.route": "/pay"}, true},
		{"string attribute mismatch", "attributes.route:/pay", map[string]any{"attributes.route": "/other"}, false},
		{"float64 attribute matches its decimal string", "attributes.duration_ms:250", map[string]any{"attributes.duration_ms": float64(250)}, true},
		{"int64 attribute matches its decimal string", "attributes.retry_count:3", map[string]any{"attributes.retry_count": int64(3)}, true},
		{"bool attribute matches true/false text", "attributes.premium:true", map[string]any{"attributes.premium": true}, true},
		{"bool attribute mismatch", "attributes.premium:true", map[string]any{"attributes.premium": false}, false},
		{"missing key never matches", "attributes.route:/pay", map[string]any{}, false},
		{"unsupported value type never matches", "attributes.blob:x", map[string]any{"attributes.blob": []int{1, 2}}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := Parse(tt.query)
			if got := f.Match(mkTraceWithAttrs(tt.attrs)); got != tt.want {
				t.Fatalf("Match() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFilter_FreeText(t *testing.T) {
	f := Parse("handle")
	if !f.Match(mkTrace("payment", false, 10)) {
		t.Fatal("expected free text to match span name")
	}
	if Parse("nomatch").Match(mkTrace("payment", false, 10)) {
		t.Fatal("expected non-matching free text to fail")
	}
}
