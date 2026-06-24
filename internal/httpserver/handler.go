package httpserver

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/rozen-labs/webhook-filter/internal/auth"
	"github.com/rozen-labs/webhook-filter/internal/config"
	"github.com/rozen-labs/webhook-filter/internal/expression"
	"github.com/rozen-labs/webhook-filter/internal/forwarder"
	"github.com/rozen-labs/webhook-filter/internal/matcher"
	"github.com/rozen-labs/webhook-filter/internal/metrics"
)

type Handler struct {
	cfg *config.Config
	log *slog.Logger
	fwd *forwarder.Forwarder
}

func NewHandler(cfg *config.Config, logger *slog.Logger, fwd *forwarder.Forwarder) *Handler {
	return &Handler{cfg: cfg, log: logger, fwd: fwd}
}

func (h *Handler) Healthz(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	if r.URL.Path == "/healthz" {
		h.Healthz(w, r)
		return
	}

	var route *config.RouteConfig
	for i := range h.cfg.Routes {
		if matcher.Match(r, h.cfg.Routes[i].Match) {
			route = &h.cfg.Routes[i]
			break
		}
	}
	if route == nil {
		h.writeText(w, http.StatusNotFound, "not found")
		metrics.RequestsTotal.WithLabelValues("", "404", r.Method).Inc()
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, h.cfg.Server.MaxBodyBytes)
	rawBody, err := io.ReadAll(r.Body)
	if err != nil {
		if strings.Contains(err.Error(), "request body too large") {
			h.writeText(w, http.StatusRequestEntityTooLarge, "request too large")
			metrics.RequestsTotal.WithLabelValues(route.Name, "413", r.Method).Inc()
			metrics.RequestDuration.WithLabelValues(route.Name, "413", r.Method).Observe(time.Since(start).Seconds())
			return
		}
		h.writeText(w, http.StatusBadRequest, "bad request")
		return
	}
	if int64(len(rawBody)) > h.cfg.Server.MaxBodyBytes {
		h.writeText(w, http.StatusRequestEntityTooLarge, "request too large")
		metrics.RequestsTotal.WithLabelValues(route.Name, "413", r.Method).Inc()
		metrics.RequestDuration.WithLabelValues(route.Name, "413", r.Method).Observe(time.Since(start).Seconds())
		return
	}

	if err := auth.Authenticate(route.Auth, r, rawBody); err != nil {
		h.log.Info("auth failed", slog.String("route", route.Name), slog.String("error", err.Error()))
		h.writeText(w, http.StatusUnauthorized, "unauthorized")
		metrics.AuthFailedTotal.WithLabelValues(route.Name, r.Method).Inc()
		metrics.RequestsTotal.WithLabelValues(route.Name, "401", r.Method).Inc()
		metrics.RequestDuration.WithLabelValues(route.Name, "401", r.Method).Observe(time.Since(start).Seconds())
		return
	}

	var body any
	if err := json.Unmarshal(rawBody, &body); err != nil {
		h.writeText(w, http.StatusBadRequest, "invalid json")
		metrics.RequestsTotal.WithLabelValues(route.Name, "400", r.Method).Inc()
		metrics.RequestDuration.WithLabelValues(route.Name, "400", r.Method).Observe(time.Since(start).Seconds())
		return
	}

	vars := map[string]any{
		"method":  r.Method,
		"path":    r.URL.Path,
		"query":   queryToMap(r),
		"headers": headersToMap(r.Header),
		"body":    body,
		"config":  route.Config,
	}
	passed, err := expression.Evaluate(route.Conditions.Expression, vars)
	if err != nil {
		h.log.Error("condition evaluation failed", slog.String("route", route.Name), slog.String("error", err.Error()))
		h.writeText(w, http.StatusBadRequest, "invalid condition")
		metrics.RequestsTotal.WithLabelValues(route.Name, "400", r.Method).Inc()
		metrics.RequestDuration.WithLabelValues(route.Name, "400", r.Method).Observe(time.Since(start).Seconds())
		return
	}
	if !passed {
		h.writeFiltered(w, route)
		metrics.FilteredTotal.WithLabelValues(route.Name, r.Method).Inc()
		metrics.RequestsTotal.WithLabelValues(route.Name, fmt.Sprintf("%d", route.Response.OnFiltered.StatusCode), r.Method).Inc()
		metrics.RequestDuration.WithLabelValues(route.Name, fmt.Sprintf("%d", route.Response.OnFiltered.StatusCode), r.Method).Observe(time.Since(start).Seconds())
		return
	}

	result, err := h.fwd.Forward(r.Context(), r, rawBody, *route)
	if err != nil {
		h.log.Error("forward failed", slog.String("route", route.Name), slog.String("error", err.Error()))
		h.writeText(w, route.Response.OnForwardError.StatusCode, route.Response.OnForwardError.Body)
		metrics.ForwardErrorsTotal.WithLabelValues(route.Name, r.Method).Inc()
		metrics.RequestsTotal.WithLabelValues(route.Name, fmt.Sprintf("%d", route.Response.OnForwardError.StatusCode), r.Method).Inc()
		metrics.RequestDuration.WithLabelValues(route.Name, fmt.Sprintf("%d", route.Response.OnForwardError.StatusCode), r.Method).Observe(time.Since(start).Seconds())
		return
	}

	metrics.ForwardedTotal.WithLabelValues(route.Name, r.Method).Inc()
	metrics.RequestsTotal.WithLabelValues(route.Name, fmt.Sprintf("%d", result.StatusCode), r.Method).Inc()
	metrics.RequestDuration.WithLabelValues(route.Name, fmt.Sprintf("%d", result.StatusCode), r.Method).Observe(time.Since(start).Seconds())
	switch route.Response.OnForwardSuccess.Mode {
	case "static":
		h.writeText(w, route.Response.OnForwardSuccess.StatusCode, route.Response.OnForwardSuccess.Body)
	case "proxy":
		for k, values := range result.Header {
			if strings.EqualFold(k, "Content-Length") || strings.EqualFold(k, "Transfer-Encoding") {
				continue
			}
			for _, v := range values {
				w.Header().Add(k, v)
			}
		}
		w.WriteHeader(result.StatusCode)
		_, _ = w.Write(result.Body)
	default:
		h.writeText(w, http.StatusInternalServerError, "invalid response mode")
	}
}

