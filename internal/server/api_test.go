package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.uber.org/zap"

	"github.com/ucpr/tailsamplingpreviewer/internal/sampling"
	itrace "github.com/ucpr/tailsamplingpreviewer/internal/trace"
)

func newTestServer(t *testing.T) *Server {
	t.Helper()
	srv, err := New(zap.NewNop(), Config{Version: "test"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return srv
}

func doRequest(srv *Server, method, path string, body string) *httptest.ResponseRecorder {
	var r *http.Request
	if body != "" {
		r = httptest.NewRequest(method, path, strings.NewReader(body))
	} else {
		r = httptest.NewRequest(method, path, nil)
	}
	rec := httptest.NewRecorder()
	srv.Router().ServeHTTP(rec, r)
	return rec
}

func TestAPI_SimpleGETs(t *testing.T) {
	srv := newTestServer(t)
	cc := &collectorConn{id: "collector-1", version: "v1"}
	cc.touch()
	srv.registerCollector(cc)

	tests := []struct {
		name       string
		path       string
		wantStatus int
		wantCT     string // content-type prefix, "" = don't check
		checkBody  func(t *testing.T, body []byte)
	}{
		{
			name:       "healthz",
			path:       "/healthz",
			wantStatus: http.StatusOK,
			checkBody: func(t *testing.T, body []byte) {
				if string(body) != "ok" {
					t.Fatalf("body = %q, want %q", body, "ok")
				}
			},
		},
		{
			name:       "session",
			path:       "/api/session",
			wantStatus: http.StatusOK,
			wantCT:     "application/json",
			checkBody: func(t *testing.T, body []byte) {
				var got SessionSummary
				mustUnmarshal(t, body, &got)
				if got.State != "IDLE" {
					t.Fatalf("state = %q, want IDLE", got.State)
				}
			},
		},
		{
			name:       "statistics",
			path:       "/api/statistics",
			wantStatus: http.StatusOK,
			wantCT:     "application/json",
			checkBody: func(t *testing.T, body []byte) {
				var got StatisticsView
				mustUnmarshal(t, body, &got)
				if got.Observed != 0 {
					t.Fatalf("observed = %d, want 0", got.Observed)
				}
			},
		},
		{
			name:       "collectors",
			path:       "/api/collectors",
			wantStatus: http.StatusOK,
			wantCT:     "application/json",
			checkBody: func(t *testing.T, body []byte) {
				var got []CollectorSummary
				mustUnmarshal(t, body, &got)
				if len(got) != 1 || got[0].ID != "collector-1" {
					t.Fatalf("collectors = %+v, want one entry with ID collector-1", got)
				}
			},
		},
		{
			name:       "policy json",
			path:       "/api/policy",
			wantStatus: http.StatusOK,
			wantCT:     "application/json",
			checkBody: func(t *testing.T, body []byte) {
				var got sampling.Config
				mustUnmarshal(t, body, &got)
				if len(got.Policies) == 0 {
					t.Fatalf("expected a non-empty default policy set, got %+v", got)
				}
			},
		},
		{
			name:       "policy yaml",
			path:       "/api/policy/yaml",
			wantStatus: http.StatusOK,
			wantCT:     "text/yaml",
			checkBody: func(t *testing.T, body []byte) {
				if !strings.Contains(string(body), "tail_sampling:") {
					t.Fatalf("expected tail_sampling YAML, got:\n%s", body)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := doRequest(srv, http.MethodGet, tt.path, "")
			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d (body: %s)", rec.Code, tt.wantStatus, rec.Body.String())
			}
			if tt.wantCT != "" && !strings.HasPrefix(rec.Header().Get("Content-Type"), tt.wantCT) {
				t.Fatalf("content-type = %q, want prefix %q", rec.Header().Get("Content-Type"), tt.wantCT)
			}
			if tt.checkBody != nil {
				tt.checkBody(t, rec.Body.Bytes())
			}
		})
	}
}

func TestAPI_PutPolicy(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		wantStatus int
	}{
		{
			name:       "valid policy",
			body:       `{"decision_wait":"5s","policies":[{"name":"errors","type":"status_code","status_code":{"status_codes":["ERROR"]}}]}`,
			wantStatus: http.StatusOK,
		},
		{
			name:       "malformed json",
			body:       `{not json`,
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := newTestServer(t)
			rec := doRequest(srv, http.MethodPut, "/api/policy", tt.body)
			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d (body: %s)", rec.Code, tt.wantStatus, rec.Body.String())
			}
			if tt.wantStatus == http.StatusOK {
				if got := srv.Policy(); got.DecisionWait.AsDuration() != 5*time.Second {
					t.Fatalf("policy not applied: %+v", got)
				}
			}
		})
	}
}

