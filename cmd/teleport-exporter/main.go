package main

import (
	"context"
	"errors"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/pvandermoere/teleport-exporter/internal/collector"
	"github.com/pvandermoere/teleport-exporter/internal/config"
	"github.com/pvandermoere/teleport-exporter/internal/teleport"
)

func main() {
	configPath := flag.String("config", "", "Path to the YAML config file (optional; falls back to environment variables, see README)")
	flag.Parse()

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	cfg, err := config.Load(*configPath)
	if err != nil {
		logger.Error("failed to load configuration", "error", err)
		os.Exit(1)
	}

	client := teleport.NewClient(cfg.Address, cfg.IdentityFile, cfg.Insecure)
	defer func() { _ = client.Close() }()

	coll := collector.NewCollector(client, cfg.RefreshIntervalDuration(), cfg.CollectTimeoutDuration(), cfg.AddressHost(), logger)

	registry := prometheus.NewRegistry()
	registry.MustRegister(
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
		coll,
	)

	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.HandlerFor(registry, promhttp.HandlerOpts{}))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<html><head><title>Teleport Exporter</title></head>
<body><h1>Teleport Exporter</h1><p><a href="/metrics">Metrics</a></p></body></html>`))
	})

	server := &http.Server{
		Addr:              cfg.ListenAddress,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go coll.Run(ctx)

	go func() {
		<-ctx.Done()
		logger.Info("shutting down")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			logger.Error("error during shutdown", "error", err)
		}
	}()

	logger.Info("listening", "address", cfg.ListenAddress, "teleport", cfg.Address, "refresh_interval", cfg.RefreshInterval.String())
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Error("server error", "error", err)
		os.Exit(1)
	}
}
