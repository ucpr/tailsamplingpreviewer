package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.opentelemetry.io/collector/pdata/ptrace/ptraceotlp"
	"go.uber.org/zap"

	"github.com/ucpr/tailsamplingpreviewer/internal/protocol"
	"github.com/ucpr/tailsamplingpreviewer/internal/sampling"
)

func buildTraces(traceID [16]byte, serviceName, statusCode string) ptrace.Traces {
	td := ptrace.NewTraces()
	rs := td.ResourceSpans().AppendEmpty()
	rs.Resource().Attributes().PutStr("service.name", serviceName)
	span := rs.ScopeSpans().AppendEmpty().Spans().AppendEmpty()
	span.SetTraceID(pcommon.TraceID(traceID))
	span.SetSpanID(pcommon.SpanID{1, 2, 3, 4, 5, 6, 7, 8})
	span.SetName("handle")
	now := time.Now()
	span.SetStartTimestamp(pcommon.NewTimestampFromTime(now))
	span.SetEndTimestamp(pcommon.NewTimestampFromTime(now.Add(50 * time.Millisecond)))
	if statusCode == "ERROR" {
		span.Status().SetCode(ptrace.StatusCodeError)
	}
	return td
}

func TestEndToEnd_IngestAndDecide(t *testing.T) {
	logger := zap.NewNop()
	srv, err := New(logger, Config{Version: "test"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go srv.Run(ctx)

	ts := httptest.NewServer(srv.Router())
	defer ts.Close()

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/v1/collector"
	conn, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatalf("dial collector ws: %v", err)
	}
	defer conn.CloseNow() //nolint:errcheck

	hello := protocol.NewHello(protocol.CollectorInfo{ID: "test-collector", Version: "v1"}, protocol.ExporterInfo{Version: "v1"})
	helloData, _ := json.Marshal(hello)
	if err := conn.Write(ctx, websocket.MessageText, helloData); err != nil {
		t.Fatalf("write hello: %v", err)
	}

	// hello.ack, then session control message
	if _, _, err := conn.Read(ctx); err != nil {
		t.Fatalf("read hello.ack: %v", err)
	}
	if _, _, err := conn.Read(ctx); err != nil {
		t.Fatalf("read session control: %v", err)
	}

	// Set a short decision_wait so the test doesn't need to wait 30s.
	policy := sampling.Config{
		DecisionWait: sampling.Duration(100 * time.Millisecond),
		Policies: []sampling.PolicyCfg{
			{Name: "errors", Type: sampling.StatusCode, StatusCode: &sampling.StatusCodeCfg{StatusCodes: []string{"ERROR"}}},
		},
	}
	body, _ := json.Marshal(policy)
	req, _ := http.NewRequest(http.MethodPut, ts.URL+"/api/policy", strings.NewReader(string(body)))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("put policy: %v", err)
	}
	resp.Body.Close()

	var traceID [16]byte
	copy(traceID[:], []byte("0123456789abcdef"))
	td := buildTraces(traceID, "payment-api", "ERROR")
	reqPB := ptraceotlp.NewExportRequestFromTraces(td)
	payload, err := reqPB.MarshalProto()
	if err != nil {
		t.Fatalf("marshal proto: %v", err)
	}
	frame := protocol.EncodeFrame(protocol.FrameHeader{Version: protocol.FrameVersion, Type: protocol.FrameTypeTraces}, payload)
	if err := conn.Write(ctx, websocket.MessageBinary, frame); err != nil {
		t.Fatalf("write trace frame: %v", err)
	}

	deadline := time.Now().Add(5 * time.Second)
	var decision string
	for time.Now().Before(deadline) {
		resp, err := http.Get(ts.URL + "/api/traces")
		if err != nil {
			t.Fatalf("get traces: %v", err)
		}
		var traces []TraceSummary
		_ = json.NewDecoder(resp.Body).Decode(&traces)
		resp.Body.Close()
		if len(traces) == 1 && traces[0].Decision != "PENDING" {
			decision = traces[0].Decision
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	if decision != "KEEP" {
		t.Fatalf("expected KEEP decision, got %q", decision)
	}

	resp, err = http.Get(ts.URL + "/api/statistics")
	if err != nil {
		t.Fatalf("get statistics: %v", err)
	}
	var stats StatisticsView
	_ = json.NewDecoder(resp.Body).Decode(&stats)
	resp.Body.Close()
	if stats.Keep != 1 || stats.Observed != 1 {
		t.Fatalf("unexpected statistics: %+v", stats)
	}
}