func (h *Handler) writeFiltered(w http.ResponseWriter, route *config.RouteConfig) {
	h.writeText(w, route.Response.OnFiltered.StatusCode, route.Response.OnFiltered.Body)
}

func (h *Handler) writeText(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(body))
}

func queryToMap(r *http.Request) map[string]any {
	m := map[string]any{}
	for k, v := range r.URL.Query() {
		if len(v) == 1 {
			m[k] = v[0]
		} else {
			vals := make([]any, 0, len(v))
			for _, item := range v {
				vals = append(vals, item)
			}
			m[k] = vals
		}
	}
	return m
}

func headersToMap(hdr http.Header) map[string]any {
	m := map[string]any{}
	for k, v := range hdr {
		var val any
		if len(v) == 1 {
			val = v[0]
		} else if len(v) > 1 {
			vals := make([]any, 0, len(v))
			for _, item := range v {
				vals = append(vals, item)
			}
			val = vals
		}
		for _, alias := range headerAliases(k) {
			m[alias] = val
		}
	}
	return m
}

func headerAliases(k string) []string {
	aliases := []string{k, http.CanonicalHeaderKey(k), strings.ToLower(k), strings.ReplaceAll(k, "Github", "GitHub"), strings.ReplaceAll(k, "github", "GitHub"), strings.ReplaceAll(k, "Oauth", "OAuth"), strings.ReplaceAll(k, "oauth", "OAuth")}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(aliases))
	for _, a := range aliases {
		if a == "" {
			continue
		}
		if _, ok := seen[a]; ok {
			continue
		}
		seen[a] = struct{}{}
		out = append(out, a)
	}
	return out
}
