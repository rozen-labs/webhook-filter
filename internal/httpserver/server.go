package httpserver

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rozen-labs/webhook-filter/internal/config"
	"github.com/rozen-labs/webhook-filter/internal/forwarder"
	"github.com/rozen-labs/webhook-filter/internal/metrics"
)

type Server struct {
	httpServer *http.Server
}

func New(cfg *config.Config, logger *slog.Logger) (*Server, error) {
	h := NewHandler(cfg, logger, forwarder.New())
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", h.Healthz)
	mux.Handle("/metrics", promhttp.HandlerFor(metrics.Registry, promhttp.HandlerOpts{}))
	mux.Handle("/", h)
	s := &http.Server{
		Addr:              cfg.Server.ListenAddr,
		Handler:           mux,
		ReadTimeout:       time.Duration(cfg.Server.RequestTimeoutSeconds) * time.Second,
		ReadHeaderTimeout: 5 * time.Second,
		WriteTimeout:      time.Duration(cfg.Server.RequestTimeoutSeconds) * time.Second,
		IdleTimeout:       30 * time.Second,
	}
	return &Server{httpServer: s}, nil
}

func (s *Server) Start(ctx context.Context) error {
	errCh := make(chan error, 1)
	go func() { errCh <- s.httpServer.ListenAndServe() }()
	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return s.httpServer.Shutdown(shutdownCtx)
	case err := <-errCh:
		return err
	}
}
