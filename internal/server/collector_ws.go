package server

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"
	"go.uber.org/zap"

	"go.opentelemetry.io/collector/pdata/ptrace/ptraceotlp"

	"github.com/ucpr/tailsamplingpreviewer/internal/protocol"
	itrace "github.com/ucpr/tailsamplingpreviewer/internal/trace"
)

// collectorConn is one connected OpenTelemetry Collector
// (tailpreviewexporter), spec.md ss17-18.
type collectorConn struct {
	id      string
	version string

	connectedAt time.Time
	lastSeen    atomic.Int64 // unix nanos
	spansSent   atomic.Uint64

	writeMu sync.Mutex
	conn    *websocket.Conn
}

func (c *collectorConn) sendJSON(ctx context.Context, v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	return c.conn.Write(ctx, websocket.MessageText, data)
}

func (c *collectorConn) touch() { c.lastSeen.Store(time.Now().UnixNano()) }

func (c *collectorConn) summary() CollectorSummary {
	return CollectorSummary{
		ID:        c.id,
		Version:   c.version,
		Connected: true,
		SpansSent: c.spansSent.Load(),
		LastSeen:  time.Unix(0, c.lastSeen.Load()),
	}
}

// HandleCollectorWS upgrades and serves one Collector connection
// (spec.md ss7, ss11). Path: /v1/collector.
func (s *Server) HandleCollectorWS(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true})
	if err != nil {
		return
	}
	ctx := r.Context()
	defer conn.CloseNow() //nolint:errcheck

	helloCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	msgType, data, err := conn.Read(helloCtx)
	cancel()
	if err != nil || msgType != websocket.MessageText {
		_ = conn.Close(websocket.StatusPolicyViolation, "expected hello")
		return
	}
	var hello protocol.Hello
	if err := json.Unmarshal(data, &hello); err != nil || hello.Type != protocol.TypeHello {
		_ = conn.Close(websocket.StatusPolicyViolation, "expected hello")
		return
	}

	cc := &collectorConn{
		id:          hello.Collector.ID,
		version:     hello.Collector.Version,
		connectedAt: time.Now(),
		conn:        conn,
	}
	cc.touch()
	s.registerCollector(cc)
	defer s.unregisterCollector(cc)

	if err := cc.sendJSON(ctx, protocol.NewHelloAck(protocol.ServerInfo{Version: s.version})); err != nil {
		return
	}
	// Bring a Collector that connects mid-session up to speed immediately.
	_ = cc.sendJSON(ctx, sessionControlMessage(s.sessionMgr.Snapshot().State))

	for {
		msgType, data, err := conn.Read(ctx)
		if err != nil {
			return
		}
		cc.touch()
		switch msgType {
		case websocket.MessageBinary:
			s.handleTraceFrame(cc, data)
		case websocket.MessageText:
			s.handleCollectorControl(ctx, cc, data)
		}
	}
}

func (s *Server) registerCollector(cc *collectorConn) {
	s.collectorsMu.Lock()
	s.collectors[cc] = struct{}{}
	s.collectorsMu.Unlock()
	s.broadcast(collectorConnectedMsg{Type: evtCollectorConnected, Collector: cc.summary()})
}

func (s *Server) unregisterCollector(cc *collectorConn) {
	s.collectorsMu.Lock()
	delete(s.collectors, cc)
	s.collectorsMu.Unlock()
	s.broadcast(collectorDisconnectedMsg{Type: evtCollectorDisconnect, CollectorID: cc.id})
}

func (s *Server) collectorSummaries() []CollectorSummary {
	s.collectorsMu.Lock()
	defer s.collectorsMu.Unlock()
	out := make([]CollectorSummary, 0, len(s.collectors))
	for c := range s.collectors {
		out = append(out, c.summary())
	}
	return out
}

func (s *Server) handleCollectorControl(ctx context.Context, cc *collectorConn, data []byte) {
	env, err := decodeEnvelope(data)
	if err != nil {
		return
	}
	switch env.Type {
	case protocol.TypePing:
		var p protocol.Ping
		if json.Unmarshal(data, &p) == nil {
			_ = cc.sendJSON(ctx, protocol.NewPong(p.Timestamp))
		}
	case protocol.TypePong:
		// heartbeat acknowledged; touch() above already recorded liveness.
	}
}

// handleTraceFrame decodes a Data Plane binary frame and folds its spans
// into the Trace Store (spec.md ss7, ss20).
func (s *Server) handleTraceFrame(cc *collectorConn, raw []byte) {
	header, payload, ok := protocol.DecodeFrame(raw)
	if !ok || header.Type != protocol.FrameTypeTraces {
		return
	}

	req := ptraceotlp.NewExportRequest()
	if err := req.UnmarshalProto(payload); err != nil {
		s.logger.Debug("tailpreview: malformed trace frame", zap.String("collector", cc.id), zap.Error(err))
		return
	}
	td := req.Traces()

	spansByTrace := itrace.Assemble(td)
	spanCount := 0
	for _, spans := range spansByTrace {
		spanCount += len(spans)
	}
	cc.spansSent.Add(uint64(spanCount))
	s.spansReceived.Add(uint64(spanCount))

	now := time.Now()
	touched := s.store.Ingest(now, spansByTrace)
	for _, t := range touched {
		if t.FirstSeen.Equal(now) {
			// This Ingest call created the trace: its first sighting.
			s.tracesReceived.Add(1)
		}
		s.broadcastTraceUpdated(t)
	}
}
