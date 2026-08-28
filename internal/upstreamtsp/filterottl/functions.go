// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

// Trimmed vendor copy of internal/filter/filterottl @ v0.159.0: only the
// span-related function sets ../sampling/ottl.go actually calls are kept.
// See ../NOTICE.md.

package filterottl

import (
	"github.com/open-telemetry/opentelemetry-collector-contrib/pkg/ottl"
	"github.com/open-telemetry/opentelemetry-collector-contrib/pkg/ottl/contexts/ottlspan"
	"github.com/open-telemetry/opentelemetry-collector-contrib/pkg/ottl/contexts/ottlspanevent"
	"github.com/open-telemetry/opentelemetry-collector-contrib/pkg/ottl/ottlfuncs"
)

func StandardSpanFuncs() map[string]ottl.Factory[*ottlspan.TransformContext] {
	m := ottlfuncs.StandardConverters[*ottlspan.TransformContext]()
	isRootSpanFactory := ottlfuncs.NewIsRootSpanFactory()
	m[isRootSpanFactory.Name()] = isRootSpanFactory
	return m
}

func StandardSpanEventFuncs() map[string]ottl.Factory[*ottlspanevent.TransformContext] {
	return ottlfuncs.StandardConverters[*ottlspanevent.TransformContext]()
}
