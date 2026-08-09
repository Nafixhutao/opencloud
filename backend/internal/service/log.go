package service

import (
	"context"
	"database/sql"
	"errors"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/nazxf/opencloud/backend/internal/apperr"
	"github.com/nazxf/opencloud/backend/internal/logs"
	"github.com/nazxf/opencloud/backend/internal/model"
)

const (
	defaultLogLimit       = 200
	defaultLogWindow      = time.Hour
	defaultLiveLookback   = 15 * time.Second
	maximumLogQueryWindow = 7 * 24 * time.Hour
)

var logEnvironmentPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,31}$`)

type logScopeReader interface {
	GetProjectByAccount(context.Context, uuid.UUID, uuid.UUID) (*model.Project, error)
	GetServiceByAccount(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) (*model.Service, error)
	GetDeploymentByAccount(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, uuid.UUID) (*model.Deployment, error)
}

// LogQuery contains optional browser filters. The service replaces every
// tenant identifier with authenticated, ownership-checked values.
type LogQuery struct {
	ServiceID    *uuid.UUID
	DeploymentID *uuid.UUID
	Sources      []logs.Source
	Levels       []logs.Level
	Environment  string
	Search       string
	Start        time.Time
	End          time.Time
	Limit        int
}

// LogService authorizes customer log access and delegates storage to Loki.
type LogService struct {
	scopes    logScopeReader
	store     logs.Store
	sanitizer *logs.Sanitizer
	now       func() time.Time
}

// NewLogService constructs the tenant-safe log boundary.
func NewLogService(scopes logScopeReader, store logs.Store) *LogService {
	return &LogService{scopes: scopes, store: store, sanitizer: logs.NewSanitizer(), now: time.Now}
}

// Query returns a bounded chronological page of sanitized logs.
func (s *LogService) Query(ctx context.Context, accountID, projectID uuid.UUID, query LogQuery) ([]logs.Entry, error) {
	filter, err := s.authorizeAndFilter(ctx, accountID, projectID, query, false)
	if err != nil {
		return nil, err
	}
	rows, err := s.store.Query(ctx, filter)
	if err != nil {
		return nil, mapLogStoreError(err)
	}
	for index := range rows {
		rows[index] = s.sanitizer.Entry(rows[index])
	}
	return rows, nil
}

// Stream returns a sanitized live feed after the same ownership checks as Query.
func (s *LogService) Stream(ctx context.Context, accountID, projectID uuid.UUID, query LogQuery) (logs.Subscription, error) {
	filter, err := s.authorizeAndFilter(ctx, accountID, projectID, query, true)
	if err != nil {
		return logs.Subscription{}, err
	}
	streamCtx, cancel := context.WithCancel(ctx)
	provider, err := s.store.Tail(streamCtx, filter)
	if err != nil {
		cancel()
		return logs.Subscription{}, mapLogStoreError(err)
	}
	entries := make(chan logs.Entry, 128)
	errorsCh := make(chan error, 1)
	var once sync.Once
	closeSubscription := func() {
		once.Do(func() {
			cancel()
			if provider.Close != nil {
				provider.Close()
			}
		})
	}
	go func() {
		defer close(entries)
		defer close(errorsCh)
		defer closeSubscription()
		providerEntries, providerErrors := provider.Entries, provider.Errors
		for providerEntries != nil || providerErrors != nil {
			select {
			case entry, ok := <-providerEntries:
				if !ok {
					providerEntries = nil
					continue
				}
				select {
				case entries <- s.sanitizer.Entry(entry):
				case <-streamCtx.Done():
					return
				}
			case providerErr, ok := <-providerErrors:
				if !ok {
					providerErrors = nil
					continue
				}
				if providerErr == nil {
					continue
				}
				select {
				case errorsCh <- mapLogStoreError(providerErr):
				case <-streamCtx.Done():
				}
				return
			case <-streamCtx.Done():
				return
			}
		}
	}()
	return logs.Subscription{Entries: entries, Errors: errorsCh, Close: closeSubscription}, nil
}

func (s *LogService) authorizeAndFilter(
	ctx context.Context,
	accountID, projectID uuid.UUID,
	query LogQuery,
	live bool,
) (logs.Filter, error) {
	if _, err := s.scopes.GetProjectByAccount(ctx, accountID, projectID); err != nil {
		return logs.Filter{}, mapLogScopeError(err, "project not found")
	}
	if query.ServiceID != nil {
		if _, err := s.scopes.GetServiceByAccount(ctx, accountID, projectID, *query.ServiceID); err != nil {
			return logs.Filter{}, mapLogScopeError(err, "service not found")
		}
	}
	if query.DeploymentID != nil {
		if query.ServiceID == nil {
			return logs.Filter{}, apperr.Validation("service_id is required with deployment_id")
		}
		if _, err := s.scopes.GetDeploymentByAccount(ctx, accountID, projectID, *query.ServiceID, *query.DeploymentID); err != nil {
			return logs.Filter{}, mapLogScopeError(err, "deployment not found")
		}
	}
	now := s.now().UTC()
	end := query.End.UTC()
	if query.End.IsZero() || live {
		end = now
	}
	start := query.Start.UTC()
	if query.Start.IsZero() {
		if live {
			start = now.Add(-defaultLiveLookback)
		} else {
			start = end.Add(-defaultLogWindow)
		}
	}
	if end.After(now.Add(time.Minute)) {
		return logs.Filter{}, apperr.Validation("log end time cannot be in the future")
	}
	if end.Before(start) || end.Sub(start) > maximumLogQueryWindow {
		return logs.Filter{}, apperr.Validation("log time range must be between zero and seven days")
	}
	limit := query.Limit
	if limit == 0 {
		limit = defaultLogLimit
	}
	if limit < 1 || limit > 1000 {
		return logs.Filter{}, apperr.Validation("log limit must be between 1 and 1000")
	}
	environment := strings.ToLower(strings.TrimSpace(query.Environment))
	if environment != "" && !logEnvironmentPattern.MatchString(environment) {
		return logs.Filter{}, apperr.Validation("invalid log environment")
	}
	search := strings.TrimSpace(query.Search)
	if len(search) > 256 {
		return logs.Filter{}, apperr.Validation("log search must be at most 256 characters")
	}
	filter := logs.Filter{
		AccountID: accountID, ProjectID: projectID,
		ServiceID: query.ServiceID, DeploymentID: query.DeploymentID,
		Sources: uniqueSources(query.Sources), Levels: uniqueLevels(query.Levels),
		Environment: environment, Search: search, Start: start, End: end, Limit: limit,
	}
	if err := filter.Validate(); err != nil {
		return logs.Filter{}, apperr.Validation("invalid log filters")
	}
	return filter, nil
}

func mapLogScopeError(err error, message string) error {
	if errors.Is(err, sql.ErrNoRows) {
		return apperr.NotFound(message)
	}
	return apperr.Internal("failed to authorize log scope").Wrap(err)
}

func mapLogStoreError(err error) error {
	if errors.Is(err, logs.ErrUnavailable) {
		return apperr.Unavailable("logs are temporarily unavailable").Wrap(err)
	}
	if errors.Is(err, logs.ErrInvalidFilter) {
		return apperr.Validation("invalid log filters").Wrap(err)
	}
	return apperr.Internal("failed to read logs").Wrap(err)
}

func uniqueSources(values []logs.Source) []logs.Source {
	seen := make(map[logs.Source]struct{}, len(values))
	result := make([]logs.Source, 0, len(values))
	for _, value := range values {
		if !value.Valid() {
			return []logs.Source{"invalid"}
		}
		if _, exists := seen[value]; !exists {
			seen[value] = struct{}{}
			result = append(result, value)
		}
	}
	return result
}

func uniqueLevels(values []logs.Level) []logs.Level {
	seen := make(map[logs.Level]struct{}, len(values))
	result := make([]logs.Level, 0, len(values))
	for _, value := range values {
		if !value.Valid() {
			return []logs.Level{"invalid"}
		}
		if _, exists := seen[value]; !exists {
			seen[value] = struct{}{}
			result = append(result, value)
		}
	}
	return result
}
