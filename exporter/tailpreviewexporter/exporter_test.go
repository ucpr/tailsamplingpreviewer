package tailpreviewexporter

import (
	"context"
	"testing"
	"time"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/exporter"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.uber.org/zap"
)

func testSettings() exporter.Settings {
	return exporter.Settings{
		ID:                component.NewID(componentType),
		TelemetrySettings: component.TelemetrySettings{Logger: zap.NewNop()},
		BuildInfo:         component.BuildInfo{Version: "test"},
	}
}

func oneSpanTrace() ptrace.Traces {
	td := ptrace.NewTraces()
	rs := td.ResourceSpans().AppendEmpty()
	rs.ScopeSpans().AppendEmpty().Spans().AppendEmpty().SetName("x")
	return td
}

// TestConsumeTraces_NeverReturnsError is the single most important
// contract in spec.md ss13: Preview data loss must never propagate as
// pipeline backpressure or an error to the Production pipeline.
func TestConsumeTraces_NeverReturnsError(t *testing.T) {
	cfg := &Config{
		Endpoint:  "ws://127.0.0.1:1/unreachable",
		Queue:     QueueConfig{Enabled: true, Size: 1},
		Reconnect: ReconnectConfig{Enabled: false},
	}
	exp := newExporter(cfg, testSettings())
	// Deliberately do not start the writer goroutine / connection manager,
	// so the queue can never drain: this forces the overflow path.

	td := oneSpanTrace()
	for i := 0; i < 5; i++ {
		if err := exp.consumeTraces(context.Background(), td); err != nil {
			t.Fatalf("consumeTraces must never return an error, got: %v", err)
		}
	}

	snap := exp.Metrics()
	if snap.SpansDroppedTotal == 0 {
		t.Fatalf("expected some drops once the bounded queue filled up, got %+v", snap)
	}
	if len(exp.queue) > cap(exp.queue) {
		t.Fatalf("queue must never exceed its configured bound: len=%d cap=%d", len(exp.queue), cap(exp.queue))
	}
}

// TestConsumeTraces_DropsWhilePaused verifies spec.md ss32: while the
// Preview Session is paused, new spans must be dropped rather than
// queued, so pausing never causes a burst of buffered traffic later.
func TestConsumeTraces_DropsWhilePaused(t *testing.T) {
	cfg := &Config{
		Endpoint:  "ws://127.0.0.1:1/unreachable",
		Queue:     QueueConfig{Enabled: true, Size: 10},
		Reconnect: ReconnectConfig{Enabled: false},
	}
	exp := newExporter(cfg, testSettings())
	exp.conn.paused.Store(true)

	if err := exp.consumeTraces(context.Background(), oneSpanTrace()); err != nil {
		t.Fatalf("consumeTraces must never return an error, got: %v", err)
	}
	if len(exp.queue) != 0 {
		t.Fatalf("paused exporter must not enqueue anything, queue has %d items", len(exp.queue))
	}
	if exp.Metrics().SpansDroppedTotal != 1 {
		t.Fatalf("expected exactly one drop, got %+v", exp.Metrics())
	}
}

func TestConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     Config
		wantErr bool
	}{
		{"valid", Config{Endpoint: "ws://x", Queue: QueueConfig{Enabled: true, Size: 1}, Reconnect: ReconnectConfig{Enabled: true, Interval: time.Second}}, false},
		{"empty endpoint", Config{Queue: QueueConfig{Enabled: true, Size: 1}}, true},
		{"zero queue size while enabled", Config{Endpoint: "ws://x", Queue: QueueConfig{Enabled: true, Size: 0}}, true},
		{"zero reconnect interval while enabled", Config{Endpoint: "ws://x", Reconnect: ReconnectConfig{Enabled: true, Interval: 0}}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Fatalf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
