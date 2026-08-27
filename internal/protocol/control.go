package protocol

import "encoding/json"

// Control message "type" discriminator values exchanged between the
// Collector exporter and the Preview Server (spec.md ss8, ss16, ss17, ss9).
const (
	TypeHello         = "hello"
	TypeHelloAck      = "hello.ack"
	TypePing          = "ping"
	TypePong          = "pong"
	TypeSessionStart  = "session.start"
	TypeSessionStop   = "session.stop"
	TypeSessionPause  = "session.pause"
	TypeSessionResume = "session.resume"
)

// Envelope is the shape every control-plane text frame conforms to. Decode
// Envelope first, then dispatch on Type to unmarshal the concrete message.
type Envelope struct {
	Type string `json:"type"`
}

// CollectorInfo identifies the Collector instance sending traces.
type CollectorInfo struct {
	ID      string `json:"id"`
	Version string `json:"version"`
}

// ExporterInfo identifies the tailpreviewexporter build.
type ExporterInfo struct {
	Version string `json:"version"`
}

// Hello is sent by the exporter immediately after the WebSocket connection
// is established (spec.md ss17).
type Hello struct {
	Type      string        `json:"type"`
	Collector CollectorInfo `json:"collector"`
	Exporter  ExporterInfo  `json:"exporter"`
}

func NewHello(collector CollectorInfo, exporter ExporterInfo) Hello {
	return Hello{Type: TypeHello, Collector: collector, Exporter: exporter}
}

// ServerInfo identifies the Preview Server build.
type ServerInfo struct {
	Version string `json:"version"`
}

// HelloAck acknowledges a Hello.
type HelloAck struct {
	Type   string     `json:"type"`
	Server ServerInfo `json:"server"`
}

func NewHelloAck(server ServerInfo) HelloAck {
	return HelloAck{Type: TypeHelloAck, Server: server}
}

// Ping / Pong implement the connection heartbeat (spec.md ss16).
type Ping struct {
	Type      string `json:"type"`
	Timestamp int64  `json:"timestamp"`
}

func NewPing(unixMilli int64) Ping { return Ping{Type: TypePing, Timestamp: unixMilli} }

type Pong struct {
	Type      string `json:"type"`
	Timestamp int64  `json:"timestamp"`
}

func NewPong(unixMilli int64) Pong { return Pong{Type: TypePong, Timestamp: unixMilli} }

// SessionStart / SessionStop / SessionPause / SessionResume are Preview
// Server -> Exporter control messages (spec.md ss8, ss32).
type SessionStart struct {
	Type      string `json:"type"`
	SessionID string `json:"session_id"`
}

func NewSessionStart(sessionID string) SessionStart {
	return SessionStart{Type: TypeSessionStart, SessionID: sessionID}
}

type SessionStop struct {
	Type      string `json:"type"`
	SessionID string `json:"session_id"`
}

func NewSessionStop(sessionID string) SessionStop {
	return SessionStop{Type: TypeSessionStop, SessionID: sessionID}
}

type SessionPause struct {
	Type string `json:"type"`
}

func NewSessionPause() SessionPause { return SessionPause{Type: TypeSessionPause} }

type SessionResume struct {
	Type string `json:"type"`
}

func NewSessionResume() SessionResume { return SessionResume{Type: TypeSessionResume} }

// DecodeEnvelope reports the message type without decoding the full body,
// so callers can dispatch to the right concrete type.
func DecodeEnvelope(raw []byte) (Envelope, error) {
	var e Envelope
	if err := json.Unmarshal(raw, &e); err != nil {
		return Envelope{}, err
	}
	return e, nil
}
