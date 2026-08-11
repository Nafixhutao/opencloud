package logs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

const (
	labelAccountID    = "opencloud_account_id"
	labelProjectID    = "opencloud_project_id"
	labelServiceID    = "opencloud_service_id"
	labelDeploymentID = "opencloud_deployment_id"
	labelEnvironment  = "opencloud_environment"
	labelSource       = "opencloud_source"
	labelLevel        = "level"
	defaultPoll       = 2 * time.Second
	maxResponseBytes  = 8 << 20
)

// LokiStore queries the official Loki HTTP API and implements live tail with
// bounded query_range polling, avoiding a websocket dependency in the API.
type LokiStore struct {
	baseURL      *url.URL
	client       *http.Client
	pollInterval time.Duration
}

// NewLokiStore validates an internal Loki endpoint. Credentials belong in the
// transport or reverse proxy and are never accepted inside the URL.
func NewLokiStore(rawURL string, client *http.Client, pollInterval time.Duration) (*LokiStore, error) {
	endpoint, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || (endpoint.Scheme != "http" && endpoint.Scheme != "https") || endpoint.Host == "" {
		return nil, errors.New("logs Loki URL must be an absolute HTTP(S) URL")
	}
	if endpoint.User != nil || endpoint.RawQuery != "" || endpoint.Fragment != "" {
		return nil, errors.New("logs Loki URL must not contain credentials, a query, or a fragment")
	}
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	if pollInterval <= 0 {
		pollInterval = defaultPoll
	}
	endpoint.Path = strings.TrimRight(endpoint.Path, "/")
	return &LokiStore{baseURL: endpoint, client: client, pollInterval: pollInterval}, nil
}

// Query returns a chronological page of the latest matching entries.
func (s *LokiStore) Query(ctx context.Context, filter Filter) ([]Entry, error) {
	return s.query(ctx, filter, "backward")
}

func (s *LokiStore) query(ctx context.Context, filter Filter, direction string) ([]Entry, error) {
	if err := filter.Validate(); err != nil {
		return nil, err
	}
	queryURL := *s.baseURL
	queryURL.Path += "/loki/api/v1/query_range"
	values := queryURL.Query()
	values.Set("query", logQL(filter))
	values.Set("start", strconv.FormatInt(filter.Start.UnixNano(), 10))
	values.Set("end", strconv.FormatInt(filter.End.UnixNano(), 10))
	values.Set("limit", strconv.Itoa(filter.Limit))
	values.Set("direction", direction)
	queryURL.RawQuery = values.Encode()

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, queryURL.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("create Loki query: %w", err)
	}
	response, err := s.client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("%w: query Loki", ErrUnavailable)
	}
	defer func() {
		_ = response.Body.Close()
	}()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
		return nil, fmt.Errorf("%w: Loki returned status %d", ErrUnavailable, response.StatusCode)
	}
	var payload lokiResponse
	decoder := json.NewDecoder(io.LimitReader(response.Body, maxResponseBytes))
	if err := decoder.Decode(&payload); err != nil {
		return nil, fmt.Errorf("%w: decode Loki response", ErrUnavailable)
	}
	if payload.Status != "success" || payload.Data.ResultType != "streams" {
		return nil, fmt.Errorf("%w: invalid Loki response", ErrUnavailable)
	}
	entries := make([]Entry, 0)
	for _, stream := range payload.Data.Result {
		for _, value := range stream.Values {
			if len(value) != 2 {
				continue
			}
			nanoseconds, err := strconv.ParseInt(value[0], 10, 64)
			if err != nil {
				continue
			}
			entries = append(entries, entryFromLoki(stream.Stream, value[1], time.Unix(0, nanoseconds).UTC()))
		}
	}
	sort.SliceStable(entries, func(i, j int) bool { return entries[i].Timestamp.Before(entries[j].Timestamp) })
	return entries, nil
}

// Tail polls Loki from the last observed nanosecond and fans entries to one
// bounded subscriber. The SSE handler reconnects after failures.
func (s *LokiStore) Tail(ctx context.Context, filter Filter) (Subscription, error) {
	if err := filter.Validate(); err != nil {
		return Subscription{}, err
	}
	tailCtx, cancel := context.WithCancel(ctx)
	entries := make(chan Entry, 128)
	errorsCh := make(chan error, 1)
	var once sync.Once
	closeSubscription := func() { once.Do(cancel) }

	go func() {
		defer close(entries)
		defer close(errorsCh)
		ticker := time.NewTicker(s.pollInterval)
		defer ticker.Stop()
		cursor := filter.Start
		seen := make(map[string]struct{})
		for {
			poll := filter
			poll.Start = cursor
			poll.End = time.Now().UTC()
			if poll.End.Before(poll.Start) {
				poll.End = poll.Start
			}
			rows, err := s.query(tailCtx, poll, "forward")
			if err != nil {
				if tailCtx.Err() == nil {
					errorsCh <- err
				}
				return
			}
			for _, row := range rows {
				key := entryKey(row)
				if _, exists := seen[key]; exists {
					continue
				}
				seen[key] = struct{}{}
				if row.Timestamp.After(cursor) {
					cursor = row.Timestamp
				}
				select {
				case entries <- row:
				case <-tailCtx.Done():
					return
				}
			}
			if len(seen) > 5000 {
				seen = make(map[string]struct{})
			}
			select {
			case <-ticker.C:
			case <-tailCtx.Done():
				return
			}
		}
	}()

	return Subscription{Entries: entries, Errors: errorsCh, Close: closeSubscription}, nil
}

