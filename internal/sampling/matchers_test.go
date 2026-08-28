package sampling

import (
	"testing"
	"time"

	"go.opentelemetry.io/collector/pdata/pcommon"

	itrace "github.com/ucpr/tailsamplingpreviewer/internal/trace"
)

// spanSpec is a compact way to describe one span for a test trace; zero
// values are sensible defaults (StatusCodeUnset, 0ms duration, no attrs).
// attrs holds bare OTel attribute names (e.g. "retry_count") the way a
// PolicyCfg.Key refers to them; buildTrace stores them under the
// "attributes."-prefixed key internal/trace/assembler.go actually uses for
// a real span attribute, since that's what Evaluator's pdata rehydration
// (rehydrateTraceData) understands.
type spanSpec struct {
	statusCode itrace.StatusCode
	durationMs int64
	traceState string
	attrs      map[string]any
}

func buildTrace(specs ...spanSpec) *itrace.Trace {
	start := time.Unix(0, 0)
	spans := make([]itrace.Span, 0, len(specs))
	for _, sp := range specs {
		attrs := make(map[string]any, len(sp.attrs))
		for k, v := range sp.attrs {
			attrs["attributes."+k] = v
		}
		spans = append(spans, itrace.Span{
			StartTime:     start,
			EndTime:       start.Add(time.Duration(sp.durationMs) * time.Millisecond),
			StatusCode:    sp.statusCode,
			TraceStateRaw: sp.traceState,
			Attributes:    attrs,
		})
	}
	return &itrace.Trace{TraceID: pcommon.TraceID{1, 2, 3}, Spans: spans}
}

// traceWithID is buildTrace(spanSpec{}) with an explicit TraceID, for
// policies (probabilistic) whose decision depends on it.
func traceWithID(id pcommon.TraceID) *itrace.Trace {
	tr := buildTrace(spanSpec{})
	tr.TraceID = id
	return tr
}

