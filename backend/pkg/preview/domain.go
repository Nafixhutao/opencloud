// Package preview generates preview deployment domains.
package preview

import "fmt"

// GenerateDomain returns a temporary preview domain.
func GenerateDomain(previewID, suffix string) string {
	if suffix == "" {
		suffix = "preview.localhost"
	}
	return fmt.Sprintf("pr-%s.%s", previewID[:8], suffix)
}
