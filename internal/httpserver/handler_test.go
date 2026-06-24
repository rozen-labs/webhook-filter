package httpserver

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rozen-labs/webhook-filter/internal/config"
	"github.com/rozen-labs/webhook-filter/internal/forwarder"
)

func TestHandlerFiltersAndForwards(t *testing.T) {
	t.Setenv("GITHUB_WEBHOOK_SECRET", "secret")
	t.Setenv("TUNNEL_WEBHOOK_TOKEN", "token")

	var forwardedBody []byte
	forwardBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		forwardedBody, _ = io.ReadAll(r.Body)
		if r.Header.Get("X-Webhook-Filter") != "passed" {
			t.Fatalf("missing add header")
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("downstream ok"))
	}))
	defer forwardBackend.Close()

	cfg := &config.Config{
		Server:   config.ServerConfig{ListenAddr: ":8080", RequestTimeoutSeconds: 10, MaxBodyBytes: 1024},
		Logging:  config.LoggingConfig{Level: "info", Format: "json"},
		Security: config.SecurityConfig{DefaultFilteredStatusCode: 202, DefaultFilteredBody: "ignored"},
		Routes: []config.RouteConfig{{
			Name:       "github-filter",
			Match:      config.MatchConfig{Path: "/github/issues", Method: http.MethodPost},
			Auth:       config.AuthConfig{All: []config.AuthConfig{{Type: "github_signature", SecretEnv: "GITHUB_WEBHOOK_SECRET"}, {Type: "header_secret", Header: "X-Webhook-Token", SecretEnv: "TUNNEL_WEBHOOK_TOKEN"}}},
			Conditions: config.ConditionConfig{Expression: `headers["X-GitHub-Event"] == "issues" && body.action == "labeled" && body.label.name == config.required_label && body.sender.login in config.authorized_users`},
			Config:     map[string]any{"required_label": "deploy-approved", "authorized_users": []any{"alice", "bob"}},
			Forward:    config.ForwardConfig{URL: forwardBackend.URL, Method: http.MethodPost, PreserveBody: boolPtr(true), PreserveHeaders: []string{"Content-Type", "X-GitHub-Event"}, AddHeaders: map[string]string{"X-Webhook-Filter": "passed"}, TimeoutSeconds: 5},
			Response:   config.ResponseConfig{OnFiltered: config.ResponseMessage{StatusCode: 202, Body: "ignored"}, OnForwardSuccess: config.ForwardSuccessMode{Mode: "proxy"}, OnForwardError: config.ResponseMessage{StatusCode: 502, Body: "forward failed"}},
		}},
	}
	h := NewHandler(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)), forwarder.New())

	body := map[string]any{"action": "labeled", "label": map[string]any{"name": "deploy-approved"}, "sender": map[string]any{"login": "alice"}}
	bodyBytes, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/github/issues", bytes.NewReader(bodyBytes))
	req.Header.Set("X-GitHub-Event", "issues")
	req.Header.Set("X-Webhook-Token", "token")
	req.Header.Set("X-Hub-Signature-256", hmacSignature("secret", bodyBytes))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected proxy status 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if string(forwardedBody) != string(bodyBytes) {
		t.Fatalf("forwarded body mismatch: %s", string(forwardedBody))
	}
}

func TestHandlerFiltersWhenConditionFalse(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{ListenAddr: ":8080", RequestTimeoutSeconds: 10, MaxBodyBytes: 1024},
		Routes: []config.RouteConfig{{
			Name:       "github-filter",
			Match:      config.MatchConfig{Path: "/github/issues", Method: http.MethodPost},
			Auth:       config.AuthConfig{Type: "none"},
			Conditions: config.ConditionConfig{Expression: `false`},
			Forward:    config.ForwardConfig{URL: "http://example.com", Method: http.MethodPost, TimeoutSeconds: 5},
			Response:   config.ResponseConfig{OnFiltered: config.ResponseMessage{StatusCode: 202, Body: "ignored"}, OnForwardSuccess: config.ForwardSuccessMode{Mode: "proxy"}, OnForwardError: config.ResponseMessage{StatusCode: 502, Body: "forward failed"}},
		}},
	}
	h := NewHandler(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)), forwarder.New())
	req := httptest.NewRequest(http.MethodPost, "/github/issues", bytes.NewReader([]byte(`{"action":"labeled"}`)))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 202 {
		t.Fatalf("expected filtered 202, got %d", rec.Code)
	}
}

func hmacSignature(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func boolPtr(v bool) *bool { return &v }
