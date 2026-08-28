// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

// Trimmed vendor copy of internal/filter/filterottl @ v0.159.0: only the
// span-related surface ../sampling/ottl.go calls is kept. See ../NOTICE.md.

package filterottl

import (
	"go.opentelemetry.io/collector/component"

	"github.com/open-telemetry/opentelemetry-collector-contrib/pkg/ottl"
	"github.com/open-telemetry/opentelemetry-collector-contrib/pkg/ottl/contexts/ottlspan"
	"github.com/open-telemetry/opentelemetry-collector-contrib/pkg/ottl/contexts/ottlspanevent"
)

// NewBoolExprForSpan creates a BoolExpr[*ottlspan.TransformContext] that will return true if any of the given OTTL conditions evaluate to true.
// The passed in functions should use the ottlspan.TransformContext.
// If a function named `match` is not present in the function map it will be added automatically so that parsing works as expected
func NewBoolExprForSpan(conditions []string, functions map[string]ottl.Factory[*ottlspan.TransformContext], errorMode ottl.ErrorMode, set component.TelemetrySettings) (*ottl.ConditionSequence[*ottlspan.TransformContext], error) {
	return NewBoolExprForSpanWithOptions(conditions, functions, errorMode, set, nil)
}

// NewBoolExprForSpanWithOptions is like NewBoolExprForSpan, but with additional options.
func NewBoolExprForSpanWithOptions(conditions []string, functions map[string]ottl.Factory[*ottlspan.TransformContext], errorMode ottl.ErrorMode, set component.TelemetrySettings, parserOptions []ottl.Option[*ottlspan.TransformContext]) (*ottl.ConditionSequence[*ottlspan.TransformContext], error) {
	parser, err := ottlspan.NewParser(functions, set, parserOptions...)
	if err != nil {
		return nil, err
	}
	statements, err := parser.ParseConditions(conditions)
	if err != nil {
		return nil, err
	}
	c := ottlspan.NewConditionSequence(statements, set, ottlspan.WithConditionSequenceErrorMode(errorMode))
	return &c, nil
}

// NewBoolExprForSpanEvent creates a BoolExpr[*ottlspanevent.TransformContext] that will return true if any of the given OTTL conditions evaluate to true.
// The passed in functions should use the ottlspanevent.TransformContext.
// If a function named `match` is not present in the function map it will be added automatically so that parsing works as expected
func NewBoolExprForSpanEvent(conditions []string, functions map[string]ottl.Factory[*ottlspanevent.TransformContext], errorMode ottl.ErrorMode, set component.TelemetrySettings) (*ottl.ConditionSequence[*ottlspanevent.TransformContext], error) {
	return NewBoolExprForSpanEventWithOptions(conditions, functions, errorMode, set, nil)
}

// NewBoolExprForSpanEventWithOptions is like NewBoolExprForSpanEvent, but with additional options.
func NewBoolExprForSpanEventWithOptions(conditions []string, functions map[string]ottl.Factory[*ottlspanevent.TransformContext], errorMode ottl.ErrorMode, set component.TelemetrySettings, parserOptions []ottl.Option[*ottlspanevent.TransformContext]) (*ottl.ConditionSequence[*ottlspanevent.TransformContext], error) {
	parser, err := ottlspanevent.NewParser(functions, set, parserOptions...)
	if err != nil {
		return nil, err
	}
	statements, err := parser.ParseConditions(conditions)
	if err != nil {
		return nil, err
	}
	c := ottlspanevent.NewConditionSequence(statements, set, ottlspanevent.WithConditionSequenceErrorMode(errorMode))
	return &c, nil
}
