package server

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/coder/websocket"
	"go.uber.org/zap"

	"github.com/ucpr/tailsamplingpreviewer/internal/sampling"
	"github.com/ucpr/tailsamplingpreviewer/internal/session"
	"github.com/ucpr/tailsamplingpreviewer/internal/trace"
)

// browserConn is one connected Browser UI (spec.md ss33).
type browserConn struct {
	writeMu sync.Mutex
	conn    *websocket.Conn
}

func (b *browserConn) sendJSON(ctx context.Context, v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	b.writeMu.Lock()
	defer b.writeMu.Unlock()
	return b.conn.Write(ctx, websocket.MessageText, data)
}

// HandleBrowserWS upgrades and serves one Browser UI connection
// (spec.md ss33). Path: /v1/browser.
func (s *Server) HandleBrowserWS(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true})
	if err != nil {
		return
	}
	ctx := r.Context()
	defer conn.CloseNow() //nolint:errcheck

	bc := &browserConn{conn: conn}
	s.registerBrowser(bc)
	defer s.unregisterBrowser(bc)

	if err := bc.sendJSON(ctx, s.snapshot()); err != nil {
		return
	}

	for {
		msgType, data, err := conn.Read(ctx)
		if err != nil {
			return
		}
		if msgType != websocket.MessageText {
			continue
		}
		s.handleBrowserControl(ctx, bc, data)
	}
}

func (s *Server) registerBrowser(bc *browserConn) {
	s.browsersMu.Lock()
	defer s.browsersMu.Unlock()
	s.browsers[bc] = struct{}{}
}

func (s *Server) unregisterBrowser(bc *browserConn) {
	s.browsersMu.Lock()
	defer s.browsersMu.Unlock()
	delete(s.browsers, bc)
}

func (s *Server) handleBrowserControl(ctx context.Context, bc *browserConn, data []byte) {
	env, err := decodeEnvelope(data)
	if err != nil {
		return
	}
	switch env.Type {
	case cmdSessionStart:
		if _, err := s.sessionMgr.Start(); err != nil {
			_ = bc.sendJSON(ctx, errorMsg{Type: evtError, Message: err.Error()})
		}
	case cmdSessionPause:
		if _, err := s.sessionMgr.Pause(); err != nil {
			_ = bc.sendJSON(ctx, errorMsg{Type: evtError, Message: err.Error()})
		}
	case cmdSessionResume:
		if _, err := s.sessionMgr.Resume(); err != nil {
			_ = bc.sendJSON(ctx, errorMsg{Type: evtError, Message: err.Error()})
		}
	case cmdSessionStop:
		if _, err := s.sessionMgr.Stop(); err != nil {
			_ = bc.sendJSON(ctx, errorMsg{Type: evtError, Message: err.Error()})
		}
	}
}

func decodeEnvelope(data []byte) (envelope, error) {
	var e envelope
	err := json.Unmarshal(data, &e)
	return e, err
}

// snapshot builds the initial state sent to a newly connected Browser.
func (s *Server) snapshot() snapshotMsg {
	traces := s.store.All()
	summaries := make([]TraceSummary, 0, len(traces))
	for i := range traces {
		summaries = append(summaries, toSummary(&traces[i]))
	}
	return snapshotMsg{
		Type:       evtSnapshot,
		Session:    sessionSummary(s.sessionMgr.Snapshot(), s.spansReceived.Load(), s.tracesReceived.Load()),
		Collectors: s.collectorSummaries(),
		Traces:     summaries,
		Policy:     s.Policy(),
		Statistics: s.stats.Snapshot(),
	}
}

func (s *Server) broadcast(v any) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	s.browsersMu.Lock()
	conns := make([]*browserConn, 0, len(s.browsers))
	for c := range s.browsers {
		conns = append(conns, c)
	}
	s.browsersMu.Unlock()

	for _, c := range conns {
		if err := c.sendJSON(ctx, v); err != nil {
			s.logger.Debug("tailpreview: browser send failed", zap.Error(err))
		}
	}
}

func (s *Server) broadcastTraceUpdated(t trace.Trace) {
	s.broadcast(traceUpdatedMsg{Type: evtTraceUpdated, Trace: toSummary(&t)})
}

func (s *Server) broadcastTraceDecided(t trace.Trace) {
	s.broadcast(traceDecidedMsg{
		Type:            evtTraceDecided,
		TraceID:         t.TraceID.String(),
		Decision:        decisionLabel(t.Decision),
		MatchedPolicies: append([]string(nil), t.MatchedPolicies...),
	})
}

func (s *Server) broadcastStatistics() {
	s.broadcast(statisticsUpdatedMsg{Type: evtStatisticsUpdated, Statistics: s.stats.Snapshot()})
}

func (s *Server) broadcastSessionState(snap session.Snapshot) {
	s.broadcast(sessionStateMsg{
		Type:    evtSessionState,
		Session: sessionSummary(snap, s.spansReceived.Load(), s.tracesReceived.Load()),
	})
}

func (s *Server) broadcastPolicyUpdated(cfg sampling.Config) {
	s.broadcast(policyUpdatedMsg{Type: evtPolicyUpdated, Policy: cfg})
}

func (s *Server) broadcastFullSnapshotToAllBrowsers() {
	s.broadcast(s.snapshot())
}
