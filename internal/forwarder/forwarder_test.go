package forwarder

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rozen-labs/webhook-filter/internal/config"
)

func TestForwardPreservesOnlySelectedHeaders(t *testing.T) {
	var gotBody []byte
	var gotAuth string
	var gotEvent string
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		gotBody = b
		gotAuth = r.Header.Get("Authorization")
		gotEvent = r.Header.Get("X-GitHub-Event")
		if r.Header.Get("Connection") != "" {
			t.Fatalf("hop-by-hop header forwarded")
		}
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte("ok"))
	}))
	defer backend.Close()

	f := New()
	req, _ := http.NewRequest(http.MethodPost, "http://example.com/source", nil)
	req.Header.Set("Authorization", "Bearer nope")
	req.Header.Set("X-GitHub-Event", "issues")
	route := config.RouteConfig{Forward: config.ForwardConfig{
		URL:             backend.URL,
		Method:          http.MethodPost,
		PreserveBody:    boolPtr(true),
		PreserveHeaders: []string{"X-GitHub-Event", "Authorization", "Connection"},
		AddHeaders:      map[string]string{"X-Webhook-Filter": "passed"},
		TimeoutSeconds:  5,
	}}
	res, err := f.Forward(context.Background(), req, []byte(`{"hello":"world"}`), route)
	if err != nil {
		t.Fatalf("forward: %v", err)
	}
	if res.StatusCode != http.StatusAccepted {
		t.Fatalf("status = %d", res.StatusCode)
	}
	if string(gotBody) != `{"hello":"world"}` {
		t.Fatalf("body = %s", gotBody)
	}
	if gotAuth != "Bearer nope" || gotEvent != "issues" {
		t.Fatalf("headers not forwarded as expected")
	}
}

func boolPtr(v bool) *bool { return &v }
