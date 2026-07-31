package service

import (
	"net"
	"strings"

	"golang.org/x/net/idna"
	"golang.org/x/net/publicsuffix"

	"github.com/nazxf/opencloud/backend/internal/apperr"
)

func normalizeCustomHostname(value string) (string, error) {
	raw := strings.TrimSpace(value)
	if raw == "" || strings.Contains(raw, "://") || strings.ContainsAny(raw, "/\\?#@:") ||
		strings.HasPrefix(raw, "*.") {
		return "", invalidCustomHostname("must be a hostname without a URL, path, wildcard, or port")
	}

	raw = strings.TrimSuffix(raw, ".")
	if net.ParseIP(strings.Trim(raw, "[]")) != nil || strings.EqualFold(raw, "localhost") ||
		strings.HasSuffix(strings.ToLower(raw), ".localhost") {
		return "", invalidCustomHostname("must be a public DNS hostname, not an IP address or localhost")
	}

	hostname, err := idna.Lookup.ToASCII(raw)
	if err != nil {
		return "", invalidCustomHostname("contains an invalid internationalized DNS label")
	}
	hostname = strings.ToLower(hostname)
	if len(hostname) < 3 || len(hostname) > 253 || !strings.Contains(hostname, ".") ||
		net.ParseIP(hostname) != nil {
		return "", invalidCustomHostname("must be a fully qualified public DNS hostname")
	}
	for _, label := range strings.Split(hostname, ".") {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return "", invalidCustomHostname("contains an invalid DNS label")
		}
		for _, character := range label {
			if (character < 'a' || character > 'z') &&
				(character < '0' || character > '9') && character != '-' {
				return "", invalidCustomHostname("contains an invalid DNS character")
			}
		}
	}

	registrable, err := publicsuffix.EffectiveTLDPlusOne(hostname)
	if err != nil || registrable == "" {
		return "", invalidCustomHostname("must include a registrable name below a public suffix")
	}
	suffix, icann := publicsuffix.PublicSuffix(hostname)
	if !icann && !strings.Contains(suffix, ".") {
		return "", invalidCustomHostname("uses an unrecognized public suffix")
	}
	return hostname, nil
}

func invalidCustomHostname(issue string) error {
	return apperr.Validation(
		"invalid hostname",
		apperr.FieldIssue{Field: "hostname", Issue: issue},
	)
}
