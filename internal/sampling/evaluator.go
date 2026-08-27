package sampling

import (
	"fmt"
	"hash/fnv"
	"regexp"
	"sync"
	"time"

	"github.com/open-telemetry/opentelemetry-collector-contrib/pkg/ottl"
	"github.com/open-telemetry/opentelemetry-collector-contrib/pkg/ottl/contexts/ottlspan"

	"github.com/ucpr/tailsamplingpreviewer/internal/trace"
)

// Evaluator evaluates a Config's policies against Traces. A trace is KEPT
// if any top-level policy matches (spec.md ss23-25). Re-creating an
// Evaluator from a new Config and re-running Evaluate over the Store is
// how Policy Update re-evaluation works (spec.md ss24).
type Evaluator struct {
	cfg Config

	mu          sync.Mutex
	rateWindows map[string]*rateWindow
	regexCache  map[string]*regexp.Regexp

	// ottlConditions holds every ottl_condition policy's compiled
	// expression, keyed by the pointer identity of its OTTLConditionCfg
	// (stable for the lifetime of this Evaluator: cfg is never mutated
	// after construction). Compiling once here, like regexCache does for
	// string_attribute regexes, avoids re-parsing OTTL on every Evaluate.
	ottlConditions map[*OTTLConditionCfg]*ottl.ConditionSequence[*ottlspan.TransformContext]
}

type rateWindow struct {
	second int64
	count  int64
}

// NewEvaluator compiles cfg's policies (including every ottl_condition's
// OTTL expressions) and returns an error if any of them are invalid,
// instead of failing later or panicking during Evaluate.
func NewEvaluator(cfg Config) (*Evaluator, error) {
	e := &Evaluator{
		cfg:            cfg,
		rateWindows:    make(map[string]*rateWindow),
		regexCache:     make(map[string]*regexp.Regexp),
		ottlConditions: make(map[*OTTLConditionCfg]*ottl.ConditionSequence[*ottlspan.TransformContext]),
	}
	for _, p := range cfg.Policies {
		if err := e.compileOTTL(p.OTTLCondition); err != nil {
			return nil, fmt.Errorf("policy %q: %w", p.Name, err)
		}
		if p.Type == And && p.And != nil {
			for _, sub := range p.And.SubPolicies {
				if err := e.compileOTTL(sub.OTTLCondition); err != nil {
					return nil, fmt.Errorf("policy %q sub-policy %q: %w", p.Name, sub.Name, err)
				}
			}
		}
	}
	return e, nil
}

func (e *Evaluator) compileOTTL(c *OTTLConditionCfg) error {
	if c == nil {
		return nil
	}
	seq, err := compileOTTLSpanCondition(c)
	if err != nil {
		return err
	}
	e.ottlConditions[c] = seq
	return nil
}

func (e *Evaluator) Config() Config { return e.cfg }

// Evaluate runs every configured policy against t and returns the KEEP/DROP
// decision plus per-policy match results.
func (e *Evaluator) Evaluate(t *trace.Trace, now time.Time) EvalResult {
	results := make([]PolicyResult, 0, len(e.cfg.Policies))
	kept := false

	for _, p := range e.cfg.Policies {
		matched := e.matchPolicy(p, t, now)
		results = append(results, PolicyResult{Name: p.Name, Matched: matched})
		if matched {
			kept = true
		}
	}

	decision := trace.DecisionDrop
	if kept {
		decision = trace.DecisionKeep
	}
	return EvalResult{Decision: decision, Policies: results}
}

func (e *Evaluator) matchPolicy(p PolicyCfg, t *trace.Trace, now time.Time) bool {
	switch p.Type {
	case AlwaysSample:
		return true
	case Latency:
		return matchLatency(p.Latency, t)
	case StatusCode:
		return matchStatusCode(p.StatusCode, t)
	case NumericAttribute:
		return matchNumericAttribute(p.NumericAttribute, t)
	case StringAttribute:
		return e.matchStringAttribute(p.StringAttribute, t)
	case BooleanAttribute:
		return matchBooleanAttribute(p.BooleanAttribute, t)
	case SpanCount:
		return matchSpanCount(p.SpanCount, t)
	case TraceState:
		return matchTraceState(p.TraceState, t)
	case OTTLCondition:
		return matchOTTLCondition(e.ottlConditions[p.OTTLCondition], t)
	case Probabilistic:
		return matchProbabilistic(p.Probabilistic, p.Name, t)
	case RateLimiting:
		return e.matchRateLimiting(p.RateLimiting, p.Name, t, now)
	case And:
		return e.matchAnd(p.And, t, now)
	default:
		return false
	}
}

func matchLatency(c *LatencyCfg, t *trace.Trace) bool {
	if c == nil {
		return false
	}
	ms := t.Duration().Milliseconds()
	if ms < c.ThresholdMs {
		return false
	}
	if c.UpperThresholdMs > 0 && ms > c.UpperThresholdMs {
		return false
	}
	return true
}

func matchStatusCode(c *StatusCodeCfg, t *trace.Trace) bool {
	if c == nil {
		return false
	}
	want := make(map[string]bool, len(c.StatusCodes))
	for _, code := range c.StatusCodes {
		want[code] = true
	}
	for _, s := range t.Spans {
		if want[statusLabel(s.StatusCode)] {
			return true
		}
	}
	return false
}

