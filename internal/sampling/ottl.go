package sampling

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	noopmetric "go.opentelemetry.io/otel/metric/noop"
	nooptrace "go.opentelemetry.io/otel/trace/noop"
	"go.uber.org/zap"

	"github.com/open-telemetry/opentelemetry-collector-contrib/pkg/ottl"
	"github.com/open-telemetry/opentelemetry-collector-contrib/pkg/ottl/contexts/ottlspan"
	"github.com/open-telemetry/opentelemetry-collector-contrib/pkg/ottl/ottlfuncs"

	"github.com/ucpr/tailsamplingpreviewer/internal/trace"
)

// ottlTelemetrySettings is the no-op telemetry plumbing the OTTL
// parser/evaluator API requires but which a preview server (no real
// Collector pipeline running) has no use for. Equivalent to
// componenttest.NewNopTelemetrySettings(), reimplemented here so
// production code doesn't depend on a "test" package.
var ottlTelemetrySettings = component.TelemetrySettings{
	Logger:         zap.NewNop(),
	TracerProvider: nooptrace.NewTracerProvider(),
	MeterProvider:  noopmetric.NewMeterProvider(),
	Resource:       pcommon.NewResource(),
}

// compileOTTLSpanCondition parses an OTTLConditionCfg's Span expressions
// into a ready-to-evaluate condition sequence. Matches the real
// tailsamplingprocessor's "ottl_condition" semantics: any expression true
// -> match (ConditionSequence ORs by default). SpanEvent is rejected: this
// preview server doesn't retain span event data (internal/trace.Span only
// tracks EventCount, see its doc comment), so there is nothing to evaluate
// spanevent conditions against.
func compileOTTLSpanCondition(c *OTTLConditionCfg) (*ottl.ConditionSequence[*ottlspan.TransformContext], error) {
	if len(c.SpanEvent) > 0 {
		return nil, errors.New("ottl_condition.spanevent is not supported by the preview server (span events are not retained); use span conditions only")
	}
	if len(c.Span) == 0 {
		return nil, errors.New("ottl_condition requires at least one span condition")
	}

	errorMode := ottl.PropagateError
	if c.ErrorMode != "" {
		if err := errorMode.UnmarshalText([]byte(c.ErrorMode)); err != nil {
			return nil, fmt.Errorf("ottl_condition.error_mode: %w", err)
		}
	}

	parser, err := ottlspan.NewParser(ottlfuncs.StandardFuncs[*ottlspan.TransformContext](), ottlTelemetrySettings)
	if err != nil {
		return nil, fmt.Errorf("ottl_condition: build parser: %w", err)
	}
	conditions, err := parser.ParseConditions(c.Span)
	if err != nil {
		return nil, fmt.Errorf("ottl_condition: %w", err)
	}
	seq := ottlspan.NewConditionSequence(conditions, ottlTelemetrySettings, ottlspan.WithConditionSequenceErrorMode(errorMode))
	return &seq, nil
}

// matchOTTLCondition reports whether any span in t satisfies seq, mirroring
// anySpanAttr's "OR across every span in the trace" pattern.
func matchOTTLCondition(seq *ottl.ConditionSequence[*ottlspan.TransformContext], t *trace.Trace) bool {
	if seq == nil {
		return false
	}
	ctx := context.Background()
	for _, s := range t.Spans {
		tCtx := rehydrateSpanContext(s)
		ok, err := seq.Eval(ctx, tCtx)
		tCtx.Close()
		if err == nil && ok {
			return true
		}
	}
	return false
}

// rehydrateSpanContext reconstructs a minimal ptrace.Span/pcommon.Resource
// pair from a flattened trace.Span (internal/trace/types.go) so it can be
// evaluated against an OTTL condition. The preview server never keeps the
// original pdata objects around (spans are flattened on ingest, see
// internal/trace/assembler.go); Attributes already retains every
// resource/span attribute with its original type via pcommon.Value.AsRaw,
// so nothing is lost by rebuilding a pdata span here instead of retaining
// one per stored span.
func rehydrateSpanContext(s trace.Span) *ottlspan.TransformContext {
	rs := ptrace.NewResourceSpans()
	ss := rs.ScopeSpans().AppendEmpty()
	span := ss.Spans().AppendEmpty()

	span.SetTraceID(s.TraceID)
	span.SetSpanID(s.SpanID)
	span.SetParentSpanID(s.ParentSpanID)
	span.SetName(s.Name)
	span.SetKind(ptrace.SpanKind(s.Kind))
	span.SetStartTimestamp(pcommon.NewTimestampFromTime(s.StartTime))
	span.SetEndTimestamp(pcommon.NewTimestampFromTime(s.EndTime))
	span.Status().SetCode(toPtraceStatusCode(s.StatusCode))
	span.Status().SetMessage(s.StatusMessage)
	span.TraceState().FromRaw(s.TraceStateRaw)

	for k, v := range s.Attributes {
		switch {
		case strings.HasPrefix(k, "attributes."):
			_ = span.Attributes().PutEmpty(strings.TrimPrefix(k, "attributes.")).FromRaw(v)
		case strings.HasPrefix(k, "resource."):
			_ = rs.Resource().Attributes().PutEmpty(strings.TrimPrefix(k, "resource.")).FromRaw(v)
		}
	}

	return ottlspan.NewTransformContextPtr(rs, ss, span)
}

func toPtraceStatusCode(c trace.StatusCode) ptrace.StatusCode {
	switch c {
	case trace.StatusCodeOK:
		return ptrace.StatusCodeOk
	case trace.StatusCodeError:
		return ptrace.StatusCodeError
	default:
		return ptrace.StatusCodeUnset
	}
}