// TestEvaluator_SinglePolicyMatch exercises every PolicyType's matching
// logic in isolation: one policy, one trace, does it match or not.
func TestEvaluator_SinglePolicyMatch(t *testing.T) {
	tests := []struct {
		name      string
		policy    PolicyCfg
		trace     *itrace.Trace
		wantMatch bool
	}{
		{
			name:      "always_sample matches anything",
			policy:    PolicyCfg{Name: "p", Type: AlwaysSample},
			trace:     buildTrace(spanSpec{}),
			wantMatch: true,
		},
		{
			// Strict lower bound, matching the real tailsamplingprocessor's
			// `latency` policy (see internal/upstreamtsp/sampling/latency.go):
			// duration must be greater than threshold_ms, not merely at least.
			name:      "latency at threshold does not match (strict lower bound)",
			policy:    PolicyCfg{Name: "p", Type: Latency, Latency: &LatencyCfg{ThresholdMs: 1000}},
			trace:     buildTrace(spanSpec{durationMs: 1000}),
			wantMatch: false,
		},
		{
			name:      "latency just above threshold matches",
			policy:    PolicyCfg{Name: "p", Type: Latency, Latency: &LatencyCfg{ThresholdMs: 1000}},
			trace:     buildTrace(spanSpec{durationMs: 1001}),
			wantMatch: true,
		},
		{
			name:      "latency below threshold does not match",
			policy:    PolicyCfg{Name: "p", Type: Latency, Latency: &LatencyCfg{ThresholdMs: 1000}},
			trace:     buildTrace(spanSpec{durationMs: 999}),
			wantMatch: false,
		},
		{
			name:      "latency above upper_threshold_ms does not match",
			policy:    PolicyCfg{Name: "p", Type: Latency, Latency: &LatencyCfg{ThresholdMs: 100, UpperThresholdMs: 1000}},
			trace:     buildTrace(spanSpec{durationMs: 2000}),
			wantMatch: false,
		},
		{
			name:      "status_code matches ERROR span",
			policy:    PolicyCfg{Name: "p", Type: StatusCode, StatusCode: &StatusCodeCfg{StatusCodes: []string{"ERROR"}}},
			trace:     buildTrace(spanSpec{statusCode: itrace.StatusCodeError}),
			wantMatch: true,
		},
		{
			name:      "status_code does not match OK span",
			policy:    PolicyCfg{Name: "p", Type: StatusCode, StatusCode: &StatusCodeCfg{StatusCodes: []string{"ERROR"}}},
			trace:     buildTrace(spanSpec{statusCode: itrace.StatusCodeOK}),
			wantMatch: false,
		},
		{
			name:      "numeric_attribute in range",
			policy:    PolicyCfg{Name: "p", Type: NumericAttribute, NumericAttribute: &NumericAttributeCfg{Key: "retry_count", MinValue: 1, MaxValue: 5}},
			trace:     buildTrace(spanSpec{attrs: map[string]any{"retry_count": int64(3)}}),
			wantMatch: true,
		},
		{
			name:      "numeric_attribute out of range",
			policy:    PolicyCfg{Name: "p", Type: NumericAttribute, NumericAttribute: &NumericAttributeCfg{Key: "retry_count", MinValue: 1, MaxValue: 5}},
			trace:     buildTrace(spanSpec{attrs: map[string]any{"retry_count": int64(9)}}),
			wantMatch: false,
		},
		{
			name:      "numeric_attribute missing key does not match",
			policy:    PolicyCfg{Name: "p", Type: NumericAttribute, NumericAttribute: &NumericAttributeCfg{Key: "retry_count", MinValue: 1, MaxValue: 5}},
			trace:     buildTrace(spanSpec{}),
			wantMatch: false,
		},
		{
			name:      "numeric_attribute invert_match flips a match to a miss",
			policy:    PolicyCfg{Name: "p", Type: NumericAttribute, NumericAttribute: &NumericAttributeCfg{Key: "retry_count", MinValue: 1, MaxValue: 5, InvertMatch: true}},
			trace:     buildTrace(spanSpec{attrs: map[string]any{"retry_count": int64(3)}}),
			wantMatch: false,
		},
		{
			name:      "numeric_attribute invert_match flips a miss to a match",
			policy:    PolicyCfg{Name: "p", Type: NumericAttribute, NumericAttribute: &NumericAttributeCfg{Key: "retry_count", MinValue: 1, MaxValue: 5, InvertMatch: true}},
			trace:     buildTrace(spanSpec{attrs: map[string]any{"retry_count": int64(9)}}),
			wantMatch: true,
		},
		{
			name:      "string_attribute exact value match",
			policy:    PolicyCfg{Name: "p", Type: StringAttribute, StringAttribute: &StringAttributeCfg{Key: "service.name", Values: []string{"payment"}}},
			trace:     buildTrace(spanSpec{attrs: map[string]any{"service.name": "payment"}}),
			wantMatch: true,
		},
		{
			name:      "string_attribute no value match",
			policy:    PolicyCfg{Name: "p", Type: StringAttribute, StringAttribute: &StringAttributeCfg{Key: "service.name", Values: []string{"payment"}}},
			trace:     buildTrace(spanSpec{attrs: map[string]any{"service.name": "catalog"}}),
			wantMatch: false,
		},
		{
			name:      "string_attribute regex match",
			policy:    PolicyCfg{Name: "p", Type: StringAttribute, StringAttribute: &StringAttributeCfg{Key: "service.name", Values: []string{"^pay.*"}, EnabledRegexMatching: true}},
			trace:     buildTrace(spanSpec{attrs: map[string]any{"service.name": "payment-api"}}),
			wantMatch: true,
		},
		{
			name:      "string_attribute regex no match",
			policy:    PolicyCfg{Name: "p", Type: StringAttribute, StringAttribute: &StringAttributeCfg{Key: "service.name", Values: []string{"^pay.*"}, EnabledRegexMatching: true}},
			trace:     buildTrace(spanSpec{attrs: map[string]any{"service.name": "catalog"}}),
			wantMatch: false,
		},
		{
			name:      "string_attribute invert_match",
			policy:    PolicyCfg{Name: "p", Type: StringAttribute, StringAttribute: &StringAttributeCfg{Key: "service.name", Values: []string{"payment"}, InvertMatch: true}},
			trace:     buildTrace(spanSpec{attrs: map[string]any{"service.name": "catalog"}}),
			wantMatch: true,
		},
		{
			name:      "boolean_attribute true matches true",
			policy:    PolicyCfg{Name: "p", Type: BooleanAttribute, BooleanAttribute: &BooleanAttributeCfg{Key: "premium", Value: true}},
			trace:     buildTrace(spanSpec{attrs: map[string]any{"premium": true}}),
			wantMatch: true,
		},
		{
			name:      "boolean_attribute false does not match true",
			policy:    PolicyCfg{Name: "p", Type: BooleanAttribute, BooleanAttribute: &BooleanAttributeCfg{Key: "premium", Value: true}},
			trace:     buildTrace(spanSpec{attrs: map[string]any{"premium": false}}),
			wantMatch: false,
		},
		{
			name:      "boolean_attribute invert_match",
			policy:    PolicyCfg{Name: "p", Type: BooleanAttribute, BooleanAttribute: &BooleanAttributeCfg{Key: "premium", Value: true, InvertMatch: true}},
			trace:     buildTrace(spanSpec{attrs: map[string]any{"premium": false}}),
			wantMatch: true,
		},
		{
			name:      "span_count within [min,max]",
			policy:    PolicyCfg{Name: "p", Type: SpanCount, SpanCount: &SpanCountCfg{MinSpans: 2, MaxSpans: 3}},
			trace:     buildTrace(spanSpec{}, spanSpec{}),
			wantMatch: true,
		},
		{
			name:      "span_count below min",
			policy:    PolicyCfg{Name: "p", Type: SpanCount, SpanCount: &SpanCountCfg{MinSpans: 2}},
			trace:     buildTrace(spanSpec{}),
			wantMatch: false,
		},
		{
			name:      "span_count above max",
			policy:    PolicyCfg{Name: "p", Type: SpanCount, SpanCount: &SpanCountCfg{MinSpans: 1, MaxSpans: 2}},
			trace:     buildTrace(spanSpec{}, spanSpec{}, spanSpec{}),
			wantMatch: false,
		},
		{
			name:      "trace_state key/value match",
			policy:    PolicyCfg{Name: "p", Type: TraceState, TraceState: &TraceStateCfg{Key: "vendor", Values: []string{"otel"}}},
			trace:     buildTrace(spanSpec{traceState: "vendor=otel,other=x"}),
			wantMatch: true,
		},
		{
			name:      "trace_state value mismatch",
			policy:    PolicyCfg{Name: "p", Type: TraceState, TraceState: &TraceStateCfg{Key: "vendor", Values: []string{"otel"}}},
			trace:     buildTrace(spanSpec{traceState: "vendor=other"}),
			wantMatch: false,
		},
		{
			name:      "trace_state missing key",
			policy:    PolicyCfg{Name: "p", Type: TraceState, TraceState: &TraceStateCfg{Key: "vendor", Values: []string{"otel"}}},
			trace:     buildTrace(spanSpec{traceState: ""}),
			wantMatch: false,
		},
		{
			name:      "probabilistic 100% always matches",
			policy:    PolicyCfg{Name: "p", Type: Probabilistic, Probabilistic: &ProbabilisticCfg{SamplingPercentage: 100}},
			trace:     buildTrace(spanSpec{}),
			wantMatch: true,
		},
		{
			name:      "probabilistic 0% never matches",
			policy:    PolicyCfg{Name: "p", Type: Probabilistic, Probabilistic: &ProbabilisticCfg{SamplingPercentage: 0}},
			trace:     buildTrace(spanSpec{}),
			wantMatch: false,
		},
		{
			// Pins down the exact algorithm (FNV-1a 64-bit hash of
			// hash_salt+TraceID vs. a big.Float-computed threshold, see
			// buildEvaluator's Probabilistic case) against a fixed
			// TraceID/salt/percentage combination, matching what the real
			// tailsamplingprocessor computes for the same inputs.
			name: "probabilistic with an explicit hash_salt",
			policy: PolicyCfg{Name: "p", Type: Probabilistic, Probabilistic: &ProbabilisticCfg{
				HashSalt: "seed", SamplingPercentage: 50,
			}},
			trace:     traceWithID(pcommon.TraceID{7, 7, 7, 7}),
			wantMatch: true,
		},
		{
			name: "probabilistic falls back to the default hash_salt when unset",
			policy: PolicyCfg{Name: "p", Type: Probabilistic, Probabilistic: &ProbabilisticCfg{
				SamplingPercentage: 50,
			}},
			trace:     traceWithID(pcommon.TraceID{3, 1, 4, 1, 5}),
			wantMatch: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ev := newEvaluatorT(t, Config{Policies: []PolicyCfg{tt.policy}})
			res, err := ev.Evaluate(tt.trace)
			if err != nil {
				t.Fatalf("Evaluate: %v", err)
			}
			gotMatch := len(res.MatchedNames()) == 1

			if gotMatch != tt.wantMatch {
				t.Fatalf("match = %v, want %v (result: %+v)", gotMatch, tt.wantMatch, res)
			}
			wantDecision := itrace.DecisionDrop
			if tt.wantMatch {
				wantDecision = itrace.DecisionKeep
			}
			if res.Decision != wantDecision {
				t.Fatalf("decision = %v, want %v", res.Decision, wantDecision)
			}
		})
	}
}

