package tailpreviewexporter

import (
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.opentelemetry.io/collector/pdata/ptrace/ptraceotlp"

	"github.com/ucpr/tailsamplingpreviewer/internal/protocol"
)

// marshal serializes td as a Data Plane binary frame: FrameHeader followed
// by an ExportTraceServiceRequest protobuf message (spec.md ss7).
func marshal(td ptrace.Traces) ([]byte, error) {
	req := ptraceotlp.NewExportRequestFromTraces(td)
	payload, err := req.MarshalProto()
	if err != nil {
		return nil, err
	}
	return protocol.EncodeFrame(protocol.FrameHeader{
		Version: protocol.FrameVersion,
		Type:    protocol.FrameTypeTraces,
	}, payload), nil
}
