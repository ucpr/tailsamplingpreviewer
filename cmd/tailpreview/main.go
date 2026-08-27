// Command tailpreview runs the Tail Sampling Preview Server (spec.md ss19,
// ss38): it accepts Collector WebSocket connections at
// server.collector_endpoint, serves the Browser UI and its WebSocket/REST
// API at server.ui_endpoint, and never persists trace data to disk.
package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.uber.org/zap"

	"github.com/ucpr/tailsamplingpreviewer/internal/server"
	"github.com/ucpr/tailsamplingpreviewer/internal/trace"
)

const version = "0.1.0"

func main() {
	collectorEndpoint := flag.String("collector-endpoint", "127.0.0.1:17777", "address the Collector WebSocket endpoint listens on")
	uiEndpoint := flag.String("ui-endpoint", "127.0.0.1:17778", "address the Browser UI + REST API listens on")
	webDir := flag.String("web-dir", "web/dist", "directory containing the built Browser UI static assets")
	maxTraces := flag.Int("max-traces", 10000, "ring buffer capacity (spec.md ss31)")
	maxAge := flag.Duration("max-age", 5*time.Minute, "ring buffer max trace age (spec.md ss31)")
	flag.Parse()

	logger, err := zap.NewProduction()
	if err != nil {
		fmt.Fprintln(os.Stderr, "tailpreview: logger init failed:", err)
		os.Exit(1)
	}
	defer logger.Sync() //nolint:errcheck

	srv := server.New(logger, server.Config{
		Version: version,
		Store:   trace.StoreConfig{MaxTraces: *maxTraces, MaxAge: *maxAge},
	})

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	mux := srv.Router()
	mux.Handle("/", staticHandler(*webDir))

	collectorSrv := &http.Server{Addr: *collectorEndpoint, Handler: mux}
	uiSrv := &http.Server{Addr: *uiEndpoint, Handler: mux}

	fmt.Println("Tail Preview")
	fmt.Println()
	fmt.Printf("Collector endpoint\n  ws://%s/v1/collector\n\n", *collectorEndpoint)
	fmt.Printf("UI\n  http://%s\n\n", *uiEndpoint)
	fmt.Println("Waiting for Collector...")

	go srv.Run(ctx)

	errCh := make(chan error, 2)
	go func() { errCh <- collectorSrv.ListenAndServe() }()
	go func() { errCh <- uiSrv.ListenAndServe() }()

	select {
	case <-ctx.Done():
	case err := <-errCh:
		if err != nil && err != http.ErrServerClosed {
			logger.Error("tailpreview: server error", zap.Error(err))
		}
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	_ = collectorSrv.Shutdown(shutdownCtx)
	_ = uiSrv.Shutdown(shutdownCtx)
}

func staticHandler(dir string) http.Handler {
	if _, err := os.Stat(dir); err != nil {
		return http.NotFoundHandler()
	}
	return http.FileServer(http.Dir(dir))
}
