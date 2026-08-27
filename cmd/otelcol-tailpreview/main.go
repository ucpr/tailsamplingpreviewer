// Command otelcol-tailpreview is a minimal OpenTelemetry Collector build
// that bundles tailpreviewexporter alongside the standard OTLP
// receiver/exporter and batch/memory-limiter processors (spec.md ss10,
// ss39 Phase 0). It exists so the exporter can be exercised end to end
// against a real Collector pipeline without depending on the upstream
// ocb builder.
package main

import (
	"context"
	"fmt"
	"os"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/confmap"
	"go.opentelemetry.io/collector/confmap/provider/envprovider"
	"go.opentelemetry.io/collector/confmap/provider/fileprovider"
	"go.opentelemetry.io/collector/confmap/provider/httpprovider"
	"go.opentelemetry.io/collector/confmap/provider/yamlprovider"
	"go.opentelemetry.io/collector/otelcol"
)

const version = "0.1.0"

func main() {
	factories, err := components()
	if err != nil {
		fmt.Fprintln(os.Stderr, "otelcol-tailpreview: failed to build components:", err)
		os.Exit(1)
	}

	configPath := "config.yaml"
	if len(os.Args) > 1 {
		configPath = os.Args[1]
	}

	settings := otelcol.CollectorSettings{
		BuildInfo: component.BuildInfo{
			Command:     "otelcol-tailpreview",
			Description: "OpenTelemetry Collector with tailpreviewexporter",
			Version:     version,
		},
		Factories: func() (otelcol.Factories, error) { return factories, nil },
		ConfigProviderSettings: otelcol.ConfigProviderSettings{
			ResolverSettings: confmap.ResolverSettings{
				URIs: []string{configPath},
				ProviderFactories: []confmap.ProviderFactory{
					fileprovider.NewFactory(),
					envprovider.NewFactory(),
					yamlprovider.NewFactory(),
					httpprovider.NewFactory(),
				},
			},
		},
	}

	col, err := otelcol.NewCollector(settings)
	if err != nil {
		fmt.Fprintln(os.Stderr, "otelcol-tailpreview: failed to construct collector:", err)
		os.Exit(1)
	}

	if err := col.Run(context.Background()); err != nil {
		fmt.Fprintln(os.Stderr, "otelcol-tailpreview: run failed:", err)
		os.Exit(1)
	}
}
