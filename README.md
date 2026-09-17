## tailsamplingpreviewer🧪

Tail sampling policies are hard to get right. You often don’t know what a policy will actually drop until it’s running in production.

This tool mirrors real Collector traffic to a local preview server over WebSocket, letting you see in real time which traces a `tail_sampling` policy would KEEP or DROP—before deploying the policy to production.

> [!WARNING]
> This is a PoC. Interfaces, config and the wire protocol may change without notice.

```text
Application ──OTLP──► Collector ──┬──► production backend
                                  └──WebSocket──► tailpreview ──► Browser UI
```

## Installation

Requires Go 1.25+ and Node.js 22+ (the UI is served from a built static bundle).

```sh
git clone https://github.com/ucpr/tailsamplingpreviewer
cd tailsamplingpreviewer

# Browser UI
cd web && npm ci && npm run build && cd ..

# Preview server
go build -o bin/tailpreview ./cmd/tailpreview

# A minimal Collector distribution bundling tailpreviewexporter
go build -o bin/otelcol-tailpreview ./cmd/otelcol-tailpreview
```

## Usage

Try the whole pipeline (synthetic load generator → Collector → preview server) with Docker:

```sh
docker compose -f examples/compose.yaml up --build
# then open http://localhost:17778
```

To use it against your own Collector, start the preview server:

```sh
./bin/tailpreview            # UI on http://127.0.0.1:17778, Collector WS on 127.0.0.1:17777
```

and add `tailpreview` as an extra exporter on your traces pipeline, alongside the production one
(see `examples/otelcol-config.yaml`):

```yaml
exporters:
  tailpreview:
    endpoint: ws://127.0.0.1:17777/v1/collector

service:
  pipelines:
    traces:
      exporters: [otlp/production, tailpreview]
```

Traces are shadowed best-effort: if the preview server is slow or down, preview data is dropped and
the production pipeline is never blocked. Nothing is persisted to disk.

In the UI you can edit the policy, import/export `tail_sampling` YAML, filter the live tail, inspect
why a trace matched, and compare a candidate policy against the current one.

`tailpreview` flags: `--collector-endpoint`, `--ui-endpoint`, `--web-dir`, `--max-traces`,
`--max-age`, `--max-memory-bytes`.

## Contribution

Issues and pull requests are welcome. Before opening a PR:

```sh
go test ./...
cd web && npm run lint && npm run build
```

Design notes and the remaining scope live in [spec.md](spec.md).

## LICENSE

MIT. See [LICENSE](LICENSE).

`internal/upstreamtsp/` contains code vendored from
[opentelemetry-collector-contrib](https://github.com/open-telemetry/opentelemetry-collector-contrib)
under Apache-2.0; see `internal/upstreamtsp/LICENSE` and `internal/upstreamtsp/NOTICE.md`.
