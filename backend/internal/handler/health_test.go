package handler_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/nazxf/opencloud/backend/internal/handler"
)

func init() { gin.SetMode(gin.TestMode) }

func TestLive_AlwaysOK(t *testing.T) {
	h := handler.NewHealth(nil, nil)
	r := gin.New()
	r.GET("/healthz", h.Live)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/healthz", nil))

	require.Equal(t, http.StatusOK, w.Code)

	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.Equal(t, "ok", body["status"])
}

func TestReady_UnavailableWhenDepsUnwired(t *testing.T) {
	// nil db/redis stand in for unreachable dependencies: /readyz must fail
	// closed with 503 rather than reporting ready.
	h := handler.NewHealth(nil, nil)
	r := gin.New()
	r.GET("/readyz", h.Ready)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/readyz", nil))

	require.Equal(t, http.StatusServiceUnavailable, w.Code)

	var body struct {
		Status string            `json:"status"`
		Checks map[string]string `json:"checks"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.Equal(t, "unready", body.Status)
	require.NotEqual(t, "ok", body.Checks["postgres"])
	require.NotEqual(t, "ok", body.Checks["redis"])
}
