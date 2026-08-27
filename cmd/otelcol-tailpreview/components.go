package main

import (
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/connector"
	"go.opentelemetry.io/collector/exporter"
	"go.opentelemetry.io/collector/exporter/debugexporter"
	"go.opentelemetry.io/collector/exporter/otlpexporter"
	"go.opentelemetry.io/collector/extension"
	"go.opentelemetry.io/collector/otelcol"
	"go.opentelemetry.io/collector/processor"
	"go.opentelemetry.io/collector/processor/batchprocessor"
	"go.opentelemetry.io/collector/processor/memorylimiterprocessor"
	"go.opentelemetry.io/collector/receiver"
	"go.opentelemetry.io/collector/receiver/otlpreceiver"
	"go.opentelemetry.io/collector/service/telemetry"

	"github.com/ucpr/tailsamplingpreviewer/exporter/tailpreviewexporter"
)

// noopTelemetryConfig is the (empty) config for the no-self-telemetry
// factory below. Wiring the Collector's own internal metrics/traces/logs
// is out of scope for this MVP (spec.md ss40 does not require it); the
// factory falls back to noop logger/meter/tracer providers.
type noopTelemetryConfig struct{}

// components assembles the minimal set of factories needed to run the
// pipeline from spec.md ss10: an OTLP receiver, batch/memory-limiter
// processors, the Production otlp exporter (or debug exporter for local
// testing), and tailpreviewexporter as the Shadow Traffic exporter.
func components() (otelcol.Factories, error) {
	receivers, err := otelcol.MakeFactoryMap[receiver.Factory](otlpreceiver.NewFactory())
	if err != nil {
		return otelcol.Factories{}, err
	}
	processors, err := otelcol.MakeFactoryMap[processor.Factory](
		batchprocessor.NewFactory(),
		memorylimiterprocessor.NewFactory(),
	)
	if err != nil {
		return otelcol.Factories{}, err
	}
	exporters, err := otelcol.MakeFactoryMap[exporter.Factory](
		otlpexporter.NewFactory(),
		debugexporter.NewFactory(),
		tailpreviewexporter.NewFactory(),
	)
	if err != nil {
		return otelcol.Factories{}, err
	}

	return otelcol.Factories{
		Receivers:  receivers,
		Processors: processors,
		Exporters:  exporters,
		Extensions: map[component.Type]extension.Factory{},
		Connectors: map[component.Type]connector.Factory{},
		Telemetry:  telemetry.NewFactory(func() component.Config { return &noopTelemetryConfig{} }),
	}, nil
}
