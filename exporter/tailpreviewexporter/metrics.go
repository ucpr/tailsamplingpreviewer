package tailpreviewexporter

import "sync/atomic"

// metrics tracks the internal telemetry counters listed in spec.md ss36.
// They are exposed as plain atomics; wiring them into the Collector's own
// OTel SDK meter provider is left as follow-up work since it is not part
// of the MVP completion criteria (spec.md ss40). Connection state itself
// lives on connectionManager (the single source of truth for Connected),
// not here, to avoid two atomics drifting out of sync.
type metrics struct {
	connectionsTotal  atomic.Int64
	reconnectTotal    atomic.Int64
	spansSentTotal    atomic.Int64
	spansDroppedTotal atomic.Int64
	queueSize         atomic.Int64
	writeErrorsTotal  atomic.Int64
}

// MetricsSnapshot is a point-in-time read of every counter, useful for
// tests and for a future debug/health endpoint.
type MetricsSnapshot struct {
	Connected         bool
	ConnectionsTotal  int64
	ReconnectTotal    int64
	SpansSentTotal    int64
	SpansDroppedTotal int64
	QueueSize         int64
	WriteErrorsTotal  int64
}

func (m *metrics) snapshot(connected bool) MetricsSnapshot {
	return MetricsSnapshot{
		Connected:         connected,
		ConnectionsTotal:  m.connectionsTotal.Load(),
		ReconnectTotal:    m.reconnectTotal.Load(),
		SpansSentTotal:    m.spansSentTotal.Load(),
		SpansDroppedTotal: m.spansDroppedTotal.Load(),
		QueueSize:         m.queueSize.Load(),
		WriteErrorsTotal:  m.writeErrorsTotal.Load(),
	}
}
