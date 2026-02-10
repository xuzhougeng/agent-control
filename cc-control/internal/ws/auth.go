package ws

import (
	"net/http"
	"strings"
)

func extractToken(r *http.Request) string {
	h := strings.TrimSpace(r.Header.Get("Authorization"))
	if strings.HasPrefix(strings.ToLower(h), "bearer ") {
		return strings.TrimSpace(h[len("Bearer "):])
	}
	q := strings.TrimSpace(r.URL.Query().Get("token"))
	return q
}
