package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/nazxf/opencloud/backend/internal/apperr"
	"github.com/nazxf/opencloud/backend/internal/logs"
	"github.com/nazxf/opencloud/backend/internal/middleware"
	"github.com/nazxf/opencloud/backend/internal/service"
)

const logHeartbeatInterval = 15 * time.Second

type logService interface {
	Query(context.Context, uuid.UUID, uuid.UUID, service.LogQuery) ([]logs.Entry, error)
	Stream(context.Context, uuid.UUID, uuid.UUID, service.LogQuery) (logs.Subscription, error)
}

// LogHandler serves tenant-scoped historical and live customer logs.
type LogHandler struct{ svc logService }

// NewLogHandler constructs the customer logs HTTP boundary.
func NewLogHandler(svc logService) *LogHandler { return &LogHandler{svc: svc} }

// List returns a bounded chronological log page.
func (h *LogHandler) List(c *gin.Context) {
	projectID, ok := projectIDParam(c)
	if !ok {
		return
	}
	query, err := logQuery(c)
	if err != nil {
		respondError(c, err)
		return
	}
	rows, err := h.svc.Query(c.Request.Context(), middleware.AccountID(c), projectID, query)
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": rows, "meta": gin.H{"count": len(rows)}})
}

// Stream emits live-tail entries over Server-Sent Events. The authenticated
// BFF proxies this response so browser EventSource never receives a bearer JWT.
func (h *LogHandler) Stream(c *gin.Context) {
	projectID, ok := projectIDParam(c)
	if !ok {
		return
	}
	query, err := logQuery(c)
	if err != nil {
		respondError(c, err)
		return
	}
	if query.Start.IsZero() {
		if eventID := strings.TrimSpace(c.GetHeader("Last-Event-ID")); eventID != "" {
			nanoseconds, parseErr := strconv.ParseInt(eventID, 10, 64)
			if parseErr != nil || nanoseconds < 1 {
				respondError(c, apperr.Validation("invalid Last-Event-ID"))
				return
			}
			query.Start = time.Unix(0, nanoseconds).UTC()
		}
	}
	subscription, err := h.svc.Stream(c.Request.Context(), middleware.AccountID(c), projectID, query)
	if err != nil {
		respondError(c, err)
		return
	}
	defer subscription.Close()

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache, no-store")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")
	c.Status(http.StatusOK)
	if _, err := c.Writer.WriteString("retry: 3000\n\n"); err != nil {
		return
	}
	c.Writer.Flush()
	heartbeat := time.NewTicker(logHeartbeatInterval)
	defer heartbeat.Stop()

	entries, streamErrors := subscription.Entries, subscription.Errors
	for entries != nil || streamErrors != nil {
		select {
		case entry, open := <-entries:
			if !open {
				entries = nil
				continue
			}
			payload, marshalErr := json.Marshal(entry)
			if marshalErr != nil {
				return
			}
			if _, writeErr := fmt.Fprintf(c.Writer, "id: %d\nevent: log\ndata: %s\n\n", entry.Timestamp.UnixNano(), payload); writeErr != nil {
				return
			}
			c.Writer.Flush()
		case streamErr, open := <-streamErrors:
			if !open {
				streamErrors = nil
				continue
			}
			if streamErr != nil {
				_, _ = c.Writer.WriteString("event: error\ndata: {\"code\":\"LOG_STREAM_INTERRUPTED\"}\n\n")
				c.Writer.Flush()
				return
			}
		case <-heartbeat.C:
			if _, writeErr := c.Writer.WriteString(": keepalive\n\n"); writeErr != nil {
				return
			}
			c.Writer.Flush()
		case <-c.Request.Context().Done():
			return
		}
	}
}

func logQuery(c *gin.Context) (service.LogQuery, error) {
	serviceID, err := optionalLogUUID(c.Query("service_id"), "service_id")
	if err != nil {
		return service.LogQuery{}, err
	}
	deploymentID, err := optionalLogUUID(c.Query("deployment_id"), "deployment_id")
	if err != nil {
		return service.LogQuery{}, err
	}
	start, err := optionalLogTime(c.Query("start"), "start")
	if err != nil {
		return service.LogQuery{}, err
	}
	end, err := optionalLogTime(c.Query("end"), "end")
	if err != nil {
		return service.LogQuery{}, err
	}
	limit := 0
	if raw := strings.TrimSpace(c.Query("limit")); raw != "" {
		limit, err = strconv.Atoi(raw)
		if err != nil {
			return service.LogQuery{}, apperr.Validation("invalid log limit")
		}
	}
	sources := make([]logs.Source, 0)
	for _, value := range splitLogFilter(c.QueryArray("source")) {
		sources = append(sources, logs.Source(strings.ToLower(value)))
	}
	levels := make([]logs.Level, 0)
	for _, value := range splitLogFilter(c.QueryArray("level")) {
		levels = append(levels, logs.Level(strings.ToLower(value)))
	}
	return service.LogQuery{
		ServiceID: serviceID, DeploymentID: deploymentID,
		Sources: sources, Levels: levels,
		Environment: c.Query("environment"), Search: c.Query("search"),
		Start: start, End: end, Limit: limit,
	}, nil
}

func optionalLogUUID(value, field string) (*uuid.UUID, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}
	id, err := uuid.Parse(value)
	if err != nil {
		return nil, apperr.Validation("invalid " + field)
	}
	return &id, nil
}

func optionalLogTime(value, field string) (time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, nil
	}
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}, apperr.Validation("invalid log " + field + " time")
	}
	return parsed.UTC(), nil
}

func splitLogFilter(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		for _, part := range strings.Split(value, ",") {
			if part = strings.TrimSpace(part); part != "" {
				result = append(result, part)
			}
		}
	}
	return result
}
