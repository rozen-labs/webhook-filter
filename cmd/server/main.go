package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/rozen-labs/webhook-filter/internal/config"
	"github.com/rozen-labs/webhook-filter/internal/httpserver"
	"github.com/rozen-labs/webhook-filter/internal/logging"
)

func main() {
	configPath := flag.String("config", "/config/config.yml", "Path to config.yml")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "config error: %v\n", err)
		os.Exit(1)
	}

	logger := logging.New(cfg.Logging)
	server, err := httpserver.New(cfg, logger)
	if err != nil {
		fmt.Fprintf(os.Stderr, "server init error: %v\n", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	logger.Info("starting webhook-filter", slog.String("listen_addr", cfg.Server.ListenAddr))
	if err := server.Start(ctx); err != nil && err != http.ErrServerClosed {
		fmt.Fprintf(os.Stderr, "server error: %v\n", err)
		os.Exit(1)
	}
}
