# OpenTelemetry Tail Sampling Preview Design Document v2

## 1. Overview

This project builds a tool for validating OpenTelemetry Collector Tail Sampling Policies in real time against real traffic, before applying them to production.

A dedicated `tailpreviewexporter` is added to the Collector, which maintains a persistent WebSocket connection to the Preview Server.

Traces received by the Collector are sent to the Preview Server as Shadow Traffic, in parallel with the normal send path to the Production Backend.

The Preview Server reassembles the traces and, for the case where a Tail Sampling Policy is applied, exposes:

* KEEP / DROP
* Matched Policy
* Sampling Rate
* Per-policy Match Rate
* Diff between the policy before and after a change

in real time, through a UI close to Datadog Live Tail.

The end goal is to provide an experience like

> Terraform `plan` for Tail Sampling Policies

---

# 2. Concept

This tool is not a Trace Backend.

The core experience is the following.

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

For that reason it is designed as a

**Tail Sampling Policy Debugger / Playground**

rather than an

**OpenTelemetry Trace Viewer**

---

# 3. Goals

## MVP

The MVP realizes the following.

* Collector traces can be sent to the Preview Server as Shadow Traffic
* The Collector and the Preview Server maintain a persistent WebSocket connection
* The Production Pipeline is unaffected even if the Preview Server stops
* Traces can be displayed in a real-time Live Tail
* Spans can be reassembled per Trace ID
* Tail Sampling Policies can be configured
* KEEP / DROP can be previewed in real time
* It is possible to see which policy a trace matched
* The Sampling Rate is displayed in real time
* OpenTelemetry Collector `tail_sampling` YAML can be imported / exported
* A Preview Session can be started and stopped

---

# 4. Non-Goals

The following are out of scope for the MVP.

* Replacing a Trace Backend
* Long-term trace storage
* Metrics / Logs Preview
* Replacing the production Tail Sampling Processor
* Automatically updating the production Collector configuration
* Automatic generation of Sampling Policies
* A distributed Preview Server
* Cost Prediction

---

# 5. Overall Architecture

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

Collector ↔ Preview Server and Preview Server ↔ Browser are separate connections.

---

# 6. Protocol Architecture

Communication is split into a Data Plane and a Control Plane.

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

The same WebSocket connection may be used for both, but the frame types must be clearly separated.

---

# 7. Collector → Preview Server Data Plane

Trace data uses the OTLP data model as-is.

No proprietary trace JSON format is introduced.

WebSocket Binary Frame:

```text
+-------------------------------+
| Frame Header                  |
+-------------------------------+
| ExportTraceServiceRequest     |
| protobuf                      |
+-------------------------------+
```

Logical payload:

```protobuf
ExportTraceServiceRequest {
    resource_spans: [...]
}
```

This keeps the conversion as simple as:

```text
Collector pdata
      ↓
OTLP protobuf
      ↓
WebSocket
      ↓
Preview Server pdata
```

---

# 8. Control Plane

Taking advantage of the bidirectional nature of WebSocket, the Preview Server can control the exporter.

However, evaluation of Sampling Policies themselves happens on the Preview Server.

The exporter is dedicated to data transport.

Example control messages:

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

One of the main reasons for adopting WebSocket is to introduce the concept of a Session.

States:

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

The exporter is kept as thin as possible.

Responsibilities:

```text
pdata.Traces
    ↓
serialization
    ↓
bounded queue
    ↓
WebSocket writer
```

What it does not do:

* Trace Assembly
* Tail Sampling Evaluation
* Filter Evaluation
* Statistics
* Query
* Storage

These are all concentrated in the Preview Server.

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

Writes to the WebSocket are funneled through a single goroutine.

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

        // Preview is best effort.
        // Never apply backpressure to the production pipeline.
        return nil
    }
}
```

---

# 13. Backpressure Policy

This is the single most important design principle of the tool.

```text
Preview overloaded
        │
        ▼
Preview traces dropped

NOT

Production blocked
```

Preview data is dropped in the following cases.

* Queue full
* Preview Server unreachable
* WebSocket write timeout
* Serialization backlog
* Preview Session paused

No error is ever returned to the production pipeline.

---

# 14. Connection Manager

The exporter contains a WebSocket Connection Manager.

Responsibilities:

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

Even if the Preview Server restarts, the Collector itself keeps running normally.

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

Traces are never persisted while reconnecting.

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

The Preview Server surfaces the health of each Collector connection in the UI.

---

# 17. Collector Identity

Collector metadata is sent when the connection is established.

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

# 18. Multiple Collectors

The Preview Server accepts connections from multiple Collectors.

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

However, for Tail Sampling correctness it must be guaranteed that

```text
same trace_id
    ↓
same Preview Server
```

In the MVP this is solved by keeping the Preview Server a single instance.

---

# 19. Preview Server

The Preview Server is the heart of the system.

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

Spans are aggregated per Trace ID.

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

Because a distributed trace has no explicit completion signal, decisions are made on a time basis, the same way the Tail Sampling Processor does.

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

Initial value:

```yaml
decision_wait: 30s
```

---

# 22. Separating Live from Decision

The UI distinguishes the state of each trace.

```text
● LIVE
✓ KEEP
× DROP
```

Example:

```text
LIVE  payment-api  1.3s  8 spans

KEEP  auth-api     2.3s  ERROR

