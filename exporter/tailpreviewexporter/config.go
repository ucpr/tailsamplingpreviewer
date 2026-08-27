package tailpreviewexporter // import "github.com/ucpr/tailsamplingpreviewer/exporter/tailpreviewexporter"

import (
	"errors"
	"time"
)

// Config is the `exporters.tailpreview:` block (spec.md ss10).
type Config struct {
	// Endpoint is the Preview Server's Collector WebSocket URL, e.g.
	// ws://127.0.0.1:17777/v1/collector.
	Endpoint string `mapstructure:"endpoint"`

	// CollectorID identifies this Collector instance in the `hello`
	// handshake (spec.md ss17). Defaults to the OS hostname when empty.
	CollectorID string `mapstructure:"collector_id"`

	Queue     QueueConfig     `mapstructure:"queue"`
	Reconnect ReconnectConfig `mapstructure:"reconnect"`
}

// QueueConfig bounds the in-memory queue between ConsumeTraces and the
// WebSocket writer goroutine (spec.md ss12). When the queue is full, new
// payloads are dropped rather than blocking the pipeline (spec.md ss13).
type QueueConfig struct {
	Enabled bool `mapstructure:"enabled"`
	Size    int  `mapstructure:"size"`
}

// ReconnectConfig controls the exponential-backoff reconnect loop
// (spec.md ss15).
type ReconnectConfig struct {
	Enabled  bool          `mapstructure:"enabled"`
	Interval time.Duration `mapstructure:"interval"`
}

func (c *Config) Validate() error {
	if c.Endpoint == "" {
		return errors.New("endpoint must not be empty")
	}
	if c.Queue.Enabled && c.Queue.Size <= 0 {
		return errors.New("queue.size must be positive when queue.enabled is true")
	}
	if c.Reconnect.Enabled && c.Reconnect.Interval <= 0 {
		return errors.New("reconnect.interval must be positive when reconnect.enabled is true")
	}
	return nil
}
