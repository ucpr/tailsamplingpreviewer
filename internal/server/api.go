package server

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"sort"

	"go.opentelemetry.io/collector/pdata/pcommon"

	"github.com/ucpr/tailsamplingpreviewer/internal/query"
	"github.com/ucpr/tailsamplingpreviewer/internal/sampling"
	"github.com/ucpr/tailsamplingpreviewer/internal/session"
)

var errInvalidTraceID = errors.New("invalid trace id")

// Router wires every HTTP/WebSocket endpoint the Browser UI and Collector
// exporter talk to (spec.md ss19, ss33, ss34).
func (s *Server) Router() *http.ServeMux {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	mux.HandleFunc("GET /v1/collector", s.HandleCollectorWS)
	mux.HandleFunc("GET /v1/browser", s.HandleBrowserWS)

	mux.HandleFunc("GET /api/session", s.handleGetSession)
	mux.HandleFunc("POST /api/session/start", s.handleSessionAction(s.sessionMgr.Start))
	mux.HandleFunc("POST /api/session/pause", s.handleSessionAction(s.sessionMgr.Pause))
	mux.HandleFunc("POST /api/session/resume", s.handleSessionAction(s.sessionMgr.Resume))
	mux.HandleFunc("POST /api/session/stop", s.handleSessionAction(s.sessionMgr.Stop))

	mux.HandleFunc("GET /api/statistics", s.handleGetStatistics)
	mux.HandleFunc("GET /api/collectors", s.handleGetCollectors)

	mux.HandleFunc("GET /api/policy", s.handleGetPolicy)
	mux.HandleFunc("PUT /api/policy", s.handlePutPolicy)
	mux.HandleFunc("GET /api/policy/yaml", s.handleGetPolicyYAML)
	mux.HandleFunc("PUT /api/policy/yaml", s.handlePutPolicyYAML)
	mux.HandleFunc("POST /api/policy/compare", s.handleComparePolicy)

	mux.HandleFunc("GET /api/traces", s.handleListTraces)
	mux.HandleFunc("GET /api/traces/{id}", s.handleGetTrace)

	return mux
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func (s *Server) handleGetSession(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, sessionSummary(s.sessionMgr.Snapshot(), s.spansReceived.Load(), s.tracesReceived.Load()))
}

func (s *Server) handleSessionAction(action func() (session.Snapshot, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		snap, err := action()
		if err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		writeJSON(w, http.StatusOK, sessionSummary(snap, s.spansReceived.Load(), s.tracesReceived.Load()))
	}
}

func (s *Server) handleGetStatistics(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, s.stats.Snapshot())
}

func (s *Server) handleGetCollectors(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, s.collectorSummaries())
}

func (s *Server) handleGetPolicy(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, s.Policy())
}

func (s *Server) handlePutPolicy(w http.ResponseWriter, r *http.Request) {
	var cfg sampling.Config
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := s.SetPolicy(cfg); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusOK, s.Policy())
}

func (s *Server) handleGetPolicyYAML(w http.ResponseWriter, _ *http.Request) {
	out, err := sampling.DumpYAML(s.Policy())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/yaml; charset=utf-8")
	_, _ = w.Write(out)
}

func (s *Server) handlePutPolicyYAML(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	cfg, err := sampling.ParseYAML(body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := s.SetPolicy(cfg); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusOK, s.Policy())
}

// handleComparePolicy previews candidate against every currently-decided
// trace without applying it (spec.md ss30 "Policy Compare") -- unlike
// handlePutPolicy/handlePutPolicyYAML, it never calls SetPolicy, so the
// active policy, store and statistics are all left untouched.
func (s *Server) handleComparePolicy(w http.ResponseWriter, r *http.Request) {
	var cfg sampling.Config
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	result, err := s.comparePolicy(cfg)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleListTraces(w http.ResponseWriter, r *http.Request) {
	f := query.Parse(r.URL.Query().Get("q"))
	all := s.store.All()

	out := make([]TraceSummary, 0, len(all))
	for i := range all {
		if !f.Empty() && !f.Match(&all[i]) {
			continue
		}
		out = append(out, toSummary(&all[i]))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].LastSeen.After(out[j].LastSeen) })
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleGetTrace(w http.ResponseWriter, r *http.Request) {
	idHex := r.PathValue("id")
	id, err := parseTraceID(idHex)
	if err != nil {
		http.Error(w, "invalid trace id", http.StatusBadRequest)
		return
	}
	t, ok := s.store.Get(id)
	if !ok {
		http.NotFound(w, r)
		return
	}
	writeJSON(w, http.StatusOK, toDetail(&t))
}

func parseTraceID(hexStr string) (pcommon.TraceID, error) {
	var id pcommon.TraceID
	b, err := hex.DecodeString(hexStr)
	if err != nil || len(b) != len(id) {
		return pcommon.TraceID{}, errInvalidTraceID
	}
	copy(id[:], b)
	return id, nil
}