func TestAPI_PutPolicyYAML(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		wantStatus int
	}{
		{
			name:       "bare yaml",
			body:       "decision_wait: 5s\npolicies:\n  - name: errors\n    type: status_code\n    status_code:\n      status_codes: [ERROR]\n",
			wantStatus: http.StatusOK,
		},
		{
			name:       "wrapped processors.tail_sampling yaml",
			body:       "processors:\n  tail_sampling:\n    decision_wait: 5s\n    policies:\n      - name: errors\n        type: status_code\n        status_code:\n          status_codes: [ERROR]\n",
			wantStatus: http.StatusOK,
		},
		{
			name:       "invalid yaml",
			body:       "not: [valid, yaml",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "yaml with no policies",
			body:       "decision_wait: 5s\n",
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := newTestServer(t)
			rec := doRequest(srv, http.MethodPut, "/api/policy/yaml", tt.body)
			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d (body: %s)", rec.Code, tt.wantStatus, rec.Body.String())
			}
		})
	}
}

func TestAPI_GetTrace(t *testing.T) {
	knownID := pcommon.TraceID{0xAB, 0xCD}

	tests := []struct {
		name       string
		id         string
		wantStatus int
	}{
		{name: "known trace", id: knownID.String(), wantStatus: http.StatusOK},
		{name: "unknown but valid hex id", id: strings.Repeat("00", 16), wantStatus: http.StatusNotFound},
		{name: "odd-length hex", id: "abc", wantStatus: http.StatusBadRequest},
		{name: "non-hex characters", id: strings.Repeat("zz", 16), wantStatus: http.StatusBadRequest},
		{name: "wrong length", id: "ab", wantStatus: http.StatusBadRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := newTestServer(t)
			srv.store.Ingest(time.Now(), map[pcommon.TraceID][]itrace.Span{
				knownID: {{Name: "handle", ServiceName: "payment"}},
			})

			rec := doRequest(srv, http.MethodGet, "/api/traces/"+tt.id, "")
			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d (body: %s)", rec.Code, tt.wantStatus, rec.Body.String())
			}
			if tt.wantStatus == http.StatusOK {
				var got TraceDetail
				mustUnmarshal(t, rec.Body.Bytes(), &got)
				if got.TraceID != knownID.String() || len(got.Spans) != 1 {
					t.Fatalf("unexpected trace detail: %+v", got)
				}
			}
		})
	}
}

func TestAPI_ListTraces_Filter(t *testing.T) {
	srv := newTestServer(t)
	now := time.Now()
	srv.store.Ingest(now, map[pcommon.TraceID][]itrace.Span{
		{1}: {{Name: "handle", ServiceName: "payment", StatusCode: itrace.StatusCodeError}},
	})
	srv.store.Ingest(now.Add(time.Millisecond), map[pcommon.TraceID][]itrace.Span{
		{2}: {{Name: "handle", ServiceName: "catalog", StatusCode: itrace.StatusCodeOK}},
	})

	tests := []struct {
		name      string
		query     string
		wantCount int
		wantFirst string // service_name of the (single) expected match, if wantCount == 1
	}{
		{name: "no filter returns everything", query: "", wantCount: 2},
		{name: "status:error narrows to one", query: "status:error", wantCount: 1, wantFirst: "payment"},
		{name: "service.name substring narrows to one", query: "service.name:cata", wantCount: 1, wantFirst: "catalog"},
		{name: "filter matching nothing", query: "service.name:nope", wantCount: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := doRequest(srv, http.MethodGet, "/api/traces?q="+tt.query, "")
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
			}
			var got []TraceSummary
			mustUnmarshal(t, rec.Body.Bytes(), &got)
			if len(got) != tt.wantCount {
				t.Fatalf("got %d traces, want %d: %+v", len(got), tt.wantCount, got)
			}
			if tt.wantCount == 1 && got[0].ServiceName != tt.wantFirst {
				t.Fatalf("service_name = %q, want %q", got[0].ServiceName, tt.wantFirst)
			}
		})
	}
}

// TestAPI_SessionActions walks the session state machine through the HTTP
// handlers in order, since each step's validity depends on the previous
// one (spec.md ss9).
func TestAPI_SessionActions(t *testing.T) {
	srv := newTestServer(t)

	steps := []struct {
		name       string
		path       string
		wantStatus int
	}{
		{"pause before start is invalid", "/api/session/pause", http.StatusConflict},
		{"stop before start is invalid", "/api/session/stop", http.StatusConflict},
		{"start", "/api/session/start", http.StatusOK},
		{"double start is invalid", "/api/session/start", http.StatusConflict},
		{"pause", "/api/session/pause", http.StatusOK},
		{"double pause is invalid", "/api/session/pause", http.StatusConflict},
		{"resume", "/api/session/resume", http.StatusOK},
		{"stop", "/api/session/stop", http.StatusOK},
		{"double stop is invalid", "/api/session/stop", http.StatusConflict},
	}

	for _, step := range steps {
		t.Run(step.name, func(t *testing.T) {
			rec := doRequest(srv, http.MethodPost, step.path, "")
			if rec.Code != step.wantStatus {
				t.Fatalf("status = %d, want %d (body: %s)", rec.Code, step.wantStatus, rec.Body.String())
			}
		})
	}
}

func mustUnmarshal(t *testing.T, body []byte, v any) {
	t.Helper()
	if err := json.Unmarshal(body, v); err != nil {
		t.Fatalf("unmarshal %s: %v", body, err)
	}
}
