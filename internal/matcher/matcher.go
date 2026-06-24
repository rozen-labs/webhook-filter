package matcher

import (
	"net/http"
	"strings"

	"github.com/rozen-labs/webhook-filter/internal/config"
)

func Match(r *http.Request, m config.MatchConfig) bool {
	if r.URL.Path != m.Path {
		return false
	}
	return strings.EqualFold(r.Method, m.Method)
}
