package server

import (
	"testing"
	"time"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.uber.org/zap"

	"github.com/ucpr/tailsamplingpreviewer/internal/sampling"
	itrace "github.com/ucpr/tailsamplingpreviewer/internal/trace"
)

// TestSetPolicy_ReevaluatesAlreadyDecidedTraces covers spec.md ss24: editing
// the policy must immediately re-evaluate everything already retained in
// the ring buffer, including traces that already reached a final KEEP/DROP
// decision, and refresh aggregate statistics to match.
func TestSetPolicy_ReevaluatesAlreadyDecidedTraces(t *testing.T) {
	srv := New(zap.NewNop(), Config{
		InitialPolicy: sampling.Config{
			DecisionWait: sampling.Duration(time.Second),
			Policies: []sampling.PolicyCfg{
				{Name: "errors", Type: sampling.StatusCode, StatusCode: &sampling.StatusCodeCfg{StatusCodes: []string{"ERROR"}}},
			},
		},
	})

	id := pcommon.TraceID{7}
	now := time.Now()
	srv.store.Ingest(now, map[pcommon.TraceID][]itrace.Span{
		id: {{
			Name:        "handle",
			ServiceName: "catalog",
			StartTime:   now,
			EndTime:     now.Add(50 * time.Millisecond),
			StatusCode:  itrace.StatusCodeOK,
			Attributes:  map[string]any{"status.code": "OK", "duration_ms": float64(50)},
		}},
	})

	// Force it through the normal decision path first: not an error trace,
	// so it should be decided DROP under the initial policy.
	srv.decideDue(now.Add(2 * time.Second))
	before, ok := srv.store.Get(id)
	if !ok || before.State != itrace.StateDecided || before.Decision != itrace.DecisionDrop {
		t.Fatalf("expected trace to be DECIDED/DROP before policy change, got ok=%v %+v", ok, before)
	}
	statsBefore := srv.stats.Snapshot()
	if statsBefore.Observed != 1 || statsBefore.Drop != 1 {
		t.Fatalf("unexpected statistics before policy change: %+v", statsBefore)
	}

	// Now widen the policy so the very same trace should flip to KEEP.
	srv.SetPolicy(sampling.Config{
		DecisionWait: sampling.Duration(time.Second),
		Policies: []sampling.PolicyCfg{
			{Name: "fast-or-slow", Type: sampling.AlwaysSample},
		},
	})

	after, ok := srv.store.Get(id)
	if !ok || after.State != itrace.StateDecided || after.Decision != itrace.DecisionKeep {
		t.Fatalf("expected trace to flip to DECIDED/KEEP after policy change, got ok=%v %+v", ok, after)
	}
	if got := after.MatchedPolicies; len(got) != 1 || got[0] != "fast-or-slow" {
		t.Fatalf("expected matched_policies to reflect the new policy, got %v", got)
	}

	statsAfter := srv.stats.Snapshot()
	if statsAfter.Observed != 1 || statsAfter.Keep != 1 || statsAfter.Drop != 0 {
		t.Fatalf("expected statistics to be recomputed from scratch after policy change, got %+v", statsAfter)
	}
}
