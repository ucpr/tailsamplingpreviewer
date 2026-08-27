package trace

import (
	"testing"
	"time"

	"go.opentelemetry.io/collector/pdata/pcommon"
)

func TestState_String(t *testing.T) {
	tests := []struct {
		name string
		s    State
		want string
	}{
		{"receiving", StateReceiving, "RECEIVING"},
		{"ready", StateReady, "READY"},
		{"decided", StateDecided, "DECIDED"},
		{"expired", StateExpired, "EXPIRED"},
		{"unknown value", State(99), "UNKNOWN"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.s.String(); got != tt.want {
				t.Fatalf("String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDecision_String(t *testing.T) {
	tests := []struct {
		name string
		d    Decision
		want string
	}{
		{"pending", DecisionPending, "PENDING"},
		{"keep", DecisionKeep, "KEEP"},
		{"drop", DecisionDrop, "DROP"},
		{"unknown value", Decision(99), "PENDING"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.d.String(); got != tt.want {
				t.Fatalf("String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSpan_Duration(t *testing.T) {
	start := time.Unix(0, 0)
	s := Span{StartTime: start, EndTime: start.Add(250 * time.Millisecond)}
	if got := s.Duration(); got != 250*time.Millisecond {
		t.Fatalf("Duration() = %v, want 250ms", got)
	}
}

func spanAt(id byte, parent byte, startMs, endMs int64) Span {
	base := time.Unix(0, 0)
	var spanID, parentID pcommon.SpanID
	spanID[0] = id
	if parent != 0 {
		parentID[0] = parent
	}
	return Span{
		SpanID:       spanID,
		ParentSpanID: parentID,
		StartTime:    base.Add(time.Duration(startMs) * time.Millisecond),
		EndTime:      base.Add(time.Duration(endMs) * time.Millisecond),
	}
}

func TestTrace_RootSpan(t *testing.T) {
	tests := []struct {
		name       string
		spans      []Span
		wantOK     bool
		wantSpanID byte
	}{
		{
			name:   "no spans",
			spans:  nil,
			wantOK: false,
		},
		{
			name: "root identified by zero ParentSpanID",
			spans: []Span{
				spanAt(2, 1, 10, 20), // child of span 1
				spanAt(1, 0, 0, 100), // root
			},
			wantOK:     true,
			wantSpanID: 1,
		},
		{
			name: "no zero-parent span: earliest StartTime wins",
			spans: []Span{
				spanAt(2, 9, 50, 60),
				spanAt(1, 9, 10, 20),
			},
			wantOK:     true,
			wantSpanID: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tr := &Trace{Spans: tt.spans}
			got, ok := tr.RootSpan()
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if ok && got.SpanID[0] != tt.wantSpanID {
				t.Fatalf("root span id = %v, want %v", got.SpanID[0], tt.wantSpanID)
			}
		})
	}
}

func TestTrace_RootServiceName(t *testing.T) {
	tests := []struct {
		name  string
		trace *Trace
		want  string
	}{
		{
			name:  "no spans",
			trace: &Trace{},
			want:  "",
		},
		{
			name: "uses the root span's service name",
			trace: &Trace{Spans: []Span{
				{ServiceName: "child", ParentSpanID: pcommon.SpanID{1}},
				{ServiceName: "root"},
			}},
			want: "root",
		},
		{
			name: "falls back to the first span when the root has no service name",
			trace: &Trace{Spans: []Span{
				{ServiceName: "fallback"},
			}},
			want: "fallback",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.trace.RootServiceName(); got != tt.want {
				t.Fatalf("RootServiceName() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestTrace_Duration(t *testing.T) {
	tests := []struct {
		name  string
		spans []Span
		want  time.Duration
	}{
		{"no spans", nil, 0},
		{"single span", []Span{spanAt(1, 0, 0, 100)}, 100 * time.Millisecond},
		{
			name: "spans overlapping: overall span from earliest start to latest end",
			spans: []Span{
				spanAt(1, 0, 10, 20),
				spanAt(2, 1, 0, 50),
				spanAt(3, 1, 5, 15),
			},
			want: 50 * time.Millisecond,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tr := &Trace{Spans: tt.spans}
			if got := tr.Duration(); got != tt.want {
				t.Fatalf("Duration() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTrace_HasError(t *testing.T) {
	tests := []struct {
		name  string
		spans []Span
		want  bool
	}{
		{"no spans", nil, false},
		{"all ok", []Span{{StatusCode: StatusCodeOK}, {StatusCode: StatusCodeUnset}}, false},
		{"one error among many", []Span{{StatusCode: StatusCodeOK}, {StatusCode: StatusCodeError}}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tr := &Trace{Spans: tt.spans}
			if got := tr.HasError(); got != tt.want {
				t.Fatalf("HasError() = %v, want %v", got, tt.want)
			}
		})
	}
}
