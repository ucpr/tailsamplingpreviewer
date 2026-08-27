package tailpreviewexporter

import (
	"context"
	"net"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
	"go.uber.org/zap"
)

// wsRegistry tracks accepted WebSocket connections so a test can force
// them closed: net/http's Server.Shutdown/Close explicitly do not close
// hijacked connections (which is what a WebSocket upgrade produces), so
// nothing else in the standard library will do this for us.
type wsRegistry struct {
	mu    sync.Mutex
	conns []*websocket.Conn
}

func (r *wsRegistry) add(c *websocket.Conn) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.conns = append(r.conns, c)
}

func (r *wsRegistry) closeAll() {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, c := range r.conns {
		_ = c.CloseNow()
	}
	r.conns = nil
}

// wsEchoServer accepts a WebSocket connection, reads (and discards) the
// hello frame, and otherwise just idles until the client or server closes.
func wsEchoServer(reg *wsRegistry) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true})
		if err != nil {
			return
		}
		reg.add(conn)
		defer conn.CloseNow() //nolint:errcheck
		for {
			if _, _, err := conn.Read(r.Context()); err != nil {
				return
			}
		}
	}
}

// listenOn starts an http.Server on addr (reusing the same port across
// calls, to simulate a Preview Server restarting), returning it and the
// bound address.
func listenOn(t *testing.T, addr string, handler http.Handler) (*http.Server, string) {
	t.Helper()
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	srv := &http.Server{Handler: handler}
	go srv.Serve(ln) //nolint:errcheck
	return srv, ln.Addr().String()
}

// TestConnectionManager_ReconnectsAfterServerRestart covers the MVP
// completion criterion "Preview Server 再起動後に reconnect できる"
// (spec.md ss40, ss15): the exporter must notice the connection drop and
// re-establish it once the Preview Server comes back, all without the
// Production pipeline (ConsumeTraces) ever being blocked in the meantime.
func TestConnectionManager_ReconnectsAfterServerRestart(t *testing.T) {
	reg1 := &wsRegistry{}
	srv1, addr := listenOn(t, "127.0.0.1:0", wsEchoServer(reg1))

	cfg := &Config{
		Endpoint:  "ws://" + addr + "/v1/collector",
		Reconnect: ReconnectConfig{Enabled: true, Interval: 50 * time.Millisecond},
	}
	cm := newConnectionManager(cfg, zap.NewNop(), "test-collector", "v1", &metrics{})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go cm.Run(ctx)

	waitFor(t, "initial connect", func() bool { return cm.Connected() })

	// Simulate the Preview Server going down: close the hijacked WS
	// connections first (Shutdown/Close never do this on their own), then
	// tear down the listener.
	reg1.closeAll()
	_ = srv1.Close()

	waitFor(t, "disconnect detected", func() bool { return !cm.Connected() })

	// And coming back on the same address.
	reg2 := &wsRegistry{}
	srv2, _ := listenOn(t, addr, wsEchoServer(reg2))
	defer func() {
		reg2.closeAll()
		_ = srv2.Close()
	}()

	waitFor(t, "reconnect", func() bool { return cm.Connected() })

	if got := cm.metrics.connectionsTotal.Load(); got < 2 {
		t.Fatalf("expected at least 2 successful connections, got %d", got)
	}
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for: %s", what)
}
