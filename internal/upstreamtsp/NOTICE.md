# Vendored from opentelemetry-collector-contrib

The Go files under `sampling/` and `filterottl/` in this directory are copied
(with the modifications noted below) from:

    github.com/open-telemetry/opentelemetry-collector-contrib
    processor/tailsamplingprocessor/internal/sampling   @ v0.159.0
    internal/filter/filterottl                          @ v0.159.0

Licensed under Apache-2.0; see LICENSE in this directory.

## Why a copy instead of an import

Both source packages live under a path segment named `internal`, which the
Go toolchain only lets code inside
`github.com/open-telemetry/opentelemetry-collector-contrib/...` import. This
repository is a separate module, so it cannot `import` them directly — this
is a real, compiler-enforced constraint, not a licensing one. Copying the
source (permitted by Apache-2.0 with attribution) is the only way to reuse
this logic verbatim.

Everything these files depend on other than each other is public API:
`pkg/samplingpolicy` (the real, unmodified upstream package, imported
normally as a Go module dependency — see go.mod) and `pkg/ottl`, both of
which live outside any `internal/` segment.

## What was left out

`internal/sampling` also has `composite.go`, `drop.go`, `not.go`,
`trace_flags.go`, `bytes_limiting.go` and `time_provider.go`. They were not
copied because this project's `sampling.PolicyCfg` (see
`internal/sampling/otel.go`) doesn't expose `composite`/`drop`/`not`/
`trace_flags` policy types yet — the same "out of scope for the MVP" line
already drawn in that file's doc comment. Nothing below is patched to make
this true; it's just an unused-code question, so add them later if the
schema grows.

## What was intentionally simplified (not just copy-pasted)

- **`probabilistic.go`**: upstream's v0.159.0 version can honor the W3C
  tracestate `rv`/`th` fields (OTel's "consistent probability sampling")
  behind the `processor.tailsamplingprocessor.usetracestate` feature gate.
  Reading that gate's live value requires
  `.../tailsamplingprocessor/internal/metadata`, itself an internal package
  we can't import either, and the gate defaults to disabled in a stock
  Collector build. Rather than vendor the gate registry too, this copy
  hardcodes the pre-tracestate legacy path (FNV-1a hash of `hashSalt` +
  TraceID vs. a `big.Float`-computed threshold) — i.e. exactly what
  v0.159.0 does when nobody has passed `--feature-gates=+processor.tail...`.
- **`and.go`**: upstream's v0.159.0 version implements `Evaluate` in terms
  of a `ThresholdEvaluator.EvaluateWithThreshold` that also propagates the
  OTel sampling threshold for the same tracestate feature above. Since
  `Evaluate` itself just discards that threshold, this copy keeps the
  plain v0.154.0-shaped `Evaluate` (all sub-policies must return Sampled),
  which produces byte-identical `Decision` values to v0.159.0's `Evaluate`.

Everything else (`always_sample.go`, `latency.go`, `numeric_tag_filter.go`,
`string_tag_filter.go`, `boolean_tag_filter.go`, `status_code.go`,
`span_count_sampler.go`, `trace_state_filter.go`, `rate_limiting.go`,
`ottl.go`, `util.go` trimmed to the helpers actually used) is byte-for-byte
upstream logic, modulo import paths (`filterottl` now points at the local
copy below instead of the internal one).

`filterottl/` is trimmed to the span/spanevent-only surface `ottl.go`
actually calls: `NewBoolExprForSpanWithPathContextNames`,
`NewBoolExprForSpanEventWithPathContextNames`, `StandardSpanFuncs`,
`StandardSpanEventFuncs`, and their shared helpers. The metric/log/resource/
scope/datapoint/exemplar/profile variants in the real package were dropped
since nothing here calls them.
