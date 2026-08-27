package tailpreviewexporter

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"
	"go.uber.org/zap"

	"github.com/ucpr/tailsamplingpreviewer/internal/protocol"
)

const (
	heartbeatInterval = 15 * time.Second
	maxBackoff        = 30 * time.Second
)

var errNotConnected = errors.New("tailpreviewexporter: not connected")

// connectionManager owns the exporter's single WebSocket connection to the
// Preview Server: connect, reconnect, heartbeat, session state and control
// message reception (spec.md ss14).
type connectionManager struct {
	cfg             *Config
	logger          *zap.Logger
	collectorID     string
	exporterVersion string
	metrics         *metrics

	writeMu sync.Mutex
	conn    *websocket.Conn

	paused    atomic.Bool
	connected atomic.Bool
}

func newConnectionManager(cfg *Config, logger *zap.Logger, collectorID, exporterVersion string, m *metrics) *connectionManager {
	return &connectionManager{
		cfg:             cfg,
		logger:          logger,
		collectorID:     collectorID,
		exporterVersion: exporterVersion,
		metrics:         m,
	}
}

// Paused reports whether the Preview Server has asked the exporter to stop
// forwarding traces (spec.md ss32). New spans must be dropped, not queued,
// while paused.
func (cm *connectionManager) Paused() bool { return cm.paused.Load() }

// Connected reports whether the WebSocket connection is currently up.
func (cm *connectionManager) Connected() bool { return cm.connected.Load() }

// Run drives the connect / reconnect loop until ctx is cancelled
// (spec.md ss15). It never returns an error: connection failures are
// logged and retried (or, if reconnect is disabled, Run simply exits and
// the exporter goes dormant without affecting the Production pipeline).
func (cm *connectionManager) Run(ctx context.Context) {
	backoff := cm.cfg.Reconnect.Interval
	if backoff <= 0 {
		backoff = 5 * time.Second
	}

	for ctx.Err() == nil {
		conn, _, err := websocket.Dial(ctx, cm.cfg.Endpoint, nil)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			cm.logger.Warn("tailpreview: connect failed", zap.Error(err), zap.String("endpoint", cm.cfg.Endpoint))
			if !cm.cfg.Reconnect.Enabled {
				return
			}
			cm.metrics.reconnectTotal.Add(1)
			select {
			case <-ctx.Done():
				return
			case <-time.After(backoff):
			}
			backoff = nextBackoff(backoff)
			continue
		}

		backoff = cm.cfg.Reconnect.Interval
		if backoff <= 0 {
			backoff = 5 * time.Second
		}
		cm.metrics.connectionsTotal.Add(1)
		cm.handleConnection(ctx, conn)

		if !cm.cfg.Reconnect.Enabled {
			return
		}
	}
}

func nextBackoff(cur time.Duration) time.Duration {
	next := cur * 2
	if next > maxBackoff {
		return maxBackoff
	}
	if next <= 0 {
		return maxBackoff
	}
	return next
}

// handleConnection owns one WebSocket connection end to end: hello
// handshake, heartbeat, and the control-plane read loop. It blocks until
// the connection closes.
func (cm *connectionManager) handleConnection(ctx context.Context, conn *websocket.Conn) {
	connCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	cm.writeMu.Lock()
	cm.conn = conn
	cm.writeMu.Unlock()
	cm.connected.Store(true)
	cm.paused.Store(false)

	defer func() {
		cm.connected.Store(false)
		cm.writeMu.Lock()
		cm.conn = nil
		cm.writeMu.Unlock()
		_ = conn.Close(websocket.StatusNormalClosure, "")
	}()

	hello := protocol.NewHello(
		protocol.CollectorInfo{ID: cm.collectorID, Version: cm.exporterVersion},
		protocol.ExporterInfo{Version: cm.exporterVersion},
	)
	if err := cm.writeJSON(connCtx, hello); err != nil {
		cm.logger.Warn("tailpreview: hello failed", zap.Error(err))
		return
	}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		cm.heartbeatLoop(connCtx)
	}()

	cm.readLoop(connCtx, conn)
	cancel()
	wg.Wait()
}

func (cm *connectionManager) heartbeatLoop(ctx context.Context) {
	ticker := time.NewTicker(heartbeatInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case t := <-ticker.C:
			if err := cm.writeJSON(ctx, protocol.NewPing(t.UnixMilli())); err != nil {
				return
			}
		}
	}
}

func (cm *connectionManager) readLoop(ctx context.Context, conn *websocket.Conn) {
	for {
		msgType, data, err := conn.Read(ctx)
		if err != nil {
			return
		}
		if msgType != websocket.MessageText {
			continue // the exporter never receives Data Plane binary frames
		}
		cm.handleControlMessage(ctx, data)
	}
}

func (cm *connectionManager) handleControlMessage(ctx context.Context, data []byte) {
	env, err := protocol.DecodeEnvelope(data)
	if err != nil {
		cm.logger.Debug("tailpreview: malformed control message", zap.Error(err))
		return
	}

	switch env.Type {
	case protocol.TypeHelloAck:
		cm.logger.Info("tailpreview: connected to preview server")
	case protocol.TypePing:
		var p protocol.Ping
		if json.Unmarshal(data, &p) == nil {
			_ = cm.writeJSON(ctx, protocol.NewPong(p.Timestamp))
		}
	case protocol.TypeSessionPause, protocol.TypeSessionStop:
		cm.paused.Store(true)
	case protocol.TypeSessionResume, protocol.TypeSessionStart:
		cm.paused.Store(false)
	}
}

// SendBinary writes a Data Plane frame. It is the only method the writer
// goroutine calls (spec.md ss12).
func (cm *connectionManager) SendBinary(ctx context.Context, data []byte) error {
	cm.writeMu.Lock()
	defer cm.writeMu.Unlock()
	if cm.conn == nil {
		return errNotConnected
	}
	return cm.conn.Write(ctx, websocket.MessageBinary, data)
}

func (cm *connectionManager) writeJSON(ctx context.Context, v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	cm.writeMu.Lock()
	defer cm.writeMu.Unlock()
	if cm.conn == nil {
		return errNotConnected
	}
	return cm.conn.Write(ctx, websocket.MessageText, data)
}
