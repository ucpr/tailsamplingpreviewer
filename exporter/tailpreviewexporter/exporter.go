// Package tailpreviewexporter is a Collector exporter that ships a copy of
// every trace it sees to a Tail Sampling Preview Server over WebSocket, in
// parallel with the normal Production exporters (spec.md ss11).
//
// It deliberately does as little as possible: pdata -> protobuf ->
// bounded queue -> WebSocket writer. Trace assembly, sampling evaluation,
// filtering and storage all happen in the Preview Server, never here
// (spec.md ss11, ss41). If the queue is full, the connection is down, or
// the Preview Session is paused, spans are silently dropped: the
// Production pipeline must never see an error or backpressure from this
// exporter (spec.md ss13).
package tailpreviewexporter // import "github.com/ucpr/tailsamplingpreviewer/exporter/tailpreviewexporter"

import (
	"context"
	"os"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/exporter"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.uber.org/zap"
)

// Exporter implements the pdata.Traces -> serialize -> bounded queue ->
// WebSocket writer pipeline described in spec.md ss12.
type Exporter struct {
	cfg     *Config
	logger  *zap.Logger
	metrics *metrics
	conn    *connectionManager

	queue  chan []byte
	cancel context.CancelFunc
}

func newExporter(cfg *Config, set exporter.Settings) *Exporter {
	collectorID := cfg.CollectorID
	if collectorID == "" {
		if h, err := os.Hostname(); err == nil {
			collectorID = h
		} else {
			collectorID = "unknown"
		}
	}

	m := &metrics{}
	queueSize := cfg.Queue.Size
	if !cfg.Queue.Enabled || queueSize <= 0 {
		queueSize = 1000
	}

	return &Exporter{
		cfg:     cfg,
		logger:  set.Logger,
		metrics: m,
		conn:    newConnectionManager(cfg, set.Logger, collectorID, set.BuildInfo.Version, m),
		queue:   make(chan []byte, queueSize),
	}
}

// Metrics returns a point-in-time read of the exporter's internal
// telemetry counters (spec.md ss36).
func (e *Exporter) Metrics() MetricsSnapshot {
	return e.metrics.snapshot(e.conn.Connected())
}

func (e *Exporter) start(_ context.Context, _ component.Host) error {
	runCtx, cancel := context.WithCancel(context.Background())
	e.cancel = cancel

	go e.conn.Run(runCtx)
	go e.writerLoop(runCtx)
	return nil
}

func (e *Exporter) shutdown(_ context.Context) error {
	if e.cancel != nil {
		e.cancel()
	}
	return nil
}

func (e *Exporter) writerLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case payload := <-e.queue:
			e.metrics.queueSize.Add(-1)
			if err := e.conn.SendBinary(ctx, payload); err != nil {
				e.metrics.writeErrorsTotal.Add(1)
				e.metrics.spansDroppedTotal.Add(1)
				continue
			}
			e.metrics.spansSentTotal.Add(1)
		}
	}
}

// consumeTraces is the pipeline entry point. It never returns a non-nil
// error: Preview data loss must never propagate as pipeline backpressure
// or a retry (spec.md ss12, ss13).
func (e *Exporter) consumeTraces(_ context.Context, td ptrace.Traces) error {
	if e.conn.Paused() {
		e.metrics.spansDroppedTotal.Add(1)
		return nil
	}

	payload, err := marshal(td)
	if err != nil {
		e.logger.Debug("tailpreview: marshal failed", zap.Error(err))
		e.metrics.spansDroppedTotal.Add(1)
		return nil
	}

	select {
	case e.queue <- payload:
		e.metrics.queueSize.Add(1)
	default:
		e.metrics.spansDroppedTotal.Add(1)
	}
	return nil
}

// tracesExporter adapts Exporter to the exporter.Traces interface
// (component.Component + consumer.Traces), spec.md ss11.
type tracesExporter struct {
	consumer.Traces
	exp *Exporter
}

func (t *tracesExporter) Start(ctx context.Context, host component.Host) error {
	return t.exp.start(ctx, host)
}

func (t *tracesExporter) Shutdown(ctx context.Context) error {
	return t.exp.shutdown(ctx)
}