func logQL(filter Filter) string {
	matchers := []string{
		labelAccountID + "=" + strconv.Quote(filter.AccountID.String()),
		labelProjectID + "=" + strconv.Quote(filter.ProjectID.String()),
	}
	if filter.ServiceID != nil {
		matchers = append(matchers, labelServiceID+"="+strconv.Quote(filter.ServiceID.String()))
	}
	if filter.DeploymentID != nil {
		matchers = append(matchers, labelDeploymentID+"="+strconv.Quote(filter.DeploymentID.String()))
	}
	if filter.Environment != "" {
		matchers = append(matchers, labelEnvironment+"="+strconv.Quote(filter.Environment))
	}
	if len(filter.Sources) > 0 {
		values := make([]string, len(filter.Sources))
		for i, source := range filter.Sources {
			values[i] = string(source)
		}
		matchers = append(matchers, labelSource+"=~"+strconv.Quote(strings.Join(values, "|")))
	}
	if len(filter.Levels) > 0 {
		values := make([]string, len(filter.Levels))
		for i, level := range filter.Levels {
			values[i] = string(level)
		}
		matchers = append(matchers, labelLevel+"=~"+strconv.Quote(strings.Join(values, "|")))
	}
	query := "{" + strings.Join(matchers, ",") + "}"
	if filter.Search != "" {
		query += " |= " + strconv.Quote(filter.Search)
	}
	return query
}

type lokiResponse struct {
	Status string `json:"status"`
	Data   struct {
		ResultType string `json:"resultType"`
		Result     []struct {
			Stream map[string]string `json:"stream"`
			Values [][]string        `json:"values"`
		} `json:"result"`
	} `json:"data"`
}

func entryFromLoki(labels map[string]string, raw string, timestamp time.Time) Entry {
	entry := Entry{
		Timestamp:   timestamp,
		Source:      Source(labels[labelSource]),
		Level:       Level(strings.ToLower(labels[labelLevel])),
		Message:     raw,
		Environment: labels[labelEnvironment],
	}
	if !entry.Source.Valid() {
		entry.Source = SourceRuntime
	}
	if !entry.Level.Valid() {
		entry.Level = ""
	}
	entry.ServiceID = parseOptionalUUID(labels[labelServiceID])
	entry.DeploymentID = parseOptionalUUID(labels[labelDeploymentID])
	decodeStructuredLine(&entry, raw)
	return entry
}

func decodeStructuredLine(entry *Entry, raw string) {
	fields := make(map[string]any)
	if json.Unmarshal([]byte(raw), &fields) != nil {
		return
	}
	for _, key := range []string{"message", "msg", "log"} {
		if value, ok := fields[key].(string); ok && value != "" {
			entry.Message = value
			break
		}
	}
	request := &RequestMetadata{
		RequestID:    stringField(fields, "request_id"),
		Method:       stringField(fields, "method"),
		Path:         stringField(fields, "path"),
		Status:       int(numberField(fields, "status")),
		DurationMS:   numberField(fields, "duration_ms"),
		ResponseSize: int64(numberField(fields, "response_size")),
	}
	if request.RequestID != "" || request.Method != "" || request.Path != "" || request.Status != 0 || request.DurationMS != 0 || request.ResponseSize != 0 {
		entry.Request = request
	}
}

func stringField(fields map[string]any, key string) string {
	value, _ := fields[key].(string)
	return value
}

func numberField(fields map[string]any, key string) float64 {
	value, _ := fields[key].(float64)
	return value
}

func parseOptionalUUID(value string) *uuid.UUID {
	id, err := uuid.Parse(value)
	if err != nil {
		return nil
	}
	return &id
}

func entryKey(entry Entry) string {
	serviceID, deploymentID := "", ""
	if entry.ServiceID != nil {
		serviceID = entry.ServiceID.String()
	}
	if entry.DeploymentID != nil {
		deploymentID = entry.DeploymentID.String()
	}
	return strconv.FormatInt(entry.Timestamp.UnixNano(), 10) + "\x00" + string(entry.Source) + "\x00" + serviceID + "\x00" + deploymentID + "\x00" + entry.Message
}
