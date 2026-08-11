// Package preview generates preview deployment domains.
package preview

import "fmt"

// GenerateDomain returns a temporary preview domain.
func GenerateDomain(previewID, suffix string) string {
	if suffix == "" {
		suffix = "preview.localhost"
	}
	short := previewID
	if len(short) > 8 {
		short = short[:8]
	}
	return fmt.Sprintf("pr-%s.%s", short, suffix)
}
