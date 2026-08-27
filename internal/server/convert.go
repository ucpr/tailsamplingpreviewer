package server

import (
	itrace "github.com/ucpr/tailsamplingpreviewer/internal/trace"
)

func stateLabel(s itrace.State) string { return s.String() }

func decisionLabel(d itrace.Decision) string { return d.String() }

func toSummary(t *itrace.Trace) TraceSummary {
	root, _ := t.RootSpan()
	return TraceSummary{
		TraceID:         t.TraceID.String(),
		ServiceName:     t.RootServiceName(),
		RootSpanName:    root.Name,
		DurationMs:      t.Duration().Milliseconds(),
		HasError:        t.HasError(),
		SpanCount:       len(t.Spans),
		State:           stateLabel(t.State),
		Decision:        decisionLabel(t.Decision),
		MatchedPolicies: append([]string(nil), t.MatchedPolicies...),
		FirstSeen:       t.FirstSeen,
		LastSeen:        t.LastSeen,
	}
}

func toDetail(t *itrace.Trace) TraceDetail {
	spans := make([]SpanSummary, 0, len(t.Spans))
	for _, s := range t.Spans {
		spans = append(spans, SpanSummary{
			SpanID:       s.SpanID.String(),
			ParentSpanID: s.ParentSpanID.String(),
			Name:         s.Name,
			ServiceName:  s.ServiceName,
			StartTime:    s.StartTime,
			EndTime:      s.EndTime,
			DurationMs:   s.Duration().Milliseconds(),
			StatusCode:   statusLabel(s.StatusCode),
			Attributes:   s.Attributes,
		})
	}
	return TraceDetail{TraceSummary: toSummary(t), Spans: spans}
}

func statusLabel(c itrace.StatusCode) string {
	switch c {
	case itrace.StatusCodeOK:
		return "OK"
	case itrace.StatusCodeError:
		return "ERROR"
	default:
		return "UNSET"
	}
}
