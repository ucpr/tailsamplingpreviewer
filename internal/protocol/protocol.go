// Package protocol defines the wire protocol between the OpenTelemetry
// Collector (tailpreviewexporter) and the Preview Server.
//
// A single WebSocket connection carries two logically separated planes:
//
//   - Data Plane: WebSocket Binary messages. Payload is a FrameHeader
//     followed by an ExportTraceServiceRequest protobuf message.
//   - Control Plane: WebSocket Text messages. Payload is a JSON object
//     with a "type" discriminator field.
//
// The message type (binary vs. text) is what separates the two planes, so
// no additional multiplexing is required on top of WebSocket framing.
package protocol

// FrameVersion identifies the binary framing format.
const FrameVersion uint8 = 1

// FrameType identifies the kind of payload that follows the FrameHeader.
type FrameType uint8

const (
	// FrameTypeTraces indicates the payload is an ExportTraceServiceRequest
	// protobuf message.
	FrameTypeTraces FrameType = 1
)

// HeaderSize is the fixed size, in bytes, of a FrameHeader.
const HeaderSize = 2

// FrameHeader precedes every Data Plane binary WebSocket message.
type FrameHeader struct {
	Version uint8
	Type    FrameType
}

// Encode writes the header followed by payload into a single byte slice.
func EncodeFrame(h FrameHeader, payload []byte) []byte {
	buf := make([]byte, HeaderSize+len(payload))
	buf[0] = h.Version
	buf[1] = uint8(h.Type)
	copy(buf[HeaderSize:], payload)
	return buf
}

// DecodeFrame splits a raw binary WebSocket message into its header and
// payload. It returns false if the message is shorter than HeaderSize.
func DecodeFrame(raw []byte) (FrameHeader, []byte, bool) {
	if len(raw) < HeaderSize {
		return FrameHeader{}, nil, false
	}
	h := FrameHeader{
		Version: raw[0],
		Type:    FrameType(raw[1]),
	}
	return h, raw[HeaderSize:], true
}
