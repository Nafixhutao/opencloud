package handler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

type fakeTLSAuthorizer struct {
	authorized bool
	err        error
	hostname   string
}

func (f *fakeTLSAuthorizer) AuthorizeTLS(_ context.Context, hostname string) (bool, error) {
	f.hostname = hostname
	return f.authorized, f.err
}

func TestCaddyPermissionHandlerFailsClosed(t *testing.T) {
	tests := []struct {
		name       string
		domain     string
		expected   string
		authorized bool
		err        error
		status     int
		retryAfter string
	}{
		{name: "active hostname", domain: "WWW.Example.COM.", expected: "www.example.com", authorized: true, status: http.StatusOK},
		{name: "unknown hostname", domain: "unknown.example.com", expected: "unknown.example.com", status: http.StatusForbidden},
		{name: "missing hostname", status: http.StatusForbidden},
		{name: "database failure", domain: "www.example.com", expected: "www.example.com", err: errors.New("database unavailable"), status: http.StatusServiceUnavailable, retryAfter: "1"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			authorizer := &fakeTLSAuthorizer{authorized: test.authorized, err: test.err}
			handler := NewCaddyPermissionHandler(authorizer)
			request := httptest.NewRequest(http.MethodGet, "/caddy/permission?domain="+test.domain, nil)
			response := httptest.NewRecorder()

			handler.ServeHTTP(response, request)

			require.Equal(t, test.status, response.Code)
			require.Empty(t, response.Body.String())
			require.Equal(t, test.retryAfter, response.Header().Get("Retry-After"))
			require.Equal(t, test.expected, authorizer.hostname)
		})
	}
}
