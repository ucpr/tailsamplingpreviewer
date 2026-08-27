package trace

import (
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

// Assemble flattens an ExportTraceServiceRequest payload into Spans grouped
// by trace ID (spec.md ss20). It performs no aggregation across calls; the
// Store is responsible for merging these into long-lived Trace aggregates.
func Assemble(td ptrace.Traces) map[pcommon.TraceID][]Span {
	out := make(map[pcommon.TraceID][]Span)

	rss := td.ResourceSpans()
	for i := 0; i < rss.Len(); i++ {
		rs := rss.At(i)
		resAttrs := rs.Resource().Attributes()
		serviceName, _ := resAttrs.Get("service.name")

		sss := rs.ScopeSpans()
		for j := 0; j < sss.Len(); j++ {
			spans := sss.At(j).Spans()
			for k := 0; k < spans.Len(); k++ {
				s := spans.At(k)
				span := toSpan(s, resAttrs, serviceName.AsString())
				out[span.TraceID] = append(out[span.TraceID], span)
			}
		}
	}
	return out
}

func toSpan(s ptrace.Span, resAttrs pcommon.Map, serviceName string) Span {
	attrs := make(map[string]any, resAttrs.Len()+s.Attributes().Len()+8)

	resAttrs.Range(func(k string, v pcommon.Value) bool {
		attrs["resource."+k] = v.AsRaw()
		return true
	})
	s.Attributes().Range(func(k string, v pcommon.Value) bool {
		attrs["attributes."+k] = v.AsRaw()
		return true
	})

	statusCode := fromPtraceStatusCode(s.Status().Code())

	attrs["name"] = s.Name()
	attrs["kind"] = s.Kind().String()
	attrs["service.name"] = serviceName
	attrs["status.code"] = statusCodeLabel(statusCode)
	attrs["duration_ms"] = float64(s.EndTimestamp().AsTime().Sub(s.StartTimestamp().AsTime()).Milliseconds())

	return Span{
		TraceID:       s.TraceID(),
		SpanID:        s.SpanID(),
		ParentSpanID:  s.ParentSpanID(),
		Name:          s.Name(),
		Kind:          int32(s.Kind()),
		StartTime:     s.StartTimestamp().AsTime(),
		EndTime:       s.EndTimestamp().AsTime(),
		StatusCode:    statusCode,
		StatusMessage: s.Status().Message(),
		TraceStateRaw: s.TraceState().AsRaw(),
		Attributes:    attrs,
		ServiceName:   serviceName,
		EventCount:    s.Events().Len(),
		LinkCount:     s.Links().Len(),
	}
}

func fromPtraceStatusCode(c ptrace.StatusCode) StatusCode {
	switch c {
	case ptrace.StatusCodeOk:
		return StatusCodeOK
	case ptrace.StatusCodeError:
		return StatusCodeError
	default:
		return StatusCodeUnset
	}
}

func statusCodeLabel(c StatusCode) string {
	switch c {
	case StatusCodeOK:
		return "OK"
	case StatusCodeError:
		return "ERROR"
	default:
		return "UNSET"
	}
}
