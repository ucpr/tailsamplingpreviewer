package tailpreviewexporter

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/coder/websocket"
)

// captureServer records every binary Data Plane frame it receives, so the
// full Start -> ConsumeTraces -> WebSocket write -> Shutdown lifecycle can
// be verified end to end without mocking any collaborator.
type captureServer struct {
	reg    *wsRegistry
	framed chan []byte
}

func newCaptureServer(t *testing.T) (*captureServer, string) {
	t.Helper()
	cs := &captureServer{reg: &wsRegistry{}, framed: make(chan []byte, 8)}
	srv, addr := listenOn(t, "127.0.0.1:0", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true})
		if err != nil {
			return
		}
		cs.reg.add(conn)
		for {
			msgType, data, err := conn.Read(r.Context())
			if err != nil {
				return
			}
			if msgType == websocket.MessageBinary {
				cs.framed <- data
			}
		}
	}))
	t.Cleanup(func() {
		cs.reg.closeAll()
		_ = srv.Close()
	})
	return cs, addr
}

// TestExporter_StartConsumeShutdown drives the exporter through its full
// component lifecycle (spec.md ss11-ss12: ConsumeTraces -> serialize ->
// bounded queue -> single writer goroutine -> WebSocket) against a real
// local WebSocket server, then verifies Shutdown stops everything cleanly.
func TestExporter_StartConsumeShutdown(t *testing.T) {
	cs, addr := newCaptureServer(t)

	cfg := &Config{
		Endpoint:  "ws://" + addr + "/v1/collector",
		Queue:     QueueConfig{Enabled: true, Size: 10},
		Reconnect: ReconnectConfig{Enabled: false},
	}
	exp := newExporter(cfg, testSettings())

	if err := exp.start(context.Background(), nil); err != nil {
		t.Fatalf("start: %v", err)
	}

	waitFor(t, "connected", func() bool { return exp.conn.Connected() })

	if err := exp.consumeTraces(context.Background(), oneSpanTrace()); err != nil {
		t.Fatalf("consumeTraces must never return an error, got: %v", err)
	}

	select {
	case frame := <-cs.framed:
		if len(frame) == 0 {
			t.Fatal("received an empty frame")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for the exporter to deliver a frame")
	}

	waitFor(t, "spansSentTotal to reflect the delivery", func() bool {
		return exp.Metrics().SpansSentTotal == 1
	})

	if err := exp.shutdown(context.Background()); err != nil {
		t.Fatalf("shutdown: %v", err)
	}

	// consumeTraces after shutdown must still never error, and the payload
	// is simply dropped since nothing drains the queue anymore.
	if err := exp.consumeTraces(context.Background(), oneSpanTrace()); err != nil {
		t.Fatalf("consumeTraces after shutdown must never return an error, got: %v", err)
	}
}
