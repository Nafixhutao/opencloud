package repository

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"

	"github.com/nazxf/opencloud/backend/internal/model"
)

// DatabaseConsoleSessionRepository manages database_console_sessions data access.
type DatabaseConsoleSessionRepository struct {
	db bun.IDB
}

// NewDatabaseConsoleSessionRepository constructs a DatabaseConsoleSessionRepository.
func NewDatabaseConsoleSessionRepository(db bun.IDB) *DatabaseConsoleSessionRepository {
	return &DatabaseConsoleSessionRepository{db: db}
}

// Create creates a new active console session.
func (r *DatabaseConsoleSessionRepository) Create(
	ctx context.Context,
	session *model.DatabaseConsoleSession,
) error {
	_, err := r.db.NewInsert().Model(session).Exec(ctx)
	return err
}

// GetActiveByDatabase retrieves an active session for a database if one exists and is not expired.
func (r *DatabaseConsoleSessionRepository) GetActiveByDatabase(
	ctx context.Context,
	accountID uuid.UUID,
	databaseID uuid.UUID,
) (*model.DatabaseConsoleSession, error) {
	var session model.DatabaseConsoleSession
	err := r.db.NewSelect().Model(&session).
		Where("account_id = ?", accountID).
		Where("database_id = ?", databaseID).
		Where("status = ?", model.ConsoleSessionActive).
		Where("expires_at > ?", time.Now()).
		OrderById("DESC").
		Limit(1).
		Scan(ctx)
	if err != nil {
		return nil, err
	}
	return &session, nil
}

// Revoke marks a session as revoked.
func (r *DatabaseConsoleSessionRepository) Revoke(
	ctx context.Context,
	sessionID uuid.UUID,
	actorID string,
) error {
	now := time.Now()
	session := &model.DatabaseConsoleSession{
		ID:      sessionID,
		RevokedAt: &now,
		Status:  model.ConsoleSessionRevoked,
	}
	_, err := r.db.NewUpdate().Model(session).
		Where("id = ?", sessionID).
		Where("status = ?", model.ConsoleSessionActive).
		Where("expires_at > ?", now).
		Where("actor_id = ?", actorID).
		Exec(ctx)
	return err
}

// ExpireOldSessions removes all sessions that have expired but are not marked as expired.
func (r *DatabaseConsoleSessionRepository) ExpireOldSessions(
	ctx context.Context,
	before time.Time,
) (int64, error) {
	result, err := r.db.NewUpdate().
		Model((*model.DatabaseConsoleSession)(nil)).
		Set("status = ?", model.ConsoleSessionExpired).
		Where("status = ?", model.ConsoleSessionActive).
		Where("expires_at <= ?", before).
		Exec(ctx)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// DeleteByDatabase removes all sessions associated with a database.
func (r *DatabaseConsoleSessionRepository) DeleteByDatabase(
	ctx context.Context,
	databaseID uuid.UUID,
) error {
	_, err := r.db.NewDelete().
		Model((*model.DatabaseConsoleSession)(nil)).
		Where("database_id = ?", databaseID).
		Exec(ctx)
	return err
}
