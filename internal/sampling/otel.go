// Package sampling implements the Tail Sampling Policy Engine that runs
// inside the Preview Server (spec.md ss23). The Collector's exporter never
// evaluates policies; only this package does, which is what lets the same
// trace set be re-evaluated against arbitrary policies on demand.
//
// The YAML schema in this file is intentionally shaped to match the field
// names of the upstream `tailsamplingprocessor` (see
// open-telemetry/opentelemetry-collector-contrib) so that a `policies:`
// block can be lifted in and out of a real Collector `tail_sampling`
// processor config with minimal editing (spec.md ss10, ss76). The MVP
// supports the subset of policy types needed to express the boolean
// predicates shown in the Policy Builder mock-up (spec.md ss25): always
// sample, latency, status code, numeric/string/boolean attribute,
// probabilistic, rate limiting, span count, trace state and an "and"
// combinator. Unsupported real-processor policy types (composite, drop,
// not, ottl_condition, ...) are out of scope for this MVP.
//
// Every struct also carries JSON tags mirroring the YAML ones so the same
// types serve the Policy Builder REST API (spec.md ss25) without a second
// schema to keep in sync.
package sampling

import (
	"fmt"
	"time"

	"gopkg.in/yaml.v3"
)

// PolicyType is the discriminator for a policy's matching logic, matching
// the `type:` values accepted by the upstream tail_sampling processor.
type PolicyType string

const (
	AlwaysSample     PolicyType = "always_sample"
	Latency          PolicyType = "latency"
	NumericAttribute PolicyType = "numeric_attribute"
	Probabilistic    PolicyType = "probabilistic"
	StatusCode       PolicyType = "status_code"
	StringAttribute  PolicyType = "string_attribute"
	RateLimiting     PolicyType = "rate_limiting"
	BooleanAttribute PolicyType = "boolean_attribute"
	SpanCount        PolicyType = "span_count"
	TraceState       PolicyType = "trace_state"
	And              PolicyType = "and"
)

// Config is the root of a `tail_sampling:` processor configuration.
type Config struct {
	DecisionWait Duration    `yaml:"decision_wait" json:"decision_wait"`
	NumTraces    int         `yaml:"num_traces,omitempty" json:"num_traces,omitempty"`
	Policies     []PolicyCfg `yaml:"policies" json:"policies"`
}

// Duration marshals to/from the "30s"-style strings the Collector config
// format uses, instead of the default integer-nanoseconds encoding.
type Duration time.Duration

func (d Duration) AsDuration() time.Duration { return time.Duration(d) }

func (d *Duration) UnmarshalYAML(node *yaml.Node) error {
	var s string
	if err := node.Decode(&s); err != nil {
		return err
	}
	parsed, err := time.ParseDuration(s)
	if err != nil {
		return fmt.Errorf("invalid duration %q: %w", s, err)
	}
	*d = Duration(parsed)
	return nil
}

func (d Duration) MarshalYAML() (any, error) {
	return time.Duration(d).String(), nil
}

func (d Duration) MarshalJSON() ([]byte, error) {
	return fmt.Appendf(nil, "%q", time.Duration(d).String()), nil
}

func (d *Duration) UnmarshalJSON(data []byte) error {
	var s string
	if err := jsonUnquote(data, &s); err != nil {
		return err
	}
	parsed, err := time.ParseDuration(s)
	if err != nil {
		return fmt.Errorf("invalid duration %q: %w", s, err)
	}
	*d = Duration(parsed)
	return nil
}

// jsonUnquote avoids importing encoding/json just for a string literal.
func jsonUnquote(data []byte, out *string) error {
	if len(data) < 2 || data[0] != '"' || data[len(data)-1] != '"' {
		return fmt.Errorf("invalid duration json %q", data)
	}
	*out = string(data[1 : len(data)-1])
	return nil
}

// PolicyCfg is one named top-level policy. A trace is KEPT if any
// top-level policy matches (spec.md ss23-25).
type PolicyCfg struct {
	Name string     `yaml:"name" json:"name"`
	Type PolicyType `yaml:"type" json:"type"`

	Latency          *LatencyCfg          `yaml:"latency,omitempty" json:"latency,omitempty"`
	NumericAttribute *NumericAttributeCfg `yaml:"numeric_attribute,omitempty" json:"numeric_attribute,omitempty"`
	Probabilistic    *ProbabilisticCfg    `yaml:"probabilistic,omitempty" json:"probabilistic,omitempty"`
	StatusCode       *StatusCodeCfg       `yaml:"status_code,omitempty" json:"status_code,omitempty"`
	StringAttribute  *StringAttributeCfg  `yaml:"string_attribute,omitempty" json:"string_attribute,omitempty"`
	RateLimiting     *RateLimitingCfg     `yaml:"rate_limiting,omitempty" json:"rate_limiting,omitempty"`
	BooleanAttribute *BooleanAttributeCfg `yaml:"boolean_attribute,omitempty" json:"boolean_attribute,omitempty"`
	SpanCount        *SpanCountCfg        `yaml:"span_count,omitempty" json:"span_count,omitempty"`
	TraceState       *TraceStateCfg       `yaml:"trace_state,omitempty" json:"trace_state,omitempty"`
	And              *AndCfg              `yaml:"and,omitempty" json:"and,omitempty"`
}

