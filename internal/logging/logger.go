package logging

import (
	"io"
	"log/slog"
	"os"

	"github.com/rozen-labs/webhook-filter/internal/config"
)

func New(cfg config.LoggingConfig) *slog.Logger {
	var handler slog.Handler
	level := slog.LevelInfo
	switch cfg.Level {
	case "debug":
		level = slog.LevelDebug
	case "info":
		level = slog.LevelInfo
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	}
	opts := &slog.HandlerOptions{Level: level}
	var w io.Writer = os.Stdout
	if cfg.Format == "text" {
		handler = slog.NewTextHandler(w, opts)
	} else {
		handler = slog.NewJSONHandler(w, opts)
	}
	return slog.New(handler)
}
