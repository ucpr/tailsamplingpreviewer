package protocol

import (
	"encoding/json"
	"testing"
)

// TestConstructors_SetType verifies every control-message constructor
// stamps the right "type" discriminator and round-trips through JSON.
func TestConstructors_SetType(t *testing.T) {
	tests := []struct {
		name     string
		v        any
		wantType string
	}{
		{"Hello", NewHello(CollectorInfo{ID: "c1", Version: "v1"}, ExporterInfo{Version: "v1"}), TypeHello},
		{"HelloAck", NewHelloAck(ServerInfo{Version: "v1"}), TypeHelloAck},
		{"Ping", NewPing(1234), TypePing},
		{"Pong", NewPong(1234), TypePong},
		{"SessionStart", NewSessionStart("session-1"), TypeSessionStart},
		{"SessionStop", NewSessionStop("session-1"), TypeSessionStop},
		{"SessionPause", NewSessionPause(), TypeSessionPause},
		{"SessionResume", NewSessionResume(), TypeSessionResume},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.v)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			env, err := DecodeEnvelope(data)
			if err != nil {
				t.Fatalf("DecodeEnvelope: %v", err)
			}
			if env.Type != tt.wantType {
				t.Fatalf("type = %q, want %q (json: %s)", env.Type, tt.wantType, data)
			}
		})
	}
}

func TestDecodeEnvelope(t *testing.T) {
	tests := []struct {
		name     string
		raw      string
		wantType string
		wantErr  bool
	}{
		{"hello", `{"type":"hello","collector":{"id":"c1"}}`, "hello", false},
		{"unknown type is not an error at the envelope level", `{"type":"something.new"}`, "something.new", false},
		{"missing type decodes to empty string", `{}`, "", false},
		{"malformed json errors", `not json`, "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env, err := DecodeEnvelope([]byte(tt.raw))
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if err == nil && env.Type != tt.wantType {
				t.Fatalf("type = %q, want %q", env.Type, tt.wantType)
			}
		})
	}
}

// TestHeartbeatTimestampRoundTrips guards against a plausible regression:
// mixing up the Ping/Pong timestamp field during a protocol change.
func TestHeartbeatTimestampRoundTrips(t *testing.T) {
	ping := NewPing(1787731200000)
	data, _ := json.Marshal(ping)
	var decoded Ping
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.Timestamp != 1787731200000 {
		t.Fatalf("timestamp = %d, want 1787731200000", decoded.Timestamp)
	}
}
