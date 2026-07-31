package handler

import (
	"context"
	"net/http"
	"strings"
)

type tlsAuthorizer interface {
	AuthorizeTLS(context.Context, string) (bool, error)
}

// CaddyPermissionHandler implements Caddy's internal On-Demand TLS permission
// contract. It deliberately returns no body and never reveals domain state.
type CaddyPermissionHandler struct {
	authorizer tlsAuthorizer
}

// NewCaddyPermissionHandler constructs an indexed DB-backed permission check.
func NewCaddyPermissionHandler(authorizer tlsAuthorizer) *CaddyPermissionHandler {
	return &CaddyPermissionHandler{authorizer: authorizer}
}

// ServeHTTP expects Caddy's official ?domain= query parameter. This handler is
// mounted only on the private metrics/internal listener, never the public API.
func (h *CaddyPermissionHandler) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	hostname := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(
		request.URL.Query().Get("domain"),
	), "."))
	if hostname == "" {
		writer.WriteHeader(http.StatusForbidden)
		return
	}
	authorized, err := h.authorizer.AuthorizeTLS(request.Context(), hostname)
	if err != nil {
		writer.Header().Set("Retry-After", "1")
		writer.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	if !authorized {
		writer.WriteHeader(http.StatusForbidden)
		return
	}
	writer.WriteHeader(http.StatusOK)
}
