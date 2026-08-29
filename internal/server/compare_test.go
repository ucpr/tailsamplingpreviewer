package server

import (
	"testing"
	"time"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.uber.org/zap"

	"github.com/ucpr/tailsamplingpreviewer/internal/sampling"
	itrace "github.com/ucpr/tailsamplingpreviewer/internal/trace"
)

// mkCompareSpan builds a one-span trace fixture with the resource
// attribute a string_attribute policy keyed "service.name" reads
// (internal/trace/assembler.go's real shape: "resource."-prefixed), a
// controllable duration for a latency policy, and a controllable status
// for HasError().
func mkCompareSpan(now time.Time, service string, durationMs int64, hasError bool) itrace.Span {
	code := itrace.StatusCodeOK
	if hasError {
		code = itrace.StatusCodeError
	}
	return itrace.Span{
		Name:        "handle",
		ServiceName: service,
		StartTime:   now,
		EndTime:     now.Add(time.Duration(durationMs) * time.Millisecond),
		StatusCode:  code,
		Attributes:  map[string]any{"resource.service.name": service},
	}
}

// decideAll forces every StateReceiving trace in the store through
// decideDue, using a decision_wait far in the past so DueForDecision picks
// them all up regardless of when they were ingested.
func decideAll(srv *Server) {
	srv.decideDue(time.Now().Add(time.Hour))
}

// TestComparePolicy_NewlyKeptAndDropped is the primary scenario: current
// and candidate are deliberately independent dimensions (service name vs.
// duration) so every quadrant -- unchanged-keep, unchanged-drop,
// newly-kept, newly-dropped-with-error, newly-dropped-without-error -- is
// reachable, proving ErrorTracesNewlyDropped is a proper subset of
// NewlyDropped rather than always equal to it.
func TestComparePolicy_NewlyKeptAndDropped(t *testing.T) {
	srv, err := New(zap.NewNop(), Config{
		InitialPolicy: sampling.Config{
			DecisionWait: sampling.Duration(time.Second),
			Policies: []sampling.PolicyCfg{
				{Name: "payment", Type: sampling.StringAttribute, StringAttribute: &sampling.StringAttributeCfg{
					Key: "service.name", Values: []string{"payment"},
				}},
			},
		},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	now := time.Now()
	spans := map[pcommon.TraceID][]itrace.Span{
		{1}: {mkCompareSpan(now, "payment", 100, false)},  // current KEEP, candidate DROP, no error
		{2}: {mkCompareSpan(now, "payment", 100, true)},   // current KEEP, candidate DROP, error
		{3}: {mkCompareSpan(now, "payment", 1000, false)}, // current KEEP, candidate KEEP (unchanged)
		{4}: {mkCompareSpan(now, "catalog", 1000, false)}, // current DROP, candidate KEEP
		{5}: {mkCompareSpan(now, "catalog", 1000, true)},  // current DROP, candidate KEEP
		{6}: {mkCompareSpan(now, "catalog", 100, false)},  // current DROP, candidate DROP (unchanged)
	}
	srv.store.Ingest(now, spans)
	decideAll(srv)

	candidate := sampling.Config{
		DecisionWait: sampling.Duration(time.Second),
		Policies: []sampling.PolicyCfg{
			{Name: "slow", Type: sampling.Latency, Latency: &sampling.LatencyCfg{ThresholdMs: 500}},
		},
	}

	result, err := srv.comparePolicy(candidate)
	if err != nil {
		t.Fatalf("comparePolicy: %v", err)
	}

	wantCurrent := PolicySnapshot{Observed: 6, Keep: 3, Drop: 3, SamplingRate: 0.5}
	wantCandidate := PolicySnapshot{Observed: 6, Keep: 3, Drop: 3, SamplingRate: 0.5}
	if result.Current != wantCurrent {
		t.Fatalf("Current = %+v, want %+v", result.Current, wantCurrent)
	}
	if result.Candidate != wantCandidate {
		t.Fatalf("Candidate = %+v, want %+v", result.Candidate, wantCandidate)
	}
	if result.NewlyKept != 2 {
		t.Fatalf("NewlyKept = %d, want 2", result.NewlyKept)
	}
	if result.NewlyDropped != 2 {
		t.Fatalf("NewlyDropped = %d, want 2", result.NewlyDropped)
	}
	if result.ErrorTracesNewlyDropped != 1 {
		t.Fatalf("ErrorTracesNewlyDropped = %d, want 1 (strictly less than NewlyDropped=%d)", result.ErrorTracesNewlyDropped, result.NewlyDropped)
	}

	// comparePolicy must not mutate anything: the live policy, the
	// traces' stored decisions, and statistics must all be untouched.
	if got := srv.Policy(); len(got.Policies) != 1 || got.Policies[0].Name != "payment" {
		t.Fatalf("active policy changed after comparePolicy: %+v", got)
	}
	if stats := srv.stats.Snapshot(); stats.Observed != 6 || stats.Keep != 3 {
		t.Fatalf("statistics changed after comparePolicy: %+v", stats)
	}
}

// TestComparePolicy_ExcludesUndecidedTraces asserts a trace that hasn't
// reached StateDecided yet (its decision_wait hasn't elapsed) never
// contributes to either side of the comparison.
func TestComparePolicy_ExcludesUndecidedTraces(t *testing.T) {
	srv, err := New(zap.NewNop(), Config{
		InitialPolicy: sampling.Config{
			DecisionWait: sampling.Duration(time.Hour), // never elapses in this test
			Policies: []sampling.PolicyCfg{
				{Name: "all", Type: sampling.AlwaysSample},
			},
		},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	now := time.Now()
	srv.store.Ingest(now, map[pcommon.TraceID][]itrace.Span{
		{9}: {mkCompareSpan(now, "payment", 100, false)},
	})
	// Deliberately no decideDue call: the trace stays StateReceiving.

	result, err := srv.comparePolicy(sampling.Config{
		Policies: []sampling.PolicyCfg{{Name: "all", Type: sampling.AlwaysSample}},
	})
	if err != nil {
		t.Fatalf("comparePolicy: %v", err)
	}
	if result.Current.Observed != 0 || result.Candidate.Observed != 0 {
		t.Fatalf("expected an undecided trace to be excluded from both sides, got %+v", result)
	}
}

// TestComparePolicy_InvalidCandidateReturnsError asserts an invalid
// candidate config (mirroring SetPolicy's own validation) is rejected
// rather than silently building an evaluator that can never match.
func TestComparePolicy_InvalidCandidateReturnsError(t *testing.T) {
	srv := newTestServer(t)

	_, err := srv.comparePolicy(sampling.Config{
		Policies: []sampling.PolicyCfg{
			{Name: "bad", Type: sampling.OTTLCondition, OTTLCondition: &sampling.OTTLConditionCfg{
				ErrorMode: "ignore",
				Span:      []string{`this is not valid OTTL`},
			}},
		},
	})
	if err == nil {
		t.Fatal("expected comparePolicy to reject an invalid candidate config, got nil error")
	}
}
