package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNormalizeCustomHostname(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		expected  string
		wantError bool
	}{
		{name: "canonical ASCII", input: " WWW.Example.COM. ", expected: "www.example.com"},
		{name: "unicode is converted to IDNA", input: "bücher.de", expected: "xn--bcher-kva.de"},
		{name: "private suffix subdomain", input: "tenant.github.io", expected: "tenant.github.io"},
		{name: "URL", input: "https://example.com", wantError: true},
		{name: "path", input: "example.com/path", wantError: true},
		{name: "port", input: "example.com:443", wantError: true},
		{name: "wildcard", input: "*.example.com", wantError: true},
		{name: "IPv4", input: "1.2.3.4", wantError: true},
		{name: "IPv6", input: "[2001:db8::1]", wantError: true},
		{name: "localhost", input: "localhost", wantError: true},
		{name: "localhost suffix", input: "site.localhost", wantError: true},
		{name: "public suffix only", input: "co.uk", wantError: true},
		{name: "private public suffix only", input: "github.io", wantError: true},
		{name: "unknown suffix", input: "example.invalid", wantError: true},
		{name: "empty label", input: "www..example.com", wantError: true},
		{name: "underscore", input: "bad_name.example.com", wantError: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			actual, err := normalizeCustomHostname(test.input)
			if test.wantError {
				require.Error(t, err)
				require.Empty(t, actual)
				return
			}
			require.NoError(t, err)
			require.Equal(t, test.expected, actual)
		})
	}
}

func TestValidatePrimarySiteDomain(t *testing.T) {
	require.NoError(t, validatePrimarySiteDomain(
		"tenant.sites.example.com",
		"sites.example.com",
	))
	require.Error(t, validatePrimarySiteDomain(
		"sites.example.com",
		"sites.example.com",
	))
	require.Error(t, validatePrimarySiteDomain(
		"customer.example.net",
		"sites.example.com",
	))
	require.NoError(t, validatePrimarySiteDomain(
		"customer.example.net",
		"",
	), "an empty suffix remains available only to development and direct tests")
}
