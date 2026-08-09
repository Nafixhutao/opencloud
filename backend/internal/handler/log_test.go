package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/nazxf/opencloud/backend/internal/logs"
	"github.com/nazxf/opencloud/backend/internal/service"
)

type fakeHandlerLogService struct {
	accountID uuid.UUID
	projectID uuid.UUID
	query     service.LogQuery
	rows      []logs.Entry
}

func (f *fakeHandlerLogService) Query(_ context.Context, accountID, projectID uuid.UUID, query service.LogQuery) ([]logs.Entry, error) {
	f.accountID, f.projectID, f.query = accountID, projectID, query
	return f.rows, nil
}
func (f *fakeHandlerLogService) Stream(_ context.Context, accountID, projectID uuid.UUID, query service.LogQuery) (logs.Subscription, error) {
	f.accountID, f.projectID, f.query = accountID, projectID, query
	entries := make(chan logs.Entry, len(f.rows))
	errorsCh := make(chan error)
	for _, row := range f.rows {
		entries <- row
	}
	close(entries)
	close(errorsCh)
	return logs.Subscription{Entries: entries, Errors: errorsCh, Close: func() {}}, nil
}

func TestLogHandlerUsesAuthenticatedAccountInsteadOfQuery(t *testing.T) {
	gin.SetMode(gin.TestMode)
	accountID, projectID, serviceID := uuid.New(), uuid.New(), uuid.New()
	svc := &fakeHandlerLogService{}
	handler := NewLogHandler(svc)
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set("account_id", accountID); c.Next() })
	router.GET("/projects/:projectID/logs", handler.List)

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/projects/"+projectID.String()+"/logs?account_id="+uuid.NewString()+"&service_id="+serviceID.String()+"&source=runtime,request&limit=50", nil)
	router.ServeHTTP(response, request)
	require.Equal(t, http.StatusOK, response.Code)
	require.Equal(t, accountID, svc.accountID)
	require.Equal(t, projectID, svc.projectID)
	require.Equal(t, serviceID, *svc.query.ServiceID)
	require.Equal(t, []logs.Source{logs.SourceRuntime, logs.SourceRequest}, svc.query.Sources)
	require.Equal(t, 50, svc.query.Limit)
}

func TestLogHandlerStreamsSSEWithoutTenantMetadata(t *testing.T) {
	gin.SetMode(gin.TestMode)
	accountID, projectID := uuid.New(), uuid.New()
	timestamp := time.Date(2026, 8, 9, 12, 0, 0, 123, time.UTC)
	svc := &fakeHandlerLogService{rows: []logs.Entry{{Timestamp: timestamp, Source: logs.SourceRuntime, Level: logs.LevelInfo, Message: "ready"}}}
	handler := NewLogHandler(svc)
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set("account_id", accountID); c.Next() })
	router.GET("/projects/:projectID/logs/stream", handler.Stream)

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/projects/"+projectID.String()+"/logs/stream", nil))
	require.Equal(t, http.StatusOK, response.Code)
	require.Equal(t, "text/event-stream", response.Header().Get("Content-Type"))
	require.Contains(t, response.Body.String(), "event: log")
	require.Contains(t, response.Body.String(), `"message":"ready"`)
	require.NotContains(t, response.Body.String(), accountID.String())
	require.True(t, strings.HasPrefix(response.Body.String(), "retry: 3000"))
}

func TestLogHandlerRejectsInvalidFilter(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := &fakeHandlerLogService{}
	router := gin.New()
	router.GET("/projects/:projectID/logs", NewLogHandler(svc).List)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/projects/"+uuid.NewString()+"/logs?service_id=not-a-uuid", nil))
	require.Equal(t, http.StatusUnprocessableEntity, response.Code)
	require.Equal(t, uuid.Nil, svc.accountID)
}
