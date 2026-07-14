package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestValidRequestID(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		id   string
		want bool
	}{
		{name: "uuid", id: "0190d8e5-8f7a-7c31-a5de-9b113817a31d", want: true},
		{name: "trace id", id: "edge:trace_123.abc", want: true},
		{name: "empty", id: "", want: false},
		{name: "whitespace", id: "not trusted", want: false},
		{name: "oversized", id: strings.Repeat("a", maxRequestIDLength+1), want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.want, validRequestID(tt.id))
		})
	}
}

func TestRecoveryReturnsContractErrorAndLogsStack(t *testing.T) {
	gin.SetMode(gin.TestMode)
	core, logs := observer.New(zap.ErrorLevel)
	r := gin.New()
	r.Use(RequestID(), Recovery(zap.New(core)))
	r.GET("/panic", func(_ *gin.Context) { panic("boom") })

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/panic", nil))

	require.Equal(t, http.StatusInternalServerError, rec.Code)
	require.Contains(t, rec.Body.String(), `"code":"INTERNAL"`)
	entries := logs.FilterMessage("panic recovered").All()
	require.Len(t, entries, 1)
	require.NotEmpty(t, entries[0].ContextMap()["stack"])
}
