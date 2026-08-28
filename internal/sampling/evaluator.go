// Evaluator runs a Config's policies using the real tailsamplingprocessor
// policy implementations vendored into internal/upstreamtsp (a copy is
// necessary, not a direct import, because they live under a path segment
// named `internal` in a different module — see
// internal/upstreamtsp/NOTICE.md). A trace is KEPT if any top-level policy
// matches (spec.md ss23-25), the same "OR across policies" semantics the
// real processor uses absent any "drop" policy, which Config cannot
// express (see NewEvaluator's doc comment for what's out of scope).
//
// Re-creating an Evaluator from a new Config and re-running Evaluate over
// the Store is how Policy Update re-evaluation works (spec.md ss24).
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
	"github.com/open-telemetry/opentelemetry-collector-contrib/processor/tailsamplingprocessor/pkg/samplingpolicy"

	"github.com/ucpr/tailsamplingpreviewer/internal/trace"
	upstream "github.com/ucpr/tailsamplingpreviewer/internal/upstreamtsp/sampling"
)

// ottlTelemetrySettings is the no-op telemetry plumbing the vendored
// evaluator constructors and the OTTL parser/evaluator API require but
// which a preview server (no real Collector pipeline running) has no use
// for. Equivalent to componenttest.NewNopTelemetrySettings(), reimplemented
// here so production code doesn't depend on a "test" package.
var ottlTelemetrySettings = component.TelemetrySettings{
	Logger:         zap.NewNop(),
	TracerProvider: nooptrace.NewTracerProvider(),
	MeterProvider:  noopmetric.NewMeterProvider(),
	Resource:       pcommon.NewResource(),
}

// Evaluator is the samplingpolicy.Evaluator-backed policy engine. One
// samplingpolicy.Evaluator is built per top-level policy at construction
// time (mirroring how a real Collector builds its processor once per
// config load), so stateful policies (rate_limiting's token bucket) carry
// state across Evaluate calls exactly as they would in a running
// Collector.
type Evaluator struct {
	cfg        Config
	evaluators []samplingpolicy.Evaluator
}

// NewEvaluator compiles cfg's policies into the vendored evaluators,
// returning an error if any policy is missing required configuration or
// isn't representable by them (see internal/upstreamtsp/NOTICE.md for what
// upstream policy types are excluded and why: composite/drop/not/
// trace_flags/bytes_limiting have no corresponding Config policy type to
// build them from yet).
func NewEvaluator(cfg Config) (*Evaluator, error) {
	evs := make([]samplingpolicy.Evaluator, 0, len(cfg.Policies))
	for _, p := range cfg.Policies {
		ev, err := buildEvaluator(p)
		if err != nil {
			return nil, fmt.Errorf("policy %q: %w", p.Name, err)
		}
		evs = append(evs, ev)
	}
	return &Evaluator{cfg: cfg, evaluators: evs}, nil
}

func (e *Evaluator) Config() Config { return e.cfg }

// Evaluate rehydrates t into a ptrace.Traces batch and runs every compiled
// evaluator against it. There is no injectable clock parameter: every
// vendored evaluator here either doesn't need wall-clock time at all or
// (rate_limiting) reads the real system clock directly — see
// internal/upstreamtsp/sampling/rate_limiting.go, whose
// samplingpolicy.Evaluator.Evaluate signature has no clock parameter
// either, matching how a real Collector's rate limiter can't be driven by
// synthetic timestamps.
func (e *Evaluator) Evaluate(t *trace.Trace) (EvalResult, error) {
	ctx := context.Background()
	td, spanCount, sizeBytes := rehydrateTraceData(t)
	traceData := &samplingpolicy.TraceData{
		SpanCount:       spanCount,
		SizeBytes:       sizeBytes,
		ReceivedBatches: td,
	}

	results := make([]PolicyResult, 0, len(e.cfg.Policies))
	kept := false
	for i, p := range e.cfg.Policies {
		decision, err := e.evaluators[i].Evaluate(ctx, t.TraceID, traceData)
		if err != nil {
			return EvalResult{}, fmt.Errorf("policy %q: %w", p.Name, err)
		}
		//nolint:staticcheck // SA1019: upstream still returns these pending their removal.
		matched := decision == samplingpolicy.Sampled || decision == samplingpolicy.InvertSampled
		results = append(results, PolicyResult{Name: p.Name, Matched: matched})
		if matched {
			kept = true
		}
	}

	decision := trace.DecisionDrop
	if kept {
		decision = trace.DecisionKeep
	}
	return EvalResult{Decision: decision, Policies: results}, nil
}

