package logs

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestLokiQueryAlwaysCarriesTrustedTenantScope(t *testing.T) {
	accountID, projectID, serviceID := uuid.New(), uuid.New(), uuid.New()
	var receivedQuery string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedQuery = r.URL.Query().Get("query")
		require.Equal(t, "backward", r.URL.Query().Get("direction"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"streams","result":[{"stream":{"opencloud_source":"request","opencloud_service_id":"` + serviceID.String() + `","opencloud_environment":"production","level":"info"},"values":[["1786248000000000000","{\"message\":\"served\",\"request_id\":\"req-1\",\"method\":\"GET\",\"path\":\"/orders?token=secret\",\"status\":200,\"duration_ms\":12.5,\"response_size\":42}"]]}]}}`))
	}))
	defer server.Close()

	store, err := NewLokiStore(server.URL, server.Client(), time.Millisecond)
	require.NoError(t, err)
	rows, err := store.Query(context.Background(), Filter{
		AccountID: accountID, ProjectID: projectID, ServiceID: &serviceID,
		Sources: []Source{SourceRequest}, Search: `needle" } |= "everything`,
		Start: time.Date(2026, 8, 9, 0, 0, 0, 0, time.UTC),
		End:   time.Date(2026, 8, 9, 1, 0, 0, 0, time.UTC), Limit: 100,
	})
	require.NoError(t, err)
	require.Contains(t, receivedQuery, `opencloud_account_id="`+accountID.String()+`"`)
	require.Contains(t, receivedQuery, `opencloud_project_id="`+projectID.String()+`"`)
	require.Contains(t, receivedQuery, `opencloud_service_id="`+serviceID.String()+`"`)
	require.Contains(t, receivedQuery, `|= "needle\" } |= \"everything"`)
	require.Len(t, rows, 1)
	require.Equal(t, SourceRequest, rows[0].Source)
	require.Equal(t, "served", rows[0].Message)
	require.Equal(t, serviceID, *rows[0].ServiceID)
	require.Equal(t, 200, rows[0].Request.Status)
}

func TestLokiRejectsUnscopedFilterBeforeRequest(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls++ }))
	defer server.Close()
	store, err := NewLokiStore(server.URL, server.Client(), time.Millisecond)
	require.NoError(t, err)
	_, err = store.Query(context.Background(), Filter{Start: time.Now(), End: time.Now(), Limit: 10})
	require.ErrorIs(t, err, ErrInvalidFilter)
	require.Zero(t, calls)
}

func TestLokiResponseDoesNotExposeArbitraryStructuredFields(t *testing.T) {
	accountID, projectID := uuid.New(), uuid.New()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		payload := map[string]any{
			"status": "success",
			"data": map[string]any{"resultType": "streams", "result": []any{map[string]any{
				"stream": map[string]string{"opencloud_source": "runtime", "password": "never-return"},
				"values": [][]string{{"1786248000000000000", `{"message":"password=hunter2","authorization":"Bearer secret","cookie":"private"}`}},
			}}},
		}
		require.NoError(t, json.NewEncoder(w).Encode(payload))
	}))
	defer server.Close()
	store, err := NewLokiStore(server.URL, server.Client(), time.Millisecond)
	require.NoError(t, err)
	rows, err := store.Query(context.Background(), Filter{
		AccountID: accountID, ProjectID: projectID,
		Start: time.Now().Add(-time.Minute), End: time.Now(), Limit: 10,
	})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	row := NewSanitizer().Entry(rows[0])
	require.Equal(t, "password=[REDACTED]", row.Message)
	require.Nil(t, row.Request)
	raw, err := json.Marshal(row)
	require.NoError(t, err)
	require.NotContains(t, string(raw), "never-return")
	require.NotContains(t, string(raw), "cookie")
}

func TestSanitizerStripsRequestQueryAndCredentials(t *testing.T) {
	sanitizer := NewSanitizer()
	row := sanitizer.Entry(Entry{
		Message: "Authorization: Bearer abc password=hunter2 api_key=xyz",
		Request: &RequestMetadata{Path: "/callback?code=secret#fragment"},
	})
	require.Equal(t, "Authorization: Bearer [REDACTED] password=[REDACTED] api_key=[REDACTED]", row.Message)
	require.Equal(t, "/callback", row.Request.Path)
}
