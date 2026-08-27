package sampling

import (
	"testing"
	"time"

	"go.opentelemetry.io/collector/pdata/pcommon"

	itrace "github.com/ucpr/tailsamplingpreviewer/internal/trace"
)

func newEvaluatorT(t *testing.T, cfg Config) *Evaluator {
	t.Helper()
	ev, err := NewEvaluator(cfg)
	if err != nil {
		t.Fatalf("NewEvaluator: %v", err)
	}
	return ev
}

func mkTrace(statusErr bool, durationMs int64, svc string) *itrace.Trace {
	start := time.Unix(0, 0)
	end := start.Add(time.Duration(durationMs) * time.Millisecond)
	code := itrace.StatusCodeOK
	if statusErr {
		code = itrace.StatusCodeError
	}
	return &itrace.Trace{
		TraceID: pcommon.TraceID{1, 2, 3},
		Spans: []itrace.Span{
			{
				StartTime:   start,
				EndTime:     end,
				StatusCode:  code,
				ServiceName: svc,
				Attributes: map[string]any{
					"service.name": svc,
					"status.code":  map[bool]string{true: "ERROR", false: "OK"}[statusErr],
				},
			},
		},
	}
}

func TestEvaluate_StatusCodeAndLatencyOR(t *testing.T) {
	cfg := Config{
		Policies: []PolicyCfg{
			{Name: "errors", Type: StatusCode, StatusCode: &StatusCodeCfg{StatusCodes: []string{"ERROR"}}},
			{Name: "slow", Type: Latency, Latency: &LatencyCfg{ThresholdMs: 1000}},
		},
	}
	ev := newEvaluatorT(t, cfg)

	errTrace := mkTrace(true, 10, "payment")
	res := ev.Evaluate(errTrace, time.Now())
	if res.Decision != itrace.DecisionKeep {
		t.Fatalf("expected KEEP for error trace, got %v", res.Decision)
	}
	if got := res.MatchedNames(); len(got) != 1 || got[0] != "errors" {
		t.Fatalf("expected only 'errors' matched, got %v", got)
	}

	slowTrace := mkTrace(false, 2000, "catalog")
	res = ev.Evaluate(slowTrace, time.Now())
	if res.Decision != itrace.DecisionKeep {
		t.Fatalf("expected KEEP for slow trace, got %v", res.Decision)
	}

	dropTrace := mkTrace(false, 10, "catalog")
	res = ev.Evaluate(dropTrace, time.Now())
	if res.Decision != itrace.DecisionDrop {
		t.Fatalf("expected DROP, got %v", res.Decision)
	}
}

func TestEvaluate_AndCombinator(t *testing.T) {
	cfg := Config{
		Policies: []PolicyCfg{
			{
				Name: "premium-and-slow",
				Type: And,
				And: &AndCfg{
					SubPolicies: []AndSubPolicyCfg{
						{Name: "premium", Type: StringAttribute, StringAttribute: &StringAttributeCfg{Key: "service.name", Values: []string{"payment"}}},
						{Name: "slow", Type: Latency, Latency: &LatencyCfg{ThresholdMs: 500}},
					},
				},
			},
		},
	}
	ev := newEvaluatorT(t, cfg)

	matches := mkTrace(false, 600, "payment")
	if res := ev.Evaluate(matches, time.Now()); res.Decision != itrace.DecisionKeep {
		t.Fatalf("expected KEEP, got %v", res.Decision)
	}

	onlySlow := mkTrace(false, 600, "catalog")
	if res := ev.Evaluate(onlySlow, time.Now()); res.Decision != itrace.DecisionDrop {
		t.Fatalf("expected DROP when only one AND branch matches, got %v", res.Decision)
	}
}

func TestParseDumpYAML_RoundTrip(t *testing.T) {
	src := []byte(`
processors:
  tail_sampling:
    decision_wait: 30s
    policies:
      - name: errors
        type: status_code
        status_code:
          status_codes: [ERROR]
`)
	cfg, err := ParseYAML(src)
	if err != nil {
		t.Fatalf("ParseYAML: %v", err)
	}
	if cfg.DecisionWait.AsDuration() != 30*time.Second {
		t.Fatalf("expected 30s decision_wait, got %v", cfg.DecisionWait.AsDuration())
	}
	if len(cfg.Policies) != 1 || cfg.Policies[0].Name != "errors" {
		t.Fatalf("unexpected policies: %+v", cfg.Policies)
	}

	out, err := DumpYAML(cfg)
	if err != nil {
		t.Fatalf("DumpYAML: %v", err)
	}
	roundTripped, err := ParseYAML(out)
	if err != nil {
		t.Fatalf("ParseYAML(roundtrip): %v\n%s", err, out)
	}
	if roundTripped.Policies[0].Name != "errors" {
		t.Fatalf("round trip lost policy: %s", out)
	}
}