// TestNewEvaluator_UnknownPolicyType asserts construction fails fast for an
// unrecognized policy type, the same way a real Collector would refuse to
// build a processor for an unknown policy at config-validation time,
// rather than silently building an evaluator that can never match.
func TestNewEvaluator_UnknownPolicyType(t *testing.T) {
	cfg := Config{Policies: []PolicyCfg{{Name: "p", Type: PolicyType("nonsense")}}}
	if _, err := NewEvaluator(cfg); err == nil {
		t.Fatal("expected NewEvaluator to reject an unknown policy type, got nil error")
	}
}

// TestEvaluator_RateLimiting cannot be a flat table since the policy is
// stateful across calls: it's a token bucket (golang.org/x/time/rate,
// matching the real tailsamplingprocessor -- see
// internal/upstreamtsp/sampling/rate_limiting.go) refilling at
// SpansPerSecond with a burst capacity of 2x that rate.
func TestEvaluator_RateLimiting(t *testing.T) {
	tests := []struct {
		name           string
		spansPerSecond int64
		callSpanCounts []int64 // number of spans in each successive trace, evaluated at the same instant
		wantMatches    []bool
	}{
		{
			name:           "first call within the doubled burst matches, second exceeding what's left does not",
			spansPerSecond: 5, // burst capacity 10
			callSpanCounts: []int64{8, 4},
			wantMatches:    []bool{true, false},
		},
		{
			name:           "calls exactly filling the burst all match",
			spansPerSecond: 4, // burst capacity 8
			callSpanCounts: []int64{4, 4},
			wantMatches:    []bool{true, true},
		},
		{
			name:           "a single call larger than the whole burst never matches",
			spansPerSecond: 2, // burst capacity 4
			callSpanCounts: []int64{5},
			wantMatches:    []bool{false},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			policy := PolicyCfg{Name: "p", Type: RateLimiting, RateLimiting: &RateLimitingCfg{SpansPerSecond: tt.spansPerSecond}}
			ev := newEvaluatorT(t, Config{Policies: []PolicyCfg{policy}})

			for i, n := range tt.callSpanCounts {
				specs := make([]spanSpec, n)
				trace := buildTrace(specs...)
				res, err := ev.Evaluate(trace)
				if err != nil {
					t.Fatalf("call %d: Evaluate: %v", i, err)
				}
				gotMatch := len(res.MatchedNames()) == 1
				if gotMatch != tt.wantMatches[i] {
					t.Fatalf("call %d: match = %v, want %v", i, gotMatch, tt.wantMatches[i])
				}
			}
		})
	}
}

