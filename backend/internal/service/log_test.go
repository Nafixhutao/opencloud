package service

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/nazxf/opencloud/backend/internal/apperr"
	"github.com/nazxf/opencloud/backend/internal/logs"
	"github.com/nazxf/opencloud/backend/internal/model"
)

type fakeLogScopes struct {
	projectAccount uuid.UUID
	projectID      uuid.UUID
	serviceID      uuid.UUID
	deploymentID   uuid.UUID
	projectErr     error
	serviceErr     error
	deploymentErr  error
}

func (f *fakeLogScopes) GetProjectByAccount(_ context.Context, accountID, projectID uuid.UUID) (*model.Project, error) {
	f.projectAccount, f.projectID = accountID, projectID
	return &model.Project{ID: projectID, AccountID: accountID}, f.projectErr
}
func (f *fakeLogScopes) GetServiceByAccount(_ context.Context, accountID, projectID, serviceID uuid.UUID) (*model.Service, error) {
	f.projectAccount, f.projectID, f.serviceID = accountID, projectID, serviceID
	return &model.Service{ID: serviceID, AccountID: accountID, ProjectID: projectID}, f.serviceErr
}
func (f *fakeLogScopes) GetDeploymentByAccount(_ context.Context, accountID, projectID, serviceID, deploymentID uuid.UUID) (*model.Deployment, error) {
	f.projectAccount, f.projectID, f.serviceID, f.deploymentID = accountID, projectID, serviceID, deploymentID
	return &model.Deployment{ID: deploymentID, AccountID: accountID, ProjectID: projectID, ServiceID: serviceID}, f.deploymentErr
}

type fakeLogStore struct {
	filter logs.Filter
	rows   []logs.Entry
	err    error
}

func (f *fakeLogStore) Query(_ context.Context, filter logs.Filter) ([]logs.Entry, error) {
	f.filter = filter
	return append([]logs.Entry(nil), f.rows...), f.err
}
func (f *fakeLogStore) Tail(_ context.Context, filter logs.Filter) (logs.Subscription, error) {
	f.filter = filter
	if f.err != nil {
		return logs.Subscription{}, f.err
	}
	entries := make(chan logs.Entry, len(f.rows))
	errorsCh := make(chan error)
	for _, row := range f.rows {
		entries <- row
	}
	close(entries)
	close(errorsCh)
	return logs.Subscription{Entries: entries, Errors: errorsCh, Close: func() {}}, nil
}

func TestLogServiceUsesAuthenticatedTenantAndSanitizesRows(t *testing.T) {
	accountID, projectID, serviceID := uuid.New(), uuid.New(), uuid.New()
	scopes := &fakeLogScopes{}
	store := &fakeLogStore{rows: []logs.Entry{{
		Timestamp: time.Now(), Source: logs.SourceRuntime,
		Message: "password=hunter2", Request: &logs.RequestMetadata{Path: "/callback?token=secret"},
	}}}
	svc := NewLogService(scopes, store)
	svc.now = func() time.Time { return time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC) }
	rows, err := svc.Query(context.Background(), accountID, projectID, LogQuery{ServiceID: &serviceID})
	require.NoError(t, err)
	require.Equal(t, accountID, store.filter.AccountID)
	require.Equal(t, projectID, store.filter.ProjectID)
	require.Equal(t, serviceID, *store.filter.ServiceID)
	require.Equal(t, "password=[REDACTED]", rows[0].Message)
	require.Equal(t, "/callback", rows[0].Request.Path)
}

func TestLogServiceNeverQueriesStoreForAnotherTenantProject(t *testing.T) {
	scopes := &fakeLogScopes{projectErr: sql.ErrNoRows}
	store := &fakeLogStore{}
	svc := NewLogService(scopes, store)
	_, err := svc.Query(context.Background(), uuid.New(), uuid.New(), LogQuery{})
	require.Error(t, err)
	require.Equal(t, 404, apperr.As(err).Status)
	require.Equal(t, uuid.Nil, store.filter.AccountID)
}

func TestLogServiceRequiresServiceForDeploymentFilter(t *testing.T) {
	svc := NewLogService(&fakeLogScopes{}, &fakeLogStore{})
	deploymentID := uuid.New()
	_, err := svc.Query(context.Background(), uuid.New(), uuid.New(), LogQuery{DeploymentID: &deploymentID})
	require.Error(t, err)
	require.Equal(t, 422, apperr.As(err).Status)
}

func TestLogServiceMapsUnavailableStore(t *testing.T) {
	svc := NewLogService(&fakeLogScopes{}, logs.UnavailableStore{})
	_, err := svc.Query(context.Background(), uuid.New(), uuid.New(), LogQuery{})
	require.Error(t, err)
	require.Equal(t, 503, apperr.As(err).Status)
}

func TestLogServiceStreamSanitizesEntries(t *testing.T) {
	store := &fakeLogStore{rows: []logs.Entry{{Timestamp: time.Now(), Source: logs.SourceRuntime, Message: "api_key=secret"}}}
	svc := NewLogService(&fakeLogScopes{}, store)
	subscription, err := svc.Stream(context.Background(), uuid.New(), uuid.New(), LogQuery{})
	require.NoError(t, err)
	defer subscription.Close()
	entry, ok := <-subscription.Entries
	require.True(t, ok)
	require.Equal(t, "api_key=[REDACTED]", entry.Message)
}
