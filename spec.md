# OpenTelemetry Tail Sampling Preview 設計書 v2

## 1. 概要

OpenTelemetry Collector の Tail Sampling Policy を、本番適用前に実トラフィックを利用してリアルタイムに検証するためのツールを開発する。

Collector に専用の `tailpreviewexporter` を導入し、Preview Server と WebSocket の常時接続を確立する。

Collector が受信した Trace は Production Backend への通常送信と並行して Preview Server に Shadow Traffic として送信する。

Preview Server では Trace を再構成し、Tail Sampling Policy を適用した場合の、

* KEEP / DROP
* マッチした Policy
* Sampling Rate
* Policy ごとの Match Rate
* Policy 変更前後の差分

を Datadog Live Tail に近い UI でリアルタイムに確認できるようにする。

最終的には、

> Tail Sampling Policy に対する Terraform `plan`

のような体験を提供する。

---

# 2. コンセプト

本ツールは Trace Backend ではない。

中心となる体験は以下である。

```text
Observe real traces
        ↓
Build sampling policy
        ↓
Preview KEEP / DROP
        ↓
Understand why
        ↓
Measure impact
        ↓
Export Collector config
```

そのため、

**OpenTelemetry Trace Viewer**

ではなく、

**Tail Sampling Policy Debugger / Playground**

として設計する。

---

# 3. ゴール

## MVP

以下を実現する。

* Collector の Trace を Preview Server に Shadow Traffic として送信できる
* Collector と Preview Server は WebSocket で常時接続する
* Preview Server が停止しても Production Pipeline に影響しない
* Trace をリアルタイムに Live Tail 表示できる
* Trace ID 単位で Span を再構成できる
* Tail Sampling Policy を設定できる
* KEEP / DROP をリアルタイムに Preview できる
* Trace がどの Policy にマッチしたか確認できる
* Sampling Rate をリアルタイム表示できる
* OpenTelemetry Collector の `tail_sampling` YAML を import / export できる
* Preview Session を開始・停止できる

---

# 4. Non-Goals

MVP では以下を対象外とする。

* Trace Backend の代替
* Trace の長期保存
* Metrics / Logs Preview
* Production Tail Sampling Processor の置換
* Production Collector configuration の自動更新
* Sampling Policy の自動生成
* 分散 Preview Server
* Cost Prediction

---

# 5. 全体アーキテクチャ

```text
                        Production Data Plane

 Application
      │
      │ OTLP
      ▼
┌───────────────────────────────────────┐
│ OpenTelemetry Collector              │
│                                       │
│ OTLP Receiver                         │
│      │                                │
│      ├──────► Production Exporter ───────► Datadog / Tempo
│      │
│      └──────► tailpreviewexporter
│                     │
└─────────────────────┼─────────────────┘
                      │
                      │ WebSocket
                      │ OTLP protobuf
                      │
                      ▼
            ┌────────────────────┐
            │ Preview Server     │
            │                    │
            │ Session Manager    │
            │ Trace Assembler    │
            │ Policy Engine      │
            │ Statistics Engine  │
            │ Ring Buffer        │
            └──────────┬─────────┘
                       │
                       │ WebSocket
                       ▼
              ┌─────────────────┐
              │ Browser UI      │
              │                 │
              │ Live Tail       │
              │ Policy Builder  │
              │ Statistics      │
              └─────────────────┘
```

Collector ↔ Preview Server と Preview Server ↔ Browser は別 connection とする。

---

# 6. Protocol Architecture

通信は Data Plane と Control Plane に分離する。

```text
Collector
    ║
    ║ Data Plane
    ║
    ║ OTLP protobuf
    ║
    ▼
Preview Server


Collector
    ▲
    │ Control Plane
    │
    │ JSON
    ▼
Preview Server
```

同じ WebSocket connection を利用してもよいが、Frame Type を明確に分離する。

---

# 7. Collector → Preview Server Data Plane

Trace data は OTLP のデータモデルをそのまま利用する。

独自の Trace JSON format は作成しない。

WebSocket Binary Frame:

```text
+-------------------------------+
| Frame Header                  |
+-------------------------------+
| ExportTraceServiceRequest     |
| protobuf                      |
+-------------------------------+
```

論理 payload:

```protobuf
ExportTraceServiceRequest {
    resource_spans: [...]
}
```

これにより、

```text
Collector pdata
      ↓
OTLP protobuf
      ↓
WebSocket
      ↓
Preview Server pdata
```

という単純な変換で済む。

---

# 8. Control Plane

WebSocket の双方向性を利用し、Preview Server から Exporter を制御できるようにする。

