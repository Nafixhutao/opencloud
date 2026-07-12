package metrics

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNormalizeMethod(t *testing.T) {
	t.Parallel()

	require.Equal(t, http.MethodGet, normalizeMethod(http.MethodGet))
	require.Equal(t, http.MethodPatch, normalizeMethod(http.MethodPatch))
	require.Equal(t, "OTHER", normalizeMethod("ATTACKER_DEFINED_METHOD"))
}

func TestNormalizeStatusClass(t *testing.T) {
	t.Parallel()

	require.Equal(t, "2xx", normalizeStatusClass(http.StatusOK))
	require.Equal(t, "5xx", normalizeStatusClass(http.StatusInternalServerError))
	require.Equal(t, "other", normalizeStatusClass(0))
}
