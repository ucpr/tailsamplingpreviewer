#!/usr/bin/env bash
#
# Refreshes the vendored tailsamplingprocessor/filterottl copy under this
# directory (internal/upstreamtsp) to a given version of
# github.com/open-telemetry/opentelemetry-collector-contrib. See NOTICE.md
# for what's vendored here, what's deliberately left out, and why.
#
# Usage:
#   internal/upstreamtsp/update.sh            # update to the latest release
#   internal/upstreamtsp/update.sh v0.160.0   # update to a specific version
#
# What this does, in order:
#   1. Resolves the target version (arg, or the latest tailsamplingprocessor
#      release on the Go module proxy).
#   2. `go mod download`s tailsamplingprocessor and internal/filter at that
#      version into the local module cache. internal/filter is never a
#      go.mod dependency of this repo (it's `internal/`-restricted, hence
#      vendored) -- this step only reads its source, it doesn't add it.
#   3. Copies every byte-for-byte-vendored file (see NOTICE.md) straight
#      from the module cache, overwriting what's here now.
#   4. Rewrites the copied sampling/ottl.go's filterottl import to point at
#      the local package instead of the internal one -- the one line this
#      repo's copy is allowed to differ on.
#   5. Prints a diff of and.go and probabilistic.go (new upstream source vs.
#      this repo's hand-adapted copies) for manual review. These two are
#      NEVER auto-overwritten: they intentionally diverge from upstream
#      (see NOTICE.md's "What was intentionally simplified" section), so an
#      automatic copy would silently undo that. Read the diff, and if
#      upstream changed something other than the tracestate/threshold bits
#      already accounted for, port it into and.go/probabilistic.go by hand.
#   6. Warns about any *.go file in upstream's internal/sampling that this
#      script doesn't know about (a new policy type, a file split, etc.) --
#      silence here would mean silently continuing to ship a stale/missing
#      file after an upstream restructure.
#   7. `go get`s the new tailsamplingprocessor version (bumping go.mod) and
#      runs `go mod tidy`.
#   8. Runs `go build ./... && go vet ./... && go test ./...` and reports
#      the result.
#
# What this does NOT do: touch composite.go/drop.go/not.go/trace_flags.go/
# bytes_limiting.go/time_provider.go (not vendored at all -- see NOTICE.md),
# or edit NOTICE.md's prose. Update NOTICE.md's stated version by hand if
# anything in its "What was intentionally simplified" section needed a
# manual change in step 5.

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
VENDOR_DIR="$REPO_ROOT/internal/upstreamtsp"
MODULE="github.com/open-telemetry/opentelemetry-collector-contrib/processor/tailsamplingprocessor"
FILTER_MODULE="github.com/open-telemetry/opentelemetry-collector-contrib/internal/filter"

# Copied verbatim (see NOTICE.md).
SAMPLING_FILES=(
	always_sample.go
	latency.go
	numeric_tag_filter.go
	string_tag_filter.go
	boolean_tag_filter.go
	status_code.go
	span_count_sampler.go
	trace_state_filter.go
	rate_limiting.go
	util.go
	doc.go
)
# Copied verbatim except a rewritten import (see the `sed` step below).
SAMPLING_FILE_WITH_IMPORT_REWRITE=ottl.go
# Never auto-copied; diffed against upstream for manual review only.
SAMPLING_FILES_MANUAL=(and.go probabilistic.go)
# Deliberately never vendored (see NOTICE.md "What was left out"); listed
# here only so the "unexpected upstream file" check in step 6 doesn't warn
# about them.
SAMPLING_FILES_EXCLUDED=(composite.go drop.go not.go trace_flags.go bytes_limiting.go time_provider.go)

FILTEROTTL_FILES=(filter.go path_context.go functions.go)

cd "$REPO_ROOT"

VERSION="${1:-}"
if [[ -z "$VERSION" ]]; then
	echo "==> No version given, resolving the latest $MODULE release ..." >&2
	VERSION="$(go list -m -versions "$MODULE" | tr ' ' '\n' | tail -1)"
