package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/rozen-labs/webhook-filter/internal/config"
)

func TestAuthenticateHeaderSecret(t *testing.T) {
	t.Setenv("WEBHOOK_TOKEN", "secret")
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{}`))
	req.Header.Set("X-Webhook-Token", "secret")
	if err := Authenticate(config.AuthConfig{Type: "header_secret", Header: "X-Webhook-Token", SecretEnv: "WEBHOOK_TOKEN"}, req, []byte(`{}`)); err != nil {
		t.Fatalf("authenticate: %v", err)
	}
}

func TestAuthenticateBearer(t *testing.T) {
	t.Setenv("WEBHOOK_TOKEN", "secret")
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{}`))
	req.Header.Set("Authorization", "Bearer secret")
	if err := Authenticate(config.AuthConfig{Type: "bearer", SecretEnv: "WEBHOOK_TOKEN"}, req, []byte(`{}`)); err != nil {
		t.Fatalf("authenticate: %v", err)
	}
}

func TestAuthenticateDefaultNone(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{}`))
	if err := Authenticate(config.AuthConfig{}, req, []byte(`{}`)); err != nil {
		t.Fatalf("authenticate: %v", err)
	}
}

func TestAuthenticateGitHubSignature(t *testing.T) {
	t.Setenv("GITHUB_WEBHOOK_SECRET", "secret")
	body := []byte(`{"action":"labeled"}`)
	mac := hmac.New(sha256.New, []byte("secret"))
	mac.Write(body)
	sig := hex.EncodeToString(mac.Sum(nil))
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(string(body)))
	req.Header.Set("X-Hub-Signature-256", "sha256="+sig)
	if err := Authenticate(config.AuthConfig{Type: "github_signature", SecretEnv: "GITHUB_WEBHOOK_SECRET"}, req, body); err != nil {
		t.Fatalf("authenticate: %v", err)
	}
}
