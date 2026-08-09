package service

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"

	"github.com/nazxf/opencloud/backend/internal/apperr"
	"github.com/nazxf/opencloud/backend/internal/model"
	"github.com/nazxf/opencloud/backend/internal/repository"
)

// DatabaseConsoleSessionService handles session business logic.
type DatabaseConsoleSessionService struct {
	repo         *repository.DatabaseConsoleSessionRepository
	databaseRepo *repository.ManagedDatabaseRepo
	enabled      bool
}

// NewDatabaseConsoleSessionService creates a new session service.
func NewDatabaseConsoleSessionService(
	repo *repository.DatabaseConsoleSessionRepository,
	databaseRepo *repository.ManagedDatabaseRepo,
	enabled bool,
) *DatabaseConsoleSessionService {
	return &DatabaseConsoleSessionService{
		repo:         repo,
		databaseRepo: databaseRepo,
		enabled:      enabled,
	}
}

// CreateOptions contains parameters for session creation.
type CreateOptions struct {
	AccountID  string
	ActorID    string
	DatabaseID string
	IPAddress  string
	UserAgent  string
	TTLMinutes int // 15, 30, or 60 minutes
}

// CreateSession validates database ownership and creates a short-lived session.
func (s *DatabaseConsoleSessionService) CreateSession(ctx context.Context, opts CreateOptions) (*model.DatabaseConsoleSession, error) {
	if !s.enabled {
		return nil, apperr.Unavailable("the database console is not configured")
	}
	accountID, err := uuid.Parse(opts.AccountID)
	if err != nil {
		return nil, apperr.Validation("invalid account id")
	}
	databaseID, err := uuid.Parse(opts.DatabaseID)
	if err != nil {
		return nil, apperr.Validation("invalid database id")
	}

	// Ownership + availability check before issuing a session.
	database, err := s.databaseRepo.GetByAccount(ctx, accountID, databaseID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, apperr.NotFound("database not found")
		}
		return nil, apperr.Internal("failed to load database").Wrap(err)
	}
	if database.Status != model.DatabaseActive {
		return nil, apperr.Conflict("database must be active before opening the console")
	}

	sessionToken := uuid.New().String()
	now := time.Now().UTC()
	session := &model.DatabaseConsoleSession{
		ID:           uuid.New().String(),
		AccountID:    opts.AccountID,
		ActorID:      opts.ActorID,
		DatabaseID:   opts.DatabaseID,
		Engine:       database.Engine,
		Status:       model.ConsoleSessionActive,
		CreatedAt:    now,
		ExpiresAt:    calculateExpiration(opts.TTLMinutes),
		SessionToken: sessionToken,
		IPAddr:       &opts.IPAddress,
		UserAgent:    &opts.UserAgent,
	}

	if err := s.repo.CreateSession(ctx, session); err != nil {
		return nil, apperr.Internal("failed to create console session").Wrap(err)
	}

	return session, nil
}

// ValidateSession checks if a session is valid, active, and not expired.
func (s *DatabaseConsoleSessionService) ValidateSession(ctx context.Context, accountID string, sessionID string) (*model.DatabaseConsoleSession, error) {
	session, err := s.repo.GetSession(ctx, accountID, sessionID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, apperr.Forbidden("invalid or expired console session")
		}
		return nil, apperr.Internal("failed to validate console session").Wrap(err)
	}
	if session.Status != model.ConsoleSessionActive {
		return nil, apperr.Forbidden("console session is no longer active")
	}
	if time.Now().After(session.ExpiresAt) {
		_ = s.repo.RevokeSession(ctx, accountID, sessionID)
		return nil, apperr.Forbidden("console session has expired")
	}
	_ = s.repo.UpdateLastActivity(ctx, accountID, sessionID)
	return session, nil
}

// RevokeSession terminates a session immediately; the row is retained for audit.
func (s *DatabaseConsoleSessionService) RevokeSession(ctx context.Context, accountID string, sessionID string) error {
	err := s.repo.RevokeSession(ctx, accountID, sessionID)
	if err != nil {
		if errors.Is(err, repository.ErrSessionNotFound) {
			return apperr.NotFound("console session not found")
		}
		return apperr.Internal("failed to revoke console session").Wrap(err)
	}
	return nil
}

// CleanupExpired runs cleanup of expired sessions.
func (s *DatabaseConsoleSessionService) CleanupExpired(ctx context.Context) (int64, error) {
	marked, err := s.repo.MarkExpired(ctx)
	if err != nil {
		return 0, err
	}
	removed, err := s.repo.CleanupExpiredSessions(ctx)
	if err != nil {
		return 0, err
	}
	return marked + removed, nil
}

func calculateExpiration(ttlMinutes int) time.Time {
	minutes := 15
	if ttlMinutes >= 15 && ttlMinutes <= 60 {
		minutes = ttlMinutes
	}
	return time.Now().UTC().Add(time.Minute * time.Duration(minutes))
}