fi
echo "==> Target version: $VERSION" >&2

echo "==> Downloading $MODULE@$VERSION and $FILTER_MODULE@$VERSION ..." >&2
SAMPLING_DIR="$(go mod download -json "$MODULE@$VERSION" | sed -n 's/.*"Dir": "\([^"]*\)".*/\1/p')/internal/sampling"
FILTEROTTL_DIR="$(go mod download -json "$FILTER_MODULE@$VERSION" | sed -n 's/.*"Dir": "\([^"]*\)".*/\1/p')/filterottl"

if [[ ! -d "$SAMPLING_DIR" || ! -d "$FILTEROTTL_DIR" ]]; then
	echo "error: could not resolve module cache directories (got sampling=$SAMPLING_DIR filterottl=$FILTEROTTL_DIR)" >&2
	exit 1
fi

echo "==> Copying vendored files ..." >&2
for f in "${SAMPLING_FILES[@]}"; do
	cp "$SAMPLING_DIR/$f" "$VENDOR_DIR/sampling/$f"
done
sed \
	's#"github.com/open-telemetry/opentelemetry-collector-contrib/internal/filter/filterottl"#"github.com/ucpr/tailsamplingpreviewer/internal/upstreamtsp/filterottl"#' \
	"$SAMPLING_DIR/$SAMPLING_FILE_WITH_IMPORT_REWRITE" >"$VENDOR_DIR/sampling/$SAMPLING_FILE_WITH_IMPORT_REWRITE"
for f in "${FILTEROTTL_FILES[@]}"; do
	cp "$FILTEROTTL_DIR/$f" "$VENDOR_DIR/filterottl/$f"
done
cp "$(dirname "$SAMPLING_DIR")/../LICENSE" "$VENDOR_DIR/LICENSE" 2>/dev/null || true

# The sed rewrite above can leave sampling/ottl.go's import block in the
# wrong alphabetical position (gofmt sorts within an import group but
# doesn't regroup by module the way goimports would); this at least fixes
# the sorting. A stray misplaced import line is harmless -- Go doesn't
# care about import order -- so this is tidiness, not correctness.
gofmt -w "$VENDOR_DIR"/sampling/*.go "$VENDOR_DIR"/filterottl/*.go

echo "==> and.go / probabilistic.go are hand-adapted and were NOT overwritten." >&2
echo "    Review this diff (new upstream source vs. this repo's copy) and port" >&2
echo "    over anything beyond the tracestate/threshold simplification noted" >&2
echo "    in NOTICE.md:" >&2
for f in "${SAMPLING_FILES_MANUAL[@]}"; do
	echo "----- $f -----"
	diff -u "$VENDOR_DIR/sampling/$f" "$SAMPLING_DIR/$f" || true
done

echo "==> Checking for new/unexpected files in upstream's internal/sampling ..." >&2
KNOWN_FILES=("${SAMPLING_FILES[@]}" "$SAMPLING_FILE_WITH_IMPORT_REWRITE" "${SAMPLING_FILES_MANUAL[@]}" "${SAMPLING_FILES_EXCLUDED[@]}")
for f in "$SAMPLING_DIR"/*.go; do
	base="$(basename "$f")"
	[[ "$base" == *_test.go ]] && continue
	known=false
	for k in "${KNOWN_FILES[@]}"; do
		[[ "$base" == "$k" ]] && known=true && break
	done
	if ! $known; then
		echo "    warning: $base exists upstream but this script doesn't vendor it -- check if it needs adding" >&2
	fi
done

echo "==> Bumping go.mod to $MODULE@$VERSION ..." >&2
go get "$MODULE@$VERSION"
go mod tidy

echo "==> Building and testing ..." >&2
if go build ./... && go vet ./... && go test ./...; then
	echo "==> OK: build/vet/test all passed with $MODULE@$VERSION." >&2
else
	echo "==> FAILED: fix the build/vet/test failures above before committing." >&2
	exit 1
fi

echo "==> Done. Remember to: review the and.go/probabilistic.go diff above," >&2
echo "    update the version numbers in NOTICE.md, and commit." >&2