ただし Sampling Policy の評価自体は Preview Server で行う。

Exporter は Data Transport に専念する。

Control Message 例:

```json
{
  "type": "session.start",
  "session_id": "01J..."
}
```

```json
{
  "type": "session.stop",
  "session_id": "01J..."
}
```

```json
{
  "type": "session.pause"
}
```

```json
{
  "type": "session.resume"
}
```

---

# 9. Preview Session

WebSocket を採用する主要な理由の1つとして Session の概念を導入する。

状態:

```text
DISCONNECTED

    ↓

CONNECTED

    ↓

IDLE

    ↓ session.start

STREAMING

    ↓ session.pause

PAUSED

    ↓ session.stop

IDLE
```

UI:

```text
Collector

● Connected

otel-gateway-01
Collector v0.xxx.x

────────────────────

Preview Session

● Streaming

Duration        02:18
Spans         821,281
Traces         91,281

[ Pause ] [ Stop ]
```

---

# 10. Exporter Configuration

Collector:

```yaml
exporters:
  tailpreview:
    endpoint: ws://127.0.0.1:17777/v1/collector

    queue:
      enabled: true
      size: 1000

    reconnect:
      enabled: true
      interval: 5s
```

Pipeline:

```yaml
service:
  pipelines:
    traces:
      receivers:
        - otlp

      processors:
        - memory_limiter
        - batch

      exporters:
        - otlp/production
        - tailpreview
```

---

# 11. tailpreviewexporter

Exporter は可能な限り薄くする。

責務:

```text
pdata.Traces
    ↓
serialization
    ↓
bounded queue
    ↓
WebSocket writer
```

実施しないもの:

* Trace Assembly
* Tail Sampling Evaluation
* Filter Evaluation
* Statistics
* Query
* Storage

これらは Preview Server に集約する。

---

# 12. Exporter Internal Architecture

```text
                   Collector Pipeline
                          │
                          ▼
                  ConsumeTraces()
                          │
                          ▼
                    Serializer
                          │
                          ▼
                  ┌──────────────┐
                  │ Bounded Queue│
                  └──────┬───────┘
                         │
                         ▼
                  Writer Goroutine
                         │
                         ▼
                     WebSocket
```

WebSocket に対する write は単一 goroutine に集約する。

```go
type Exporter struct {
    queue chan Payload

    connection *ConnectionManager
}
```

```go
func (e *Exporter) consumeTraces(
    ctx context.Context,
    td ptrace.Traces,
) error {
    payload, err := marshal(td)
    if err != nil {
        return err
    }

    select {
    case e.queue <- payload:
        return nil

    default:
        e.metrics.Dropped.Add(1)

        // Preview は best effort。
        // Production pipeline へ backpressure を返さない。
        return nil
    }
}
```

---

# 13. Backpressure Policy

本ツールで最も重要な設計原則とする。

```text
Preview overloaded
        │
        ▼
Preview traces dropped

NOT

Production blocked
```

以下の場合は Preview data を drop する。

* Queue full
* Preview Server unreachable
* WebSocket write timeout
* Serialization backlog
* Preview Session paused

Production Pipeline にはエラーを返さない。

---

# 14. Connection Manager

Exporter 内に WebSocket Connection Manager を持つ。

責務:

* connect
* reconnect
* heartbeat
* session state
* writer lifecycle
* server control message reception

```text
ConnectionManager
      │
      ├── Connector
      │
      ├── Reader
      │
      ├── Writer
      │
      └── Heartbeat
```

---

# 15. Reconnect

Preview Server が再起動しても Collector 自体は正常稼働し続ける。

```text
Preview Server DOWN

Collector
    │
    ├── Production → OK
    │
    └── Preview → DROP
```

Exporter:

```text
CONNECTED
    │
    ▼
DISCONNECTED
    │
    │ exponential backoff
    ▼
CONNECTING
    │
    ▼
CONNECTED
```

Reconnect 中に Trace を永続化しない。

---

# 16. Heartbeat

Control Frame:

```json
{
  "type": "ping",
  "timestamp": 1787731200000
}
```

Response:

```json
{
  "type": "pong",
  "timestamp": 1787731200000
}
```

Preview Server は Collector connection の health を UI に表示する。

---

# 17. Collector Identity

connection 確立時に Collector metadata を送信する。

```json
{
  "type": "hello",

  "collector": {
    "id": "otel-gateway-01",
    "version": "0.xxx.x"
  },

  "exporter": {
    "version": "0.1.0"
  }
}
```

Preview Server:

