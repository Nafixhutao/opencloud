package repository

import (
	"context"
	"errors"
	"time"

	"github.com/Nafixhutao/opencloud/backend/internal/model"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer"
	"xorm.io/bun"
)

// ErrSessionNotFound indicates session does not exist
var ErrSessionNotFound = errors.New("session not found")

// DatabaseConsoleSessionRepository handles database console session operations
type DatabaseConsoleSessionRepository struct {
	db *bun.DB
}

// NewDatabaseConsoleSessionRepository creates a new session repository
func NewDatabaseConsoleSessionRepository(db *bun.DB) *DatabaseConsoleSessionRepository {
	return &DatabaseConsoleSessionRepository{db: db}
}

// CreateSession creates a new database console session with short TTL
func (r *DatabaseConsoleSessionRepository) CreateSession(ctx context.Context, session *model.DatabaseConsoleSession) error {
	ctx, span := tracer.StartSpanFromContext(ctx, "repository.DatabaseConsoleSessionRepository.CreateSession")
	defer span.Finish()

	_, err := r.db.NewInsert().Model(session).Exec(ctx)
	return err
}

// GetSession retrieves a session by ID
func (r *DatabaseConsoleSessionRepository) GetSession(ctx context.Context, accountID string, sessionID string) (*model.DatabaseConsoleSession, error) {
	ctx, span := tracer.StartSpanFromContext(ctx, "repository.DatabaseConsoleSessionRepository.GetSession")
	defer span.Finish()

	var session model.DatabaseConsoleSession
	err := r.db.NewSelect().
		Model(&session).
		Where("account_id = ? AND id = ?", accountID, sessionID).
		Limit(1).
		Scan(ctx)

	if err != nil {
		return nil, err
	}

	return &session, nil
}

// RevokeSession revokes a session immediately
func (r *DatabaseConsoleSessionRepository) RevokeSession(ctx context.Context, accountID string, sessionID string) error {
	ctx, span := tracer.StartSpanFromContext(ctx, "repository.DatabaseConsoleSessionRepository.RevokeSession")
	defer span.Finish()

	result, err := r.db.NewDelete().
		Model((*model.DatabaseConsoleSession)(nil)).
		Where("account_id = ? AND id = ?", accountID, sessionID).
		Exec(ctx)

	if err != nil {
		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil || rowsAffected == 0 {
		return ErrSessionNotFound
	}

	return nil
}

// CleanupExpiredSessions removes expired sessions (called by worker/cron)
func (r *DatabaseConsoleSessionRepository) CleanupExpiredSessions(ctx context.Context) (int64, error) {
	ctx, span := tracer.StartSpanFromContext(ctx, "repository.DatabaseConsoleSessionRepository.CleanupExpiredSessions")
	defer span.Finish()

	now := time.Now()
	result, err := r.db.NewDelete().
		Model((*model.DatabaseConsoleSession)(nil)).
		Where("expires_at < ?", now).
		Exec(ctx)

	if err != nil {
		return 0, err
	}

	return result.RowsAffected()
}

// UpdateLastActivity updates the last activity timestamp for a session
func (r *DatabaseConsoleSessionRepository) UpdateLastActivity(ctx context.Context, accountID string, sessionID string) error {
	ctx, span := tracer.StartSpanFromContext(ctx, "repository.DatabaseConsoleSessionRepository.UpdateLastActivity")
	defer span.Finish()

	now := time.Now()
	_, err := r.db.NewUpdate().
		Model(&model.DatabaseConsoleSession{}).
		Set("last_activity_at = ?", now).
		Where("account_id = ? AND id = ?", accountID, sessionID).
		Exec(ctx)

	return err
}
