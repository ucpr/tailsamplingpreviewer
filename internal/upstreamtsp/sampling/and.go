// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

// Simplified vendor copy: upstream's v0.159.0 and.go implements Evaluate in
// terms of a ThresholdEvaluator.EvaluateWithThreshold that also propagates
// an OTel sampling threshold for the tracestate feature left out of
// probabilistic.go (see its header comment and ../NOTICE.md). Evaluate
// itself discards that threshold, so this copy keeps the plain
// v0.154.0-shaped Evaluate below, which returns byte-identical Decision
// values to v0.159.0's Evaluate.

package sampling // import "github.com/open-telemetry/opentelemetry-collector-contrib/processor/tailsamplingprocessor/internal/sampling"

import (
	"context"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.uber.org/zap"

	"github.com/open-telemetry/opentelemetry-collector-contrib/processor/tailsamplingprocessor/pkg/samplingpolicy"
)

type And struct {
	// the subpolicy evaluators
	subpolicies []samplingpolicy.Evaluator
	logger      *zap.Logger
}

var _ samplingpolicy.Evaluator = (*And)(nil)

func NewAnd(
	logger *zap.Logger,
	subpolicies []samplingpolicy.Evaluator,
) samplingpolicy.Evaluator {
	return &And{
		subpolicies: subpolicies,
		logger:      logger,
	}
}

// Evaluate looks at the trace data and returns a corresponding SamplingDecision.
func (c *And) Evaluate(ctx context.Context, traceID pcommon.TraceID, trace *samplingpolicy.TraceData) (samplingpolicy.Decision, error) {
	// The policy iterates over all sub-policies and returns Sampled if all sub-policies returned a Sampled Decision.
	// If any subpolicy returns NotSampled or InvertNotSampled, it returns NotSampled Decision.
	for _, sub := range c.subpolicies {
		decision, err := sub.Evaluate(ctx, traceID, trace)
		if err != nil {
			return samplingpolicy.Unspecified, err
		}
		//nolint:staticcheck // SA1019: Use of inverted decisions until they are fully removed.
		if decision == samplingpolicy.NotSampled || decision == samplingpolicy.InvertNotSampled {
			return samplingpolicy.NotSampled, nil
		}
	}
	return samplingpolicy.Sampled, nil
}

func (c *And) IsStateful() bool {
	for _, sub := range c.subpolicies {
		if sub.IsStateful() {
			return true
		}
	}
	return false
}
