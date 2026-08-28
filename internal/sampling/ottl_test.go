package sampling

import (
	"strings"
	"testing"
	"time"

	"go.opentelemetry.io/collector/pdata/pcommon"

	itrace "github.com/ucpr/tailsamplingpreviewer/internal/trace"
)

func TestEvaluate_OTTLCondition_SpanAttribute(t *testing.T) {
	cfg := Config{
		Policies: []PolicyCfg{
			{
				Name: "slow-checkout",
				Type: OTTLCondition,
				OTTLCondition: &OTTLConditionCfg{
					ErrorMode: "ignore",
					Span:      []string{`resource.attributes["service.name"] == "checkout"`},
				},
			},
		},
	}
	ev := newEvaluatorT(t, cfg)

	matching := &itrace.Trace{
		TraceID: pcommon.TraceID{1},
		Spans: []itrace.Span{
			{
				Name:        "handle",
				ServiceName: "checkout",
				StartTime:   time.Unix(0, 0),
				EndTime:     time.Unix(0, 0).Add(10 * time.Millisecond),
				Attributes: map[string]any{
					"resource.service.name": "checkout",
				},
			},
		},
	}
	if res := evaluateT(t, ev, matching); res.Decision != itrace.DecisionKeep {
		t.Fatalf("expected KEEP, got %v (%+v)", res.Decision, res)
	}

	nonMatching := &itrace.Trace{
		TraceID: pcommon.TraceID{2},
		Spans: []itrace.Span{
			{
				Name:        "handle",
				ServiceName: "catalog",
				StartTime:   time.Unix(0, 0),
				EndTime:     time.Unix(0, 0).Add(10 * time.Millisecond),
				Attributes: map[string]any{
					"resource.service.name": "catalog",
				},
			},
		},
	}
	if res := evaluateT(t, ev, nonMatching); res.Decision != itrace.DecisionDrop {
		t.Fatalf("expected DROP, got %v (%+v)", res.Decision, res)
	}
}

func TestNewEvaluator_OTTLCondition_InvalidSyntax(t *testing.T) {
	cfg := Config{
		Policies: []PolicyCfg{
			{
				Name: "bad",
				Type: OTTLCondition,
				OTTLCondition: &OTTLConditionCfg{
					ErrorMode: "ignore",
					Span:      []string{`this is not valid OTTL`},
				},
			},
		},
	}
	_, err := NewEvaluator(cfg)
	if err == nil {
		t.Fatal("expected NewEvaluator to reject invalid OTTL syntax, got nil error")
	}
}

func TestNewEvaluator_OTTLCondition_SpanEventRejected(t *testing.T) {
	cfg := Config{
		Policies: []PolicyCfg{
			{
				Name: "uses-spanevent",
				Type: OTTLCondition,
				OTTLCondition: &OTTLConditionCfg{
					ErrorMode: "ignore",
					SpanEvent: []string{`name == "some_event"`},
				},
			},
		},
	}
	_, err := NewEvaluator(cfg)
	if err == nil {
		t.Fatal("expected NewEvaluator to reject spanevent conditions, got nil error")
	}
	if !strings.Contains(err.Error(), "spanevent") {
		t.Fatalf("expected error to mention spanevent, got: %v", err)
	}
}