func statusLabel(c trace.StatusCode) string {
	switch c {
	case trace.StatusCodeOK:
		return "OK"
	case trace.StatusCodeError:
		return "ERROR"
	default:
		return "UNSET"
	}
}

func matchNumericAttribute(c *NumericAttributeCfg, t *trace.Trace) bool {
	if c == nil {
		return false
	}
	matched := anySpanAttr(t, c.Key, func(v any) bool {
		n, ok := asFloat64(v)
		if !ok {
			return false
		}
		return int64(n) >= c.MinValue && int64(n) <= c.MaxValue
	})
	if c.InvertMatch {
		return !matched
	}
	return matched
}

func (e *Evaluator) matchStringAttribute(c *StringAttributeCfg, t *trace.Trace) bool {
	if c == nil {
		return false
	}
	matched := anySpanAttr(t, c.Key, func(v any) bool {
		s, ok := v.(string)
		if !ok {
			return false
		}
		if c.EnabledRegexMatching {
			for _, pattern := range c.Values {
				if e.regexMatch(pattern, s) {
					return true
				}
			}
			return false
		}
		for _, want := range c.Values {
			if s == want {
				return true
			}
		}
		return false
	})
	if c.InvertMatch {
		return !matched
	}
	return matched
}

func (e *Evaluator) regexMatch(pattern, s string) bool {
	e.mu.Lock()
	re, ok := e.regexCache[pattern]
	if !ok {
		re = regexp.MustCompile(pattern)
		e.regexCache[pattern] = re
	}
	e.mu.Unlock()
	return re.MatchString(s)
}

func matchBooleanAttribute(c *BooleanAttributeCfg, t *trace.Trace) bool {
	if c == nil {
		return false
	}
	matched := anySpanAttr(t, c.Key, func(v any) bool {
		b, ok := v.(bool)
		return ok && b == c.Value
	})
	if c.InvertMatch {
		return !matched
	}
	return matched
}

func matchSpanCount(c *SpanCountCfg, t *trace.Trace) bool {
	if c == nil {
		return false
	}
	n := int32(len(t.Spans))
	if n < c.MinSpans {
		return false
	}
	if c.MaxSpans > 0 && n > c.MaxSpans {
		return false
	}
	return true
}

func matchTraceState(c *TraceStateCfg, t *trace.Trace) bool {
	if c == nil {
		return false
	}
	for _, s := range t.Spans {
		v, ok := parseTraceState(s.TraceStateRaw)[c.Key]
		if !ok {
			continue
		}
		for _, want := range c.Values {
			if v == want {
				return true
			}
		}
	}
	return false
}

// parseTraceState parses a W3C tracestate header ("k1=v1,k2=v2") into a map.
func parseTraceState(raw string) map[string]string {
	out := map[string]string{}
	start := 0
	for i := 0; i <= len(raw); i++ {
		if i == len(raw) || raw[i] == ',' {
			member := raw[start:i]
			if eq := indexByte(member, '='); eq >= 0 {
				out[member[:eq]] = member[eq+1:]
			}
			start = i + 1
		}
	}
	return out
}

func indexByte(s string, b byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == b {
			return i
		}
	}
	return -1
}

func matchProbabilistic(c *ProbabilisticCfg, policyName string, t *trace.Trace) bool {
	if c == nil {
		return false
	}
	h := fnv.New32a()
	_, _ = h.Write(t.TraceID[:])
	_, _ = h.Write([]byte(c.HashSalt))
	_, _ = h.Write([]byte(policyName))
	bucket := float64(h.Sum32()%10000) / 100.0 // [0, 100)
	return bucket < c.SamplingPercentage
}

// matchRateLimiting is a best-effort approximation: it caps the number of
// *evaluations* accepted per wall-clock second per policy. Because Preview
// re-evaluation can replay a whole ring buffer instantaneously (spec.md
// ss24), this is necessarily approximate rather than a faithful replay of
// the Collector's real-time rate limiter.
func (e *Evaluator) matchRateLimiting(c *RateLimitingCfg, policyName string, t *trace.Trace, now time.Time) bool {
	if c == nil || c.SpansPerSecond <= 0 {
		return false
	}
	spans := int64(len(t.Spans))

	e.mu.Lock()
	defer e.mu.Unlock()
	w, ok := e.rateWindows[policyName]
	if !ok {
		w = &rateWindow{}
		e.rateWindows[policyName] = w
	}
	sec := now.Unix()
	if w.second != sec {
		w.second = sec
		w.count = 0
	}
	if w.count+spans > c.SpansPerSecond {
		return false
	}
	w.count += spans
	return true
}

func (e *Evaluator) matchAnd(c *AndCfg, t *trace.Trace, now time.Time) bool {
	if c == nil || len(c.SubPolicies) == 0 {
		return false
	}
	for _, sub := range c.SubPolicies {
		if !e.matchPolicy(subToPolicy(sub), t, now) {
			return false
		}
	}
	return true
}

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

func anySpanAttr(t *trace.Trace, key string, pred func(any) bool) bool {
	for _, s := range t.Spans {
		if v, ok := s.Attributes[key]; ok && pred(v) {
			return true
		}
	}
	return false
}

func asFloat64(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case int64:
		return float64(n), true
	case int:
		return float64(n), true
	default:
		return 0, false
	}
}
