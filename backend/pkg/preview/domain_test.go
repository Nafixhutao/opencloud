package preview_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/nazxf/opencloud/backend/pkg/preview"
)

func TestGenerateDomain(t *testing.T) {
	t.Run("with-suffix", func(t *testing.T) {
		domain := preview.GenerateDomain("abc12345-6789", "preview.example.com")
		require.Equal(t, "pr-abc12345.preview.example.com", domain)
	})

	t.Run("without-suffix-defaults-to-localhost", func(t *testing.T) {
		domain := preview.GenerateDomain("abc12345-6789", "")
		require.Contains(t, domain, "pr-abc12345")
		require.Contains(t, domain, "preview.localhost")
	})

	t.Run("short-id", func(t *testing.T) {
		domain := preview.GenerateDomain("ab", "suffix.test")
		require.Equal(t, "pr-ab.suffix.test", domain)
	})
}

func TestSanitizeJobID(t *testing.T) {
	domain := preview.GenerateDomain("550e8400-e29b-41d4-a716-446655440000", "test.io")
	require.Equal(t, "pr-550e8400.test.io", domain)
}
