// Command loadgen is a synthetic OTLP trace generator used by
// examples/compose.yaml to exercise the whole Tail Sampling Preview
// pipeline end to end: loadgen -> Collector (otlpreceiver) ->
// tailpreviewexporter -> Preview Server.
//
// It emits a steady mix of fast/slow and OK/ERROR traces across a handful
// of fake services, so the Live Tail, Policy Builder and Statistics
// panels all have something to show immediately after `docker compose up`.
package main

import (
	"context"
	"log"
	"math/rand"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	otelresource "go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"

	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

// scenario describes one fake service's traffic shape. service.name is an
// OTLP *resource* attribute, so each scenario gets its own TracerProvider
// (sharing one exporter) rather than trying to override it per span.
type scenario struct {
	service     string
	route       string
	children    []string
	baseMs      int
	slowMs      int
	slowChance  float64
	errorChance float64
}

var scenarios = []scenario{
	{service: "payment-api", route: "/v1/charge", children: []string{"postgres", "stripe-webhook"}, baseMs: 40, slowMs: 1800, slowChance: 0.12, errorChance: 0.08},
	{service: "catalog-api", route: "/v1/products", children: []string{"redis"}, baseMs: 15, slowMs: 900, slowChance: 0.05, errorChance: 0.02},
	{service: "auth-api", route: "/v1/login", children: []string{"postgres", "redis"}, baseMs: 25, slowMs: 1200, slowChance: 0.08, errorChance: 0.1},
	{service: "notification-api", route: "/v1/notify", children: []string{"sqs"}, baseMs: 10, slowMs: 600, slowChance: 0.03, errorChance: 0.03},
}

func main() {
	endpoint := getenv("OTLP_ENDPOINT", "127.0.0.1:4318")
	interval := getenvDuration("TRACE_INTERVAL", 400*time.Millisecond)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	exp, err := otlptracehttp.New(ctx,
		otlptracehttp.WithEndpoint(endpoint),
		otlptracehttp.WithInsecure(),
	)
	if err != nil {
		log.Fatalf("loadgen: failed to create OTLP exporter: %v", err)
	}

	tracers := make(map[string]trace.Tracer, len(scenarios))
	var providers []*sdktrace.TracerProvider
	for _, sc := range scenarios {
		res, err := otelresource.New(ctx, otelresource.WithAttributes(
			semconv.ServiceName(sc.service),
			semconv.DeploymentEnvironment("dev"),
		))
		if err != nil {
			log.Fatalf("loadgen: failed to build resource for %s: %v", sc.service, err)
		}
		tp := sdktrace.NewTracerProvider(
			sdktrace.WithBatcher(exp, sdktrace.WithBatchTimeout(time.Second)),
			sdktrace.WithResource(res),
		)
		providers = append(providers, tp)
		tracers[sc.service] = tp.Tracer("loadgen")
	}
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		for _, tp := range providers {
			_ = tp.Shutdown(shutdownCtx)
		}
	}()

	log.Printf("loadgen: sending synthetic traces to %s every %s", endpoint, interval)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	var sent int
	for {
		select {
		case <-ctx.Done():
			log.Printf("loadgen: shutting down after sending %d traces", sent)
			return
		case <-ticker.C:
			sc := scenarios[rand.Intn(len(scenarios))]
			emitTrace(ctx, tracers[sc.service], sc)
			sent++
			if sent%20 == 0 {
				log.Printf("loadgen: sent %d traces so far", sent)
			}
		}
	}
}

func emitTrace(ctx context.Context, tracer trace.Tracer, sc scenario) {
	durationMs := sc.baseMs + rand.Intn(sc.baseMs+1)
	if rand.Float64() < sc.slowChance {
		durationMs = sc.slowMs + rand.Intn(sc.slowMs/2+1)
	}
	isError := rand.Float64() < sc.errorChance

	rootCtx, root := tracer.Start(ctx, "handle "+sc.route,
		trace.WithAttributes(
			attribute.String("http.method", "POST"),
			attribute.String("http.route", sc.route),
		),
	)

	remaining := durationMs
	for _, child := range sc.children {
		share := remaining / (len(sc.children) + 1)
		if share <= 0 {
			share = 1
		}
		_, cs := tracer.Start(rootCtx, child)
		time.Sleep(time.Duration(share) * time.Millisecond)
		cs.End()
		remaining -= share
	}
	if remaining > 0 {
		time.Sleep(time.Duration(remaining) * time.Millisecond)
	}

	if isError {
		root.SetStatus(codes.Error, "internal error")
	} else {
		root.SetStatus(codes.Ok, "")
	}
	root.End()
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func getenvDuration(key string, def time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return def
}