// TestEvaluator_RateLimiting_Refill checks that the token bucket actually
// refills over real elapsed time (not just that a single instant's burst
// eventually runs out, which TestEvaluator_RateLimiting already covers). A
// real sleep is required: rate_limiting's vendored Evaluate
// (internal/upstreamtsp/sampling/rate_limiting.go) calls time.Now()
// directly rather than accepting an injectable clock, matching how a real
// Collector's rate limiter can't be driven by synthetic timestamps either.
func TestEvaluator_RateLimiting_Refill(t *testing.T) {
	policy := PolicyCfg{Name: "p", Type: RateLimiting, RateLimiting: &RateLimitingCfg{SpansPerSecond: 1}}
	ev := newEvaluatorT(t, Config{Policies: []PolicyCfg{policy}})
	tr := buildTrace(spanSpec{})

	assertMatch := func(t *testing.T, want bool) {
		t.Helper()
		res, err := ev.Evaluate(tr)
		if err != nil {
			t.Fatalf("Evaluate: %v", err)
		}
		if got := len(res.MatchedNames()) == 1; got != want {
			t.Fatalf("match = %v, want %v", got, want)
		}
	}

	// Burst capacity is 2x the rate (2 tokens here).
	assertMatch(t, true)
	assertMatch(t, true)
	assertMatch(t, false) // burst exhausted

	time.Sleep(1100 * time.Millisecond)
	assertMatch(t, true) // one token refilled at 1/s
	assertMatch(t, false)
}
