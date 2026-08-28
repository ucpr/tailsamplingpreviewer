// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

// Trimmed vendor copy of internal/filter/filterottl @ v0.159.0: only the
// span-related surface ../sampling/ottl.go actually calls is kept. See
// ../NOTICE.md.

package filterottl

import (
	"go.opentelemetry.io/collector/component"

	"github.com/open-telemetry/opentelemetry-collector-contrib/pkg/ottl"
	"github.com/open-telemetry/opentelemetry-collector-contrib/pkg/ottl/contexts/ottlspan"
	"github.com/open-telemetry/opentelemetry-collector-contrib/pkg/ottl/contexts/ottlspanevent"
)

// newBoolExprWithPathContextNames wraps parser in a single-context ottl.ParserCollection so
// that conditions without an explicit path context are rewritten to use contextName as their
// context. The parser must be constructed with EnablePathContextNames().
func newBoolExprWithPathContextNames[K any](
	contextName string,
	parser ottl.Parser[K],
	conditions []string,
	set component.TelemetrySettings,
	newConditionSequence func([]*ottl.Condition[K]) ottl.ConditionSequence[K],
) (*ottl.ConditionSequence[K], error) {
	pc, err := ottl.NewParserCollection(
		set,
		ottl.EnableParserCollectionModifiedPathsLogging[*ottl.ConditionSequence[K]](true),
		ottl.WithParserCollectionContext(
			contextName,
			&parser,
			ottl.WithConditionConverter(func(_ *ottl.ParserCollection[*ottl.ConditionSequence[K]], _ ottl.ConditionsGetter, parsed []*ottl.Condition[K]) (*ottl.ConditionSequence[K], error) {
				cs := newConditionSequence(parsed)
				return &cs, nil
			}),
		),
	)
	if err != nil {
		return nil, err
	}
	return pc.ParseConditionsWithContext(contextName, ottl.NewConditionsGetter(conditions), true)
}

// NewBoolExprForSpanWithPathContextNames is like NewBoolExprForSpan, but conditions may use OTTL
// path context names (e.g. `span.attributes["foo"]`). Conditions without an explicit context are
// rewritten to use the span context (e.g. `attributes["foo"]` becomes `span.attributes["foo"]`).
func NewBoolExprForSpanWithPathContextNames(conditions []string, functions map[string]ottl.Factory[*ottlspan.TransformContext], errorMode ottl.ErrorMode, set component.TelemetrySettings) (*ottl.ConditionSequence[*ottlspan.TransformContext], error) {
	parser, err := ottlspan.NewParser(functions, set, ottlspan.EnablePathContextNames())
	if err != nil {
		return nil, err
	}
	return newBoolExprWithPathContextNames(ottlspan.ContextName, parser, conditions, set, func(parsed []*ottl.Condition[*ottlspan.TransformContext]) ottl.ConditionSequence[*ottlspan.TransformContext] {
		return ottlspan.NewConditionSequence(parsed, set, ottlspan.WithConditionSequenceErrorMode(errorMode))
	})
}

// NewBoolExprForSpanEventWithPathContextNames is like NewBoolExprForSpanEvent, but conditions may use
// OTTL path context names (e.g. `spanevent.attributes["foo"]`). Conditions without an explicit
// context are rewritten to use the spanevent context.
func NewBoolExprForSpanEventWithPathContextNames(conditions []string, functions map[string]ottl.Factory[*ottlspanevent.TransformContext], errorMode ottl.ErrorMode, set component.TelemetrySettings) (*ottl.ConditionSequence[*ottlspanevent.TransformContext], error) {
	parser, err := ottlspanevent.NewParser(functions, set, ottlspanevent.EnablePathContextNames())
	if err != nil {
		return nil, err
	}
	return newBoolExprWithPathContextNames(ottlspanevent.ContextName, parser, conditions, set, func(parsed []*ottl.Condition[*ottlspanevent.TransformContext]) ottl.ConditionSequence[*ottlspanevent.TransformContext] {
		return ottlspanevent.NewConditionSequence(parsed, set, ottlspanevent.WithConditionSequenceErrorMode(errorMode))
	})
}
