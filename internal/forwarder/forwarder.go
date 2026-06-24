package forwarder

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/rozen-labs/webhook-filter/internal/config"
)

var hopByHop = map[string]struct{}{
	"Connection":          {},
	"Keep-Alive":          {},
	"Proxy-Authenticate":  {},
	"Proxy-Authorization": {},
	"TE":                  {},
	"Trailer":             {},
	"Transfer-Encoding":   {},
	"Upgrade":             {},
}

type Result struct {
	StatusCode int
	Body       []byte
	Header     http.Header
}

type Forwarder struct {
	client *http.Client
}

func New() *Forwarder { return &Forwarder{client: &http.Client{}} }

func (f *Forwarder) Forward(ctx context.Context, incoming *http.Request, rawBody []byte, route config.RouteConfig) (*Result, error) {
	if _, err := url.Parse(route.Forward.URL); err != nil {
		return nil, err
	}
	var bodyReader *bytes.Reader
	if route.Forward.PreserveBody != nil && *route.Forward.PreserveBody {
		bodyReader = bytes.NewReader(rawBody)
	} else {
		bodyReader = bytes.NewReader(nil)
	}
	outReq, err := http.NewRequestWithContext(ctx, strings.ToUpper(route.Forward.Method), route.Forward.URL, bodyReader)
	if err != nil {
		return nil, err
	}
	if route.Forward.PreserveBody != nil && *route.Forward.PreserveBody {
		outReq.ContentLength = int64(len(rawBody))
	}
	for _, name := range route.Forward.PreserveHeaders {
		canon := http.CanonicalHeaderKey(name)
		if _, blocked := hopByHop[canon]; blocked {
			continue
		}
		for _, value := range incoming.Header.Values(name) {
			outReq.Header.Add(canon, value)
		}
	}
	for k, v := range route.Forward.AddHeaders {
		canon := http.CanonicalHeaderKey(k)
		if _, blocked := hopByHop[canon]; blocked {
			continue
		}
		outReq.Header.Set(canon, v)
	}
	for k := range hopByHop {
		outReq.Header.Del(k)
	}

	client := *f.client
	client.Timeout = time.Duration(route.Forward.TimeoutSeconds) * time.Second
	resp, err := client.Do(outReq)
	if err != nil {
		return nil, fmt.Errorf("forward request failed: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return &Result{StatusCode: resp.StatusCode, Body: body, Header: resp.Header.Clone()}, nil
}
