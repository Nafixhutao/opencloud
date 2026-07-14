package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/nazxf/opencloud/backend/internal/config"
	"github.com/nazxf/opencloud/backend/internal/metrics"
)

func TestRecoveredPanicIsObserved(t *testing.T) {
	gin.SetMode(gin.TestMode)

	m := metrics.New()
	s := New(&config.Config{HTTPAddr: ":0", MetricsAddr: ":0"}, zap.NewNop(), nil, nil, m)
	r := s.http.Handler.(*gin.Engine)
	r.GET("/panic", func(_ *gin.Context) { panic("boom") })

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/panic", nil))
	require.Equal(t, http.StatusInternalServerError, w.Code)

	families, err := m.Registry().Gather()
	require.NoError(t, err)
	found := false
	for _, family := range families {
		if family.GetName() != "http_requests_total" {
			continue
		}
		for _, metric := range family.GetMetric() {
			labels := make(map[string]string, len(metric.GetLabel()))
			for _, label := range metric.GetLabel() {
				labels[label.GetName()] = label.GetValue()
			}
			if labels["route"] == "/panic" && labels["method"] == http.MethodGet && labels["status"] == "5xx" {
				require.Equal(t, float64(1), metric.GetCounter().GetValue())
				found = true
			}
		}
	}
	require.True(t, found, "recovered panic was not recorded in request metrics")
}

func TestHTTPTimeoutsAreConfigured(t *testing.T) {
	gin.SetMode(gin.TestMode)

	s := New(&config.Config{HTTPAddr: ":0", MetricsAddr: ":0"}, zap.NewNop(), nil, nil, metrics.New())
	require.Equal(t, 10*time.Second, s.http.ReadHeaderTimeout)
	require.Equal(t, 30*time.Second, s.http.ReadTimeout)
	require.Equal(t, 30*time.Second, s.http.WriteTimeout)
	require.Equal(t, 60*time.Second, s.http.IdleTimeout)
	require.Equal(t, 1<<20, s.http.MaxHeaderBytes)
}

func TestMetricsUsesSeparateListener(t *testing.T) {
	gin.SetMode(gin.TestMode)

	s := New(&config.Config{HTTPAddr: ":0", MetricsAddr: ":0"}, zap.NewNop(), nil, nil, metrics.New())

	public := httptest.NewRecorder()
	s.http.Handler.ServeHTTP(public, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	require.Equal(t, http.StatusNotFound, public.Code)

	internal := httptest.NewRecorder()
	s.metrics.Handler.ServeHTTP(internal, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	require.Equal(t, http.StatusOK, internal.Code)

	other := httptest.NewRecorder()
	s.metrics.Handler.ServeHTTP(other, httptest.NewRequest(http.MethodGet, "/not-metrics", nil))
	require.Equal(t, http.StatusNotFound, other.Code)
}