func buildEvaluator(p PolicyCfg) (samplingpolicy.Evaluator, error) {
	switch p.Type {
	case AlwaysSample:
		return upstream.NewAlwaysSample(ottlTelemetrySettings), nil

	case Latency:
		if p.Latency == nil {
			return nil, errors.New("latency config required")
		}
		return upstream.NewLatency(ottlTelemetrySettings, p.Latency.ThresholdMs, p.Latency.UpperThresholdMs), nil

	case StatusCode:
		if p.StatusCode == nil {
			return nil, errors.New("status_code config required")
		}
		return upstream.NewStatusCodeFilter(ottlTelemetrySettings, p.StatusCode.StatusCodes)

	case NumericAttribute:
		if p.NumericAttribute == nil {
			return nil, errors.New("numeric_attribute config required")
		}
		c := p.NumericAttribute
		minV, maxV := c.MinValue, c.MaxValue
		return upstream.NewNumericAttributeFilter(ottlTelemetrySettings, c.Key, &minV, &maxV, c.InvertMatch), nil

	case StringAttribute:
		if p.StringAttribute == nil {
			return nil, errors.New("string_attribute config required")
		}
		c := p.StringAttribute
		return upstream.NewStringAttributeFilter(ottlTelemetrySettings, c.Key, c.Values, c.EnabledRegexMatching, 0, c.InvertMatch)

	case BooleanAttribute:
		if p.BooleanAttribute == nil {
			return nil, errors.New("boolean_attribute config required")
		}
		c := p.BooleanAttribute
		return upstream.NewBooleanAttributeFilter(ottlTelemetrySettings, c.Key, c.Value, c.InvertMatch), nil

	case SpanCount:
		if p.SpanCount == nil {
			return nil, errors.New("span_count config required")
		}
		c := p.SpanCount
		return upstream.NewSpanCount(ottlTelemetrySettings, c.MinSpans, c.MaxSpans), nil

	case TraceState:
		if p.TraceState == nil {
			return nil, errors.New("trace_state config required")
		}
		c := p.TraceState
		return upstream.NewTraceStateFilter(ottlTelemetrySettings, c.Key, c.Values), nil

	case Probabilistic:
		if p.Probabilistic == nil {
			return nil, errors.New("probabilistic config required")
		}
		c := p.Probabilistic
		return upstream.NewProbabilisticSampler(ottlTelemetrySettings, c.HashSalt, c.SamplingPercentage), nil

	case RateLimiting:
		if p.RateLimiting == nil {
			return nil, errors.New("rate_limiting config required")
		}
		return upstream.NewRateLimiting(ottlTelemetrySettings, p.RateLimiting.SpansPerSecond), nil

	case OTTLCondition:
		return buildOTTL(p.OTTLCondition)

	case And:
		if p.And == nil || len(p.And.SubPolicies) == 0 {
			return nil, errors.New("and requires at least one sub-policy")
		}
		subs := make([]samplingpolicy.Evaluator, 0, len(p.And.SubPolicies))
		for _, sub := range p.And.SubPolicies {
			ev, err := buildEvaluator(subToPolicy(sub))
			if err != nil {
				return nil, fmt.Errorf("sub-policy %q: %w", sub.Name, err)
			}
			subs = append(subs, ev)
		}
		return upstream.NewAnd(ottlTelemetrySettings.Logger, subs), nil

	default:
		return nil, fmt.Errorf("unsupported policy type %q", p.Type)
	}
}

// subToPolicy converts an AndSubPolicyCfg (which cannot itself nest an
// "and") into the equivalent top-level PolicyCfg so buildEvaluator can
// build both from one switch.
func subToPolicy(sub AndSubPolicyCfg) PolicyCfg {
	return PolicyCfg{
		Name:             sub.Name,
		Type:             sub.Type,
		Latency:          sub.Latency,
		NumericAttribute: sub.NumericAttribute,
		Probabilistic:    sub.Probabilistic,
		StatusCode:       sub.StatusCode,
		StringAttribute:  sub.StringAttribute,
		RateLimiting:     sub.RateLimiting,
		BooleanAttribute: sub.BooleanAttribute,
		SpanCount:        sub.SpanCount,
		TraceState:       sub.TraceState,
		OTTLCondition:    sub.OTTLCondition,
	}
}

// buildOTTL validates c the way the real tail_sampling processor's config
// loader would before handing it to the vendored NewOTTLConditionFilter.
// SpanEvent is rejected outright: this preview server doesn't retain span
// event data (internal/trace.Span only tracks EventCount), so there is
// nothing to evaluate spanevent conditions against.
func buildOTTL(c *OTTLConditionCfg) (samplingpolicy.Evaluator, error) {
	if c == nil {
		return nil, errors.New("ottl_condition config required")
	}
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

	return upstream.NewOTTLConditionFilter(ottlTelemetrySettings, c.Span, nil, errorMode)
}

// rehydrateTraceData rebuilds a full ptrace.Traces (one ResourceSpans per
// span) from t, since samplingpolicy.Evaluator only operates on pdata,
// never on this project's flattened trace.Span. internal/trace/assembler.go
// flattens resource and span attributes into t.Attributes under
// "resource."/"attributes."-prefixed keys, which is what's unpacked back
// into real pdata attributes here.
//
// Grouping one span per ResourceSpans instead of batching spans that share
// a resource is safe: every vendored filter either scans every
// ResourceSpans unconditionally (hasSpanWithCondition) or treats "this
// resource or any of its spans matches" independently per ResourceSpans
// (hasResourceOrSpanWithCondition), so how spans are grouped into
// ResourceSpans never changes the Decision.
func rehydrateTraceData(t *trace.Trace) (ptrace.Traces, int64, uint64) {
	td := ptrace.NewTraces()
	for _, s := range t.Spans {
		rs := td.ResourceSpans().AppendEmpty()
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
	}

	marshaler := ptrace.ProtoMarshaler{}
	return td, int64(len(t.Spans)), uint64(marshaler.TracesSize(td))
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