```json
{
  "type": "hello.ack",

  "server": {
    "version": "0.1.0"
  }
}
```

---

# 18. 複数 Collector

Preview Server は複数 Collector connection を受け入れる。

```text
Collector A ──┐
              │
Collector B ──┼──► Preview Server
              │
Collector C ──┘
```

UI:

```text
Collectors

● gateway-01
● gateway-02
● gateway-03
```

ただし Tail Sampling の正確性については、

```text
same trace_id
    ↓
same Preview Server
```

を保証する必要がある。

MVP では Preview Server を single instance とすることで解決する。

---

# 19. Preview Server

Preview Server は本システムの中心となる。

```text
WebSocket Ingest
      │
      ▼
Protocol Decoder
      │
      ▼
Trace Assembler
      │
      ├──────────► Live Update
      │
      ▼
Trace Store
      │
      ▼
Policy Engine
      │
      ├──────────► Decision Store
      │
      ▼
Statistics Engine
      │
      ▼
Browser Stream
```

---

# 20. Trace Assembler

Span を Trace ID 単位に集約する。

```go
type TraceState struct {
    TraceID pcommon.TraceID

    FirstSeen time.Time
    LastSeen  time.Time

    Spans []Span

    SizeBytes uint64

    State TraceStateType
}
```

State:

```text
RECEIVING
READY
DECIDED
EXPIRED
```

---

# 21. Decision Timing

Distributed Trace に明示的な完了通知はないため、Tail Sampling Processor と同様に time based で decision を行う。

```text
first span
    │
    ├──── span
    ├──── span
    │
    └────────── decision_wait
                    │
                    ▼
                 evaluate
```

初期値:

```yaml
decision_wait: 30s
```

---

# 22. Live と Decision を分離する

UI 上では Trace の状態を区別する。

```text
● LIVE
✓ KEEP
× DROP
```

例:

```text
LIVE  payment-api  1.3s  8 spans

KEEP  auth-api     2.3s  ERROR

DROP  catalog      82ms   OK
```

まだ decision_wait が終了していない Trace について KEEP/DROP を断定しない。

---

# 23. Policy Engine

Preview Server 内で Tail Sampling Policy を評価する。

重要なのは、

**Collector Exporter では Policy を評価しない**

ことである。

```text
Collector
     │
     │ ALL preview traces
     ▼
Preview Server
     │
     ├── Policy A
     ├── Policy B
     └── Policy C
```

これにより同一 Trace Set を複数 Policy で何度でも再評価できる。

---

# 24. Policy Update

ユーザーが UI から Policy を変更する。

```text
Policy A

status = ERROR
```

↓

```text
Policy B

status = ERROR
OR
duration > 1000ms
```

Collector への configuration change は発生しない。

Preview Server 内で保持している Trace を即座に再評価する。

```text
Policy changed
      │
      ▼
re-evaluate ring buffer
      │
      ▼
new statistics
```

これを本ツールの重要な UX とする。

---

# 25. Policy Builder

UI:

```text
KEEP IF

[ status.code ] [ = ] [ ERROR ]

              OR

[ duration ] [ > ] [ 1000ms ]

              OR

[ resource.service.name ]
[ = ]
[ payment ]
```

右下で常に結果を表示する。

```text
Sampling Rate

4.21%

KEEP
8,421

DROP
191,579
```

---

# 26. Live Tail

基本表示単位は Trace とする。

```text
18:41:31  KEEP  payment-api   2.3s   ERROR  18 spans
18:41:31  DROP  api           182ms  OK      4 spans
18:41:30  KEEP  auth          1.8s   ERROR   8 spans
```

Datadog Live Tail に近い操作感を目指す。

---

# 27. Live Tail Query

Sampling Policy とは別に UI Filter を持つ。

```text
service.name:payment
```

```text
status:error duration:>1s
```

```text
resource.deployment.environment:production
```

Filter はあくまで表示対象を制御する。

Sampling Decision には影響しない。

---

# 28. Trace Detail

Trace 選択時:

```text
Trace

payment-api
2.31s
ERROR
18 spans

─────────────────────────

Decision

KEEP

Matched Policies

✓ errors
✓ slow
× premium

─────────────────────────

api
├─────────────── 2.31s

 payment
   ├─────────── 1.82s

 postgres
      ├─────── 1.21s

 redis
                  ├─ 80ms
```

---

# 29. Statistics

表示する主要 Metrics:

```text
Observed traces       1,024,821

KEEP                      42,182
DROP                     982,639

Sampling Rate               4.12%
```

Policy:

```text
Policy             Matches

errors              12,381
slow                21,442
premium              4,821
baseline            18,231
```

---

# 30. Policy Compare

将来的に同じ Trace Set を複数 candidate に対して評価できるようにする。

```text
              Current      Candidate

Sampling       4.21%         6.82%

KEEP          42,100        68,200

DROP         957,900       931,800
```

さらに、

```text
Newly Kept
+26,100

Newly Dropped
312

ERROR traces newly dropped
12
```

まで表示する。

---

# 31. Storage

MVP は In-Memory のみ。

```text
Ring Buffer

max traces
max duration
max bytes
```

設定:

```yaml
storage:
  max_traces: 10000
  max_age: 5m
  max_memory: 512MiB
```

最古の Trace から eviction する。

---

# 32. Pause

Preview Session を Pause した場合、新しい Trace を Collector から送らない。

```text
UI

[Pause]
   │
   ▼
Preview Server

session.pause
   │
   ▼
Collector Exporter
```

Exporter:

```text
session == PAUSED

ConsumeTraces
     │
     └── DROP
```

Queue に溜めない。

これにより大量 Trace の buffering を防ぐ。

---

# 33. Browser Communication

Preview Server → Browser も WebSocket を利用する。

```text
Preview Server
      ║
      ║ UI events
      ▼
Browser
```

イベント例:

```json
{
  "type": "trace.updated",
  "trace_id": "abc"
}
```

```json
{
  "type": "trace.decided",
  "trace_id": "abc",
  "decision": "KEEP"
}
```

```json
{
  "type": "statistics.updated",
  "sampling_rate": 0.0421
}
```

Browser へ OTLP protobuf をそのまま送る必要はない。

UI に必要な projection のみ送信する。

---

# 34. Security

Default:

```text
Collector → localhost Preview

Browser → localhost Preview
```

```yaml
server:
  collector_endpoint: 127.0.0.1:17777
  ui_endpoint: 127.0.0.1:17778
```

Remote mode を追加する場合は、

* TLS
* authentication
* origin validation
* connection authorization

を必須とする。

---

# 35. Sensitive Data

Trace に含まれる可能性:

* Authorization Header
* User ID
* Email
* SQL Query
* Request Body
* URL Parameter

Default:

```text
Persistence OFF

Remote access OFF
```

将来的には Collector 側で Preview 専用 transform/redaction processor を挟める。

```text
receiver
   │
   ├── Production
   │
   └── redaction
          │
          ▼
      tailpreview
```

---

# 36. Internal Telemetry

Exporter:

```text
tailpreview_exporter_connected

tailpreview_exporter_connections_total

tailpreview_exporter_reconnect_total

tailpreview_exporter_spans_sent_total

tailpreview_exporter_spans_dropped_total

tailpreview_exporter_queue_size

tailpreview_exporter_write_errors_total
```

Preview Server:

```text
tailpreview_received_spans_total

tailpreview_received_traces_total

tailpreview_pending_traces

tailpreview_trace_buffer_bytes

tailpreview_trace_evicted_total

tailpreview_late_spans_total

tailpreview_policy_evaluations_total

tailpreview_policy_matches_total

tailpreview_decisions_total

tailpreview_sampling_rate

tailpreview_connected_collectors

tailpreview_connected_browsers
```

---

# 37. Repository Structure

```text
tailpreview/

├── cmd/
│   ├── tailpreview/
│   └── otelcol-tailpreview/
│
├── exporter/
│   └── tailpreviewexporter/
│       ├── exporter.go
│       ├── factory.go
│       ├── config.go
│       ├── connection.go
│       ├── protocol.go
│       └── metrics.go
│
├── internal/
│   ├── protocol/
│   │
│   ├── ingest/
│   │
│   ├── trace/
│   │   ├── assembler.go
│   │   └── store.go
│   │
│   ├── sampling/
│   │   ├── evaluator.go
│   │   ├── otel.go
│   │   └── decision.go
│   │
│   ├── session/
│   ├── statistics/
│   ├── query/
│   └── server/
│
├── web/
│   └── ...
│
└── examples/
```

---

# 38. CLI UX

起動:

```bash
$ tailpreview
```

```text
Tail Preview

Collector endpoint
  ws://127.0.0.1:17777/v1/collector

UI
  http://127.0.0.1:17778


Waiting for Collector...


✓ Collector connected

  ID       otel-gateway-01
  Version  v0.xxx.x

Preview session is idle.

Open:
http://127.0.0.1:17778
```

UI で Start:

```text
Preview started.

Receiving traces...

Traces        12,821
Spans        182,811
Trace rate     2,183/s
```