type LatencyCfg struct {
	ThresholdMs      int64 `yaml:"threshold_ms" json:"threshold_ms"`
	UpperThresholdMs int64 `yaml:"upper_threshold_ms,omitempty" json:"upper_threshold_ms,omitempty"`
}

type NumericAttributeCfg struct {
	Key         string `yaml:"key" json:"key"`
	MinValue    int64  `yaml:"min_value" json:"min_value"`
	MaxValue    int64  `yaml:"max_value" json:"max_value"`
	InvertMatch bool   `yaml:"invert_match,omitempty" json:"invert_match,omitempty"`
}

type ProbabilisticCfg struct {
	HashSalt           string  `yaml:"hash_salt,omitempty" json:"hash_salt,omitempty"`
	SamplingPercentage float64 `yaml:"sampling_percentage" json:"sampling_percentage"`
}

type StatusCodeCfg struct {
	StatusCodes []string `yaml:"status_codes" json:"status_codes"`
}

type StringAttributeCfg struct {
	Key                  string   `yaml:"key" json:"key"`
	Values               []string `yaml:"values" json:"values"`
	EnabledRegexMatching bool     `yaml:"enabled_regex_matching,omitempty" json:"enabled_regex_matching,omitempty"`
	InvertMatch          bool     `yaml:"invert_match,omitempty" json:"invert_match,omitempty"`
}

type RateLimitingCfg struct {
	SpansPerSecond int64 `yaml:"spans_per_second" json:"spans_per_second"`
}

type BooleanAttributeCfg struct {
	Key         string `yaml:"key" json:"key"`
	Value       bool   `yaml:"value" json:"value"`
	InvertMatch bool   `yaml:"invert_match,omitempty" json:"invert_match,omitempty"`
}

type SpanCountCfg struct {
	MinSpans int32 `yaml:"min_spans" json:"min_spans"`
	MaxSpans int32 `yaml:"max_spans,omitempty" json:"max_spans,omitempty"`
}

type TraceStateCfg struct {
	Key    string   `yaml:"key" json:"key"`
	Values []string `yaml:"values" json:"values"`
}

// AndCfg matches on every sub-policy (AND semantics), each of which reuses
// the same shared field set as a top-level PolicyCfg minus nested "and".
type AndCfg struct {
	SubPolicies []AndSubPolicyCfg `yaml:"and_sub_policy" json:"and_sub_policy"`
}

type AndSubPolicyCfg struct {
	Name string     `yaml:"name" json:"name"`
	Type PolicyType `yaml:"type" json:"type"`

	Latency          *LatencyCfg          `yaml:"latency,omitempty" json:"latency,omitempty"`
	NumericAttribute *NumericAttributeCfg `yaml:"numeric_attribute,omitempty" json:"numeric_attribute,omitempty"`
	Probabilistic    *ProbabilisticCfg    `yaml:"probabilistic,omitempty" json:"probabilistic,omitempty"`
	StatusCode       *StatusCodeCfg       `yaml:"status_code,omitempty" json:"status_code,omitempty"`
	StringAttribute  *StringAttributeCfg  `yaml:"string_attribute,omitempty" json:"string_attribute,omitempty"`
	RateLimiting     *RateLimitingCfg     `yaml:"rate_limiting,omitempty" json:"rate_limiting,omitempty"`
	BooleanAttribute *BooleanAttributeCfg `yaml:"boolean_attribute,omitempty" json:"boolean_attribute,omitempty"`
	SpanCount        *SpanCountCfg        `yaml:"span_count,omitempty" json:"span_count,omitempty"`
	TraceState       *TraceStateCfg       `yaml:"trace_state,omitempty" json:"trace_state,omitempty"`
}

// ParseYAML decodes a `tail_sampling:` processor configuration.
//
// Two shapes are accepted so that a whole Collector config file can be
// pasted in directly (spec.md ss76 "YAML import"):
//
//	decision_wait: 30s
//	policies: [...]
//
//	processors:
//	  tail_sampling:
//	    decision_wait: 30s
//	    policies: [...]
func ParseYAML(data []byte) (Config, error) {
	var direct Config
	if err := yaml.Unmarshal(data, &direct); err == nil && len(direct.Policies) > 0 {
		return direct, nil
	}

	var wrapped struct {
		Processors struct {
			TailSampling Config `yaml:"tail_sampling"`
		} `yaml:"processors"`
	}
	if err := yaml.Unmarshal(data, &wrapped); err != nil {
		return Config{}, fmt.Errorf("parse tail_sampling yaml: %w", err)
	}
	if len(wrapped.Processors.TailSampling.Policies) == 0 {
		return Config{}, fmt.Errorf("parse tail_sampling yaml: no policies found")
	}
	return wrapped.Processors.TailSampling, nil
}

// DumpYAML renders a Config back to the `processors.tail_sampling` shape a
// Collector config expects (spec.md ss76 "YAML export").
func DumpYAML(cfg Config) ([]byte, error) {
	wrapped := struct {
		Processors struct {
			TailSampling Config `yaml:"tail_sampling"`
		} `yaml:"processors"`
	}{}
	wrapped.Processors.TailSampling = cfg
	return yaml.Marshal(wrapped)
}
