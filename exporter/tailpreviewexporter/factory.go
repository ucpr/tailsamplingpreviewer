package tailpreviewexporter // import "github.com/ucpr/tailsamplingpreviewer/exporter/tailpreviewexporter"

import (
	"context"
	"time"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/exporter"
)

var componentType = component.MustNewType("tailpreview")

// NewFactory returns the tailpreviewexporter component factory.
func NewFactory() exporter.Factory {
	return exporter.NewFactory(
		componentType,
		createDefaultConfig,
		exporter.WithTraces(createTracesExporter, component.StabilityLevelAlpha),
	)
}

func createDefaultConfig() component.Config {
	return &Config{
		Endpoint: "ws://127.0.0.1:17777/v1/collector",
		Queue: QueueConfig{
			Enabled: true,
			Size:    1000,
		},
		Reconnect: ReconnectConfig{
			Enabled:  true,
			Interval: 5 * time.Second,
		},
	}
}

func createTracesExporter(_ context.Context, set exporter.Settings, cfg component.Config) (exporter.Traces, error) {
	c := cfg.(*Config)
	exp := newExporter(c, set)

	base, err := consumer.NewTraces(exp.consumeTraces)
	if err != nil {
		return nil, err
	}
	return &tracesExporter{Traces: base, exp: exp}, nil
}