---

# 39. MVP 開発順

## Phase 0: WebSocket Transport

まず Collector → Preview Server の transport のみ検証する。

```text
Collector
    │
    │ WS + protobuf
    ▼
Preview Server
```

実装:

* custom exporter
* WebSocket connect
* protobuf serialization
* bounded queue
* reconnect
* counter

UI は不要。

---

## Phase 1: Live Tail

追加:

* Trace Assembler
* Ring Buffer
* Browser WebSocket
* Trace List

```text
Collector
   ↓
Preview
   ↓
Browser
```

ここで Live Tail として成立させる。

---

## Phase 2: Sampling Preview

追加:

* Tail Sampling Evaluator
* KEEP / DROP
* Matched Policy
* Sampling Rate
* YAML import

これが最初のプロダクト MVP。

---

## Phase 3: Policy Builder

追加:

* Visual Policy Builder
* YAML export
* re-evaluation
* Policy statistics

---

## Phase 4: Policy Compare

追加:

```text
Current
   vs
Candidate
```

* Newly Kept
* Newly Dropped
* Error Trace regression
* Sampling Rate Diff

---

# 40. MVP Completion Criteria

* [ ] `tailpreviewexporter` が WebSocket connection を確立できる
* [ ] OTLP Trace を protobuf binary frame として送信できる
* [ ] Preview Server 再起動後に reconnect できる
* [ ] Preview Server 停止中も Production Pipeline が正常動作する
* [ ] Queue overflow 時に Preview data のみ drop される
* [ ] Preview Session を Start / Pause / Stop できる
* [ ] Live Tail UI で Trace を表示できる
* [ ] Trace ID 単位に Span を aggregate できる
* [ ] Tail Sampling Policy を読み込める
* [ ] KEEP / DROP を Preview できる
* [ ] Matched Policy を確認できる
* [ ] Sampling Rate を確認できる
* [ ] Policy 変更時に既存 Ring Buffer を再評価できる
* [ ] Production Collector の設定を Preview Server から変更しない
* [ ] Trace を永続化しない

---

# 41. 設計原則

## Production First

Preview の障害・遅延・停止は Production Pipeline に伝播させない。

---

## Best Effort

Preview Trace は欠損してもよい。

Collector を block するより data を drop する。

---

## Transport と Evaluation を分離する

```text
Exporter

transport only
```

```text
Preview Server

analysis
evaluation
storage
UI
```

とする。

---

## Full Shadow Stream

Exporter 側で Sampling Policy による filtering をしない。

```text
full trace stream
       ↓
Preview Server
       ↓
arbitrary policy
```

とすることで Policy を自由に変更・比較できる。

---

## Native OTel Data Model

Trace data に独自 schema を作らない。

Collector → Server は OTLP protobuf を利用する。

---

## Interactive Debugger

WebSocket を採用する目的は単なる connection reuse ではない。

以下の UX を提供するために利用する。

```text
connection
session
pause/resume
live updates
collector identity
control messages
```

---

# 42. 最終イメージ

```text
                        OpenTelemetry Collector

                         Production
                            │
                            └──────────────► Datadog

Receiver
   │
   └── tailpreviewexporter
             ║
             ║ WebSocket
             ║ OTLP protobuf
             ║
             ▼

        Tail Preview Server
        ┌──────────────────────────┐
        │ ● Collector connected    │
        │                          │
        │ Trace Buffer             │
        │ Tail Sampling Evaluator  │
        │ Statistics               │
        │ Session                  │
        └────────────┬─────────────┘
                     ║
                     ║ WebSocket
                     ▼

        ┌──────────────────────────────────────┐
        │ Tail Sampling Preview               │
        │                                      │
        │ ● LIVE                              │
        │                                      │
        │ KEEP payment  2.3s ERROR            │
        │ DROP api      182ms OK              │
        │ KEEP auth     1.4s ERROR            │
        │                                      │
        │ Sampling Rate                        │
        │ 4.21%                                │
        │                                      │
        │ Policy                               │
        │ status = ERROR                       │
        │ OR duration > 1000ms                 │
        │                                      │
        │ [Pause] [Export YAML]                │
        └──────────────────────────────────────┘
```

この構成では `tailpreviewexporter` は telemetry pipeline と Preview 環境の境界としてのみ振る舞い、Sampling のロジックはすべて Preview Server に閉じ込める。

これにより、実トラフィックを安全に Shadow しながら、

**Policy を変更 → 即座に再評価 → KEEP/DROP と影響を見る**

という Tail Sampling 専用のインタラクティブな検証環境を実現する。
