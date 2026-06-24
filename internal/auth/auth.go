package auth

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/rozen-labs/webhook-filter/internal/config"
)

func Authenticate(rule config.AuthConfig, req *http.Request, rawBody []byte) error {
	if len(rule.All) > 0 {
		for _, sub := range rule.All {
			if err := Authenticate(sub, req, rawBody); err != nil {
				return err
			}
		}
		return nil
	}
	if len(rule.Any) > 0 {
		var errs []string
		for _, sub := range rule.Any {
			if err := Authenticate(sub, req, rawBody); err == nil {
				return nil
			} else {
				errs = append(errs, err.Error())
			}
		}
		return fmt.Errorf("any auth rule failed: %s", strings.Join(errs, "; "))
	}

	switch rule.Type {
	case "none":
		return nil
	case "header_secret":
		return checkHeaderSecret(req.Header.Get(rule.Header), os.Getenv(rule.SecretEnv))
	case "bearer":
		auth := req.Header.Get("Authorization")
		if !strings.HasPrefix(auth, "Bearer ") {
			return fmt.Errorf("missing bearer token")
		}
		return checkHeaderSecret(strings.TrimPrefix(auth, "Bearer "), os.Getenv(rule.SecretEnv))
	case "github_signature":
		if rule.SignatureHeader == "" {
			rule.SignatureHeader = "X-Hub-Signature-256"
		}
		if rule.SignaturePrefix == "" {
			rule.SignaturePrefix = "sha256="
		}
		return verifyHMAC(req.Header.Get(rule.SignatureHeader), os.Getenv(rule.SecretEnv), rawBody, rule.SignaturePrefix)
	case "hmac_sha256":
		return verifyHMAC(req.Header.Get(rule.SignatureHeader), os.Getenv(rule.SecretEnv), rawBody, rule.SignaturePrefix)
	default:
		return fmt.Errorf("unsupported auth type %q", rule.Type)
	}
}

func checkHeaderSecret(got, expected string) error {
	if subtle.ConstantTimeCompare([]byte(got), []byte(expected)) != 1 {
		return fmt.Errorf("unauthorized")
	}
	return nil
}

func verifyHMAC(headerValue, secret string, body []byte, prefix string) error {
	if secret == "" {
		return fmt.Errorf("missing secret")
	}
	if !strings.HasPrefix(headerValue, prefix) {
		return fmt.Errorf("invalid signature prefix")
	}
	providedHex := strings.TrimPrefix(headerValue, prefix)
	provided, err := hex.DecodeString(providedHex)
	if err != nil {
		return fmt.Errorf("invalid signature encoding")
	}
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(body)
	expected := mac.Sum(nil)
	if !bytes.Equal(expected, provided) {
		return fmt.Errorf("unauthorized")
	}
	return nil
}
