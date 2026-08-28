// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

// Trimmed vendor copy: upstream's v0.159.0 util.go additionally defines
// SetAttrOnScopeSpans, SetBoolAttrOnScopeSpans (used only by composite.go,
// not vendored here) and WriteEffectiveThreshold (part of the tracestate
// threshold-propagation feature also left out, see ../NOTICE.md). Only the
// four helpers every vendored filter here actually calls are kept.

package sampling // import "github.com/open-telemetry/opentelemetry-collector-contrib/processor/tailsamplingprocessor/internal/sampling"

import (
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/open-telemetry/opentelemetry-collector-contrib/processor/tailsamplingprocessor/pkg/samplingpolicy"
)

// hasResourceOrSpanWithCondition iterates through all the resources and instrumentation library spans until any
// callback returns true.
func hasResourceOrSpanWithCondition(
	td ptrace.Traces,
	shouldSampleResource func(resource pcommon.Resource) bool,
	shouldSampleSpan func(span ptrace.Span) bool,
) samplingpolicy.Decision {
	for i := 0; i < td.ResourceSpans().Len(); i++ {
		rs := td.ResourceSpans().At(i)

		resource := rs.Resource()
		if shouldSampleResource(resource) {
			return samplingpolicy.Sampled
		}

		if hasInstrumentationLibrarySpanWithCondition(rs.ScopeSpans(), shouldSampleSpan, false) {
			return samplingpolicy.Sampled
		}
	}
	return samplingpolicy.NotSampled
}

// invertHasResourceOrSpanWithCondition iterates through all the resources and instrumentation library spans until any
// callback returns false.
func invertHasResourceOrSpanWithCondition(
	td ptrace.Traces,
	shouldSampleResource func(resource pcommon.Resource) bool,
	shouldSampleSpan func(span ptrace.Span) bool,
) samplingpolicy.Decision {
	for i := 0; i < td.ResourceSpans().Len(); i++ {
		rs := td.ResourceSpans().At(i)

		resource := rs.Resource()
		if !shouldSampleResource(resource) {
			return samplingpolicy.NotSampled
		}

		if !hasInstrumentationLibrarySpanWithCondition(rs.ScopeSpans(), shouldSampleSpan, true) {
			return samplingpolicy.NotSampled
		}
	}

	return samplingpolicy.Sampled
}

// hasSpanWithCondition iterates through all the instrumentation library spans until any callback returns true.
func hasSpanWithCondition(td ptrace.Traces, shouldSample func(span ptrace.Span) bool) samplingpolicy.Decision {
	for i := 0; i < td.ResourceSpans().Len(); i++ {
		rs := td.ResourceSpans().At(i)

		if hasInstrumentationLibrarySpanWithCondition(rs.ScopeSpans(), shouldSample, false) {
			return samplingpolicy.Sampled
		}
	}
	return samplingpolicy.NotSampled
}

func hasInstrumentationLibrarySpanWithCondition(ilss ptrace.ScopeSpansSlice, check func(span ptrace.Span) bool, invert bool) bool {
	for i := 0; i < ilss.Len(); i++ {
		ils := ilss.At(i)

		for j := 0; j < ils.Spans().Len(); j++ {
			span := ils.Spans().At(j)

			if r := check(span); r != invert {
				return r
			}
		}
	}
	return invert
}