DROP  catalog      82ms   OK
```

For traces whose decision_wait has not yet elapsed, KEEP/DROP is never asserted.

---

# 23. Policy Engine

Tail Sampling Policies are evaluated inside the Preview Server.

The important point is that

**the Collector exporter never evaluates policies**

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

This allows the same trace set to be re-evaluated any number of times against multiple policies.

---

# 24. Policy Update

The user changes a policy from the UI.

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

No configuration change occurs on the Collector.

The traces retained inside the Preview Server are re-evaluated immediately.

```text
Policy changed
      │
      ▼
re-evaluate ring buffer
      │
      ▼
new statistics
```

This is the key UX of the tool.

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

The results are always shown in the lower right.

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

The basic unit of display is the trace.

```text
18:41:31  KEEP  payment-api   2.3s   ERROR  18 spans
18:41:31  DROP  api           182ms  OK      4 spans
18:41:30  KEEP  auth          1.8s   ERROR   8 spans
```

The goal is a feel close to Datadog Live Tail.

---

# 27. Live Tail Query

Separate from the Sampling Policy, the UI has its own filter.

```text
service.name:payment
```

```text
status:error duration:>1s
```

```text
resource.deployment.environment:production
```

The filter only controls what is displayed.

It has no effect on the sampling decision.

---

# 28. Trace Detail

When a trace is selected:

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

Key metrics to display:

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

In the future, the same trace set should be evaluable against multiple candidates.

```text
              Current      Candidate

Sampling       4.21%         6.82%

KEEP          42,100        68,200

DROP         957,900       931,800
```

And further:

```text
Newly Kept
+26,100

Newly Dropped
312

ERROR traces newly dropped
12
```

should be displayed as well.

---

# 31. Storage

The MVP is in-memory only.

```text
Ring Buffer

max traces
max duration
max bytes
```

Configuration:

```yaml
storage:
  max_traces: 10000
  max_age: 5m
  max_memory: 512MiB
```

Eviction starts from the oldest trace.

---

# 32. Pause

When a Preview Session is paused, the Collector stops sending new traces.

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

Nothing accumulates in the queue.

This prevents buffering huge numbers of traces.

---

# 33. Browser Communication

Preview Server → Browser also uses WebSocket.

```text
Preview Server
      ║
      ║ UI events
      ▼
Browser
```

Example events:

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

There is no need to send OTLP protobuf to the browser as-is.

Only the projection the UI needs is sent.

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

If a remote mode is added, the following become mandatory:

* TLS
* authentication
* origin validation
* connection authorization

---

# 35. Sensitive Data

Traces may contain:

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

In the future, a preview-only transform/redaction processor can be inserted on the Collector side.

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

Startup:

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

After Start in the UI:

```text
Preview started.

Receiving traces...

Traces        12,821
Spans        182,811
Trace rate     2,183/s
```

---

# 39. MVP Development Order

## Phase 0: WebSocket Transport

First, validate only the Collector → Preview Server transport.

```text
Collector
    │
    │ WS + protobuf
    ▼
Preview Server
```

Implementation:

* custom exporter
* WebSocket connect
* protobuf serialization
* bounded queue
* reconnect
* counter

No UI required.

---

## Phase 1: Live Tail

Adds:

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

This is where it becomes a working Live Tail.

---

## Phase 2: Sampling Preview

Adds:

* Tail Sampling Evaluator
* KEEP / DROP
* Matched Policy
* Sampling Rate
* YAML import

This is the first product MVP.

---

## Phase 3: Policy Builder

Adds:

* Visual Policy Builder
* YAML export
* re-evaluation
* Policy statistics

---

## Phase 4: Policy Compare

Adds:

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

* [ ] `tailpreviewexporter` can establish a WebSocket connection
* [ ] OTLP traces can be sent as protobuf binary frames
* [ ] The exporter can reconnect after the Preview Server restarts
* [ ] The production pipeline keeps working while the Preview Server is down
* [ ] On queue overflow, only preview data is dropped
* [ ] A Preview Session can be started / paused / stopped
* [ ] Traces can be displayed in the Live Tail UI
* [ ] Spans can be aggregated per Trace ID
* [ ] Tail Sampling Policies can be loaded
* [ ] KEEP / DROP can be previewed
* [ ] Matched policies can be inspected
* [ ] The Sampling Rate can be inspected
* [ ] The existing ring buffer is re-evaluated when the policy changes
* [ ] The Preview Server never changes the production Collector configuration
* [ ] Traces are never persisted

---

# 41. Design Principles

## Production First

Preview failures, latency, and outages must never propagate to the production pipeline.

---

## Best Effort

Preview traces may be lost.

Drop data rather than block the Collector.

---

## Separate Transport from Evaluation

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

---

## Full Shadow Stream

The exporter never filters by Sampling Policy.

```text
full trace stream
       ↓
Preview Server
       ↓
arbitrary policy
```

This is what makes policies freely changeable and comparable.

---

## Native OTel Data Model

No proprietary schema is created for trace data.

Collector → Server uses OTLP protobuf.

---

## Interactive Debugger

WebSocket is not adopted merely for connection reuse.

It is adopted to provide the following UX:

```text
connection
session
pause/resume
live updates
collector identity
control messages
```

---

# 42. Final Picture

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

In this configuration, `tailpreviewexporter` acts only as the boundary between the telemetry pipeline and the preview environment, and all sampling logic stays inside the Preview Server.

This safely shadows real traffic while providing an interactive validation environment dedicated to Tail Sampling:

**change a policy → re-evaluate instantly → see KEEP/DROP and its impact**
