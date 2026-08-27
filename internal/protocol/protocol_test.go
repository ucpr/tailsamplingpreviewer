package protocol

import (
	"bytes"
	"testing"
)

func TestEncodeDecodeFrame_RoundTrip(t *testing.T) {
	tests := []struct {
		name    string
		header  FrameHeader
		payload []byte
	}{
		{"typical traces frame", FrameHeader{Version: FrameVersion, Type: FrameTypeTraces}, []byte{0x01, 0x02, 0x03}},
		{"empty payload", FrameHeader{Version: FrameVersion, Type: FrameTypeTraces}, []byte{}},
		{"nil payload", FrameHeader{Version: FrameVersion, Type: FrameTypeTraces}, nil},
		{"large payload", FrameHeader{Version: FrameVersion, Type: FrameTypeTraces}, bytes.Repeat([]byte{0xAB}, 4096)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encoded := EncodeFrame(tt.header, tt.payload)

			gotHeader, gotPayload, ok := DecodeFrame(encoded)
			if !ok {
				t.Fatalf("DecodeFrame reported ok=false for a validly-encoded frame")
			}
			if gotHeader != tt.header {
				t.Fatalf("header = %+v, want %+v", gotHeader, tt.header)
			}
			if !bytes.Equal(gotPayload, tt.payload) {
				t.Fatalf("payload = %v, want %v", gotPayload, tt.payload)
			}
		})
	}
}

func TestDecodeFrame_ShortInput(t *testing.T) {
	tests := []struct {
		name string
		raw  []byte
	}{
		{"empty", []byte{}},
		{"nil", nil},
		{"one byte, shorter than the header", []byte{0x01}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, ok := DecodeFrame(tt.raw)
			if ok {
				t.Fatalf("expected ok=false for input shorter than HeaderSize (%d bytes)", HeaderSize)
			}
		})
	}
}
