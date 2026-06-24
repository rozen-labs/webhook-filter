package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadAndValidate(t *testing.T) {
	t.Setenv("GITHUB_WEBHOOK_SECRET", "secret")
	t.Setenv("TUNNEL_WEBHOOK_TOKEN", "token")
	path := filepath.Join(t.TempDir(), "config.yml")
	data := []byte(`server:
  listen_addr: ":8080"
routes:
  - name: test
    match:
      path: "/github/issues"
      method: POST
    auth:
      all:
        - type: github_signature
          secret_env: GITHUB_WEBHOOK_SECRET
        - type: header_secret
          header: X-Webhook-Token
          secret_env: TUNNEL_WEBHOOK_TOKEN
    conditions:
      expression: headers["X-GitHub-Event"] == "issues"
    forward:
      url: http://example.com/webhook
      method: POST
      timeout_seconds: 5
`)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.Server.RequestTimeoutSeconds != 10 {
		t.Fatalf("default timeout not applied: %d", cfg.Server.RequestTimeoutSeconds)
	}
	if got := cfg.Routes[0].Response.OnFiltered.StatusCode; got != 202 {
		t.Fatalf("filtered default status = %d", got)
	}
}

func TestValidateRejectsDuplicateRoutes(t *testing.T) {
	t.Setenv("GITHUB_WEBHOOK_SECRET", "secret")
	cfg := &Config{
		Server: ServerConfig{ListenAddr: ":8080", RequestTimeoutSeconds: 10, MaxBodyBytes: 1024},
		Routes: []RouteConfig{{
			Name:       "dup",
			Match:      MatchConfig{Path: "/a", Method: "POST"},
			Auth:       AuthConfig{Type: "none"},
			Conditions: ConditionConfig{Expression: "true"},
			Forward:    ForwardConfig{URL: "http://example.com", Method: "POST", TimeoutSeconds: 5},
		}, {
			Name:       "dup",
			Match:      MatchConfig{Path: "/b", Method: "POST"},
			Auth:       AuthConfig{Type: "none"},
			Conditions: ConditionConfig{Expression: "true"},
			Forward:    ForwardConfig{URL: "http://example.com", Method: "POST", TimeoutSeconds: 5},
		}},
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected duplicate route error")
	}
}
