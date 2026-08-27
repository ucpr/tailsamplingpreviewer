package tailpreviewexporter

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.opentelemetry.io/collector/pipeline"
)

func TestFactory_TypeAndDefaultConfig(t *testing.T) {
	f := NewFactory()
	if got := f.Type().String(); got != "tailpreview" {
		t.Fatalf("Type() = %q, want %q", got, "tailpreview")
	}

	cfg, ok := f.CreateDefaultConfig().(*Config)
	if !ok {
		t.Fatalf("CreateDefaultConfig() returned %T, want *Config", f.CreateDefaultConfig())
	}

	tests := []struct {
		name string
		got  any
		want any
	}{
		{"endpoint", cfg.Endpoint, "ws://127.0.0.1:17777/v1/collector"},
		{"queue.enabled", cfg.Queue.Enabled, true},
		{"queue.size", cfg.Queue.Size, 1000},
		{"reconnect.enabled", cfg.Reconnect.Enabled, true},
		{"reconnect.interval", cfg.Reconnect.Interval, 5 * time.Second},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Fatalf("default %s = %v, want %v", tt.name, tt.got, tt.want)
			}
		})
	}

	if err := cfg.Validate(); err != nil {
		t.Fatalf("default config must be valid: %v", err)
	}
}

func TestFactory_SignalSupport(t *testing.T) {
	f := NewFactory()
	cfg := f.CreateDefaultConfig()

	tests := []struct {
		name        string
		err         error
		wantSupport bool
	}{
		{
			name:        "traces is supported",
			wantSupport: true,
			err: func() error {
				_, err := f.CreateTraces(context.Background(), testSettings(), cfg)
				return err
			}(),
		},
		{
			name: "metrics is not supported",
			err: func() error {
				_, err := f.CreateMetrics(context.Background(), testSettings(), cfg)
				return err
			}(),
		},
		{
			name: "logs is not supported",
			err: func() error {
				_, err := f.CreateLogs(context.Background(), testSettings(), cfg)
				return err
			}(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.wantSupport {
				if tt.err != nil {
					t.Fatalf("expected success, got error: %v", tt.err)
				}
				return
			}
			if !errors.Is(tt.err, pipeline.ErrSignalNotSupported) {
				t.Fatalf("expected ErrSignalNotSupported, got: %v", tt.err)
			}
		})
	}
}
