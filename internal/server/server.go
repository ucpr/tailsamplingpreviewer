// Package server implements the Preview Server described end to end in
// spec.md ss19: WebSocket ingest from Collectors, the Trace Assembler /
// Policy Engine / Statistics Engine / Ring Buffer, and a WebSocket + REST
// API for the Browser UI.
package server

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/zap"

	"github.com/ucpr/tailsamplingpreviewer/internal/sampling"
	"github.com/ucpr/tailsamplingpreviewer/internal/session"
	"github.com/ucpr/tailsamplingpreviewer/internal/statistics"
	"github.com/ucpr/tailsamplingpreviewer/internal/trace"
)

// Server is the Preview Server's business logic, independent of transport
// (spec.md ss19). HTTP/WebSocket handlers in collector_ws.go, browser_ws.go
// and api.go all operate on one shared *Server.
type Server struct {
	logger  *zap.Logger
	version string

	store      *trace.Store
	stats      *statistics.Engine
	sessionMgr *session.Manager

	mu        sync.RWMutex
	evaluator *sampling.Evaluator

	collectorsMu sync.Mutex
	collectors   map[*collectorConn]struct{}

	browsersMu sync.Mutex
	browsers   map[*browserConn]struct{}

	spansReceived  atomic.Uint64
	tracesReceived atomic.Uint64
}

// Config configures a new Server.
type Config struct {
	Version       string
	Store         trace.StoreConfig
	InitialPolicy sampling.Config
}

func DefaultPolicy() sampling.Config {
	return sampling.Config{
		DecisionWait: sampling.Duration(30 * time.Second),
		Policies: []sampling.PolicyCfg{
			{Name: "errors", Type: sampling.StatusCode, StatusCode: &sampling.StatusCodeCfg{StatusCodes: []string{"ERROR"}}},
		},
	}
}

func New(logger *zap.Logger, cfg Config) (*Server, error) {
	policy := cfg.InitialPolicy
	if policy.DecisionWait.AsDuration() <= 0 {
		policy = DefaultPolicy()
	}
	evaluator, err := sampling.NewEvaluator(policy)
	if err != nil {
		return nil, fmt.Errorf("initial policy: %w", err)
	}
	s := &Server{
		logger:     logger,
		version:    cfg.Version,
		store:      trace.NewStore(cfg.Store),
		stats:      statistics.NewEngine(),
		sessionMgr: session.NewManager(),
		evaluator:  evaluator,
		collectors: make(map[*collectorConn]struct{}),
		browsers:   make(map[*browserConn]struct{}),
	}
	s.sessionMgr.OnChange(s.onSessionChange)
	return s, nil
}

// Run starts the background decision loop (spec.md ss21) and blocks until
// ctx is cancelled.
func (s *Server) Run(ctx context.Context) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			s.decideDue(now)
		}
	}
}

func (s *Server) currentEvaluator() *sampling.Evaluator {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.evaluator
}

// decideDue evaluates every trace whose decision_wait has elapsed
// (spec.md ss21) and broadcasts the outcome.
func (s *Server) decideDue(now time.Time) {
	ev := s.currentEvaluator()
	decisionWait := ev.Config().DecisionWait.AsDuration()
	if decisionWait <= 0 {
		decisionWait = 30 * time.Second
	}

	due := s.store.DueForDecision(now, decisionWait)
	if len(due) == 0 {
		return
	}
	for _, t := range due {
		result := ev.Evaluate(&t, now)
		updated, ok := s.store.ApplyDecision(t.TraceID, result.Decision, result.MatchedNames())
		if !ok {
			continue
		}
		s.stats.Record(result)
		s.broadcastTraceDecided(updated)
	}
	s.broadcastStatistics()
}

// SetPolicy replaces the active Policy Engine configuration and
// immediately re-evaluates every retained trace (spec.md ss24). It returns
// an error, leaving the previous policy active, if cfg fails to compile
// (e.g. an invalid ottl_condition expression).
func (s *Server) SetPolicy(cfg sampling.Config) error {
	ev, err := sampling.NewEvaluator(cfg)
	if err != nil {
		return err
	}

	s.mu.Lock()
	s.evaluator = ev
	s.mu.Unlock()

	now := time.Now()
	s.stats.Reset()
	s.store.ForEachMutate(func(t *trace.Trace) {
		result := ev.Evaluate(t, now)
		t.Decision = result.Decision
		t.MatchedPolicies = result.MatchedNames()
		if t.State == trace.StateDecided {
			s.stats.Record(result)
		}
	})

	s.broadcastPolicyUpdated(cfg)
	s.broadcastStatistics()
	s.broadcastFullSnapshotToAllBrowsers()
	return nil
}

func (s *Server) Policy() sampling.Config {
	return s.currentEvaluator().Config()
}

func (s *Server) onSessionChange(snap session.Snapshot) {
	s.broadcastSessionState(snap)

	msg := sessionControlMessage(snap.State)
	if msg == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	s.collectorsMu.Lock()
	conns := make([]*collectorConn, 0, len(s.collectors))
	for c := range s.collectors {
		conns = append(conns, c)
	}
	s.collectorsMu.Unlock()
	for _, c := range conns {
		_ = c.sendJSON(ctx, msg)
	}
}

func sessionControlMessage(state session.State) any {
	switch state {
	case session.StateStreaming:
		return map[string]string{"type": "session.resume"}
	case session.StatePaused:
		return map[string]string{"type": "session.pause"}
	default:
		return map[string]string{"type": "session.pause"}
	}
}
