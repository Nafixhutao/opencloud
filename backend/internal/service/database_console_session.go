package service

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/nazxf/opencloud/backend/internal/model"
	"github.com/nazxf/opencloud/backend/internal/repository"
)

// DatabaseConsoleSessionService handles session business logic
type DatabaseConsoleSessionService struct {
	repo *repository.DatabaseConsoleSessionRepository
}

// NewDatabaseConsoleSessionService creates a new session service
func NewDatabaseConsoleSessionService(repo *repository.DatabaseConsoleSessionRepository) *DatabaseConsoleSessionService {
	return &DatabaseConsoleSessionService{repo: repo}
}

// CreateOptions contains parameters for session creation
type CreateOptions struct {
	AccountID  string
	DatabaseID string
	IPAddress  string
	UserAgent  string
	TTLMinutes int // 15, 30, or 60 minutes
}

// CreateSession creates a new database console session
func (s *DatabaseConsoleSessionService) CreateSession(ctx context.Context, opts CreateOptions) (*model.DatabaseConsoleSession, error) {
	sessionToken := uuid.New().String()
	expiresAt := calculateExpiration(opts.TTLMinutes)

	session := &model.DatabaseConsoleSession{
		ID:           uuid.New().String(),
		AccountID:    opts.AccountID,
		DatabaseID:   opts.DatabaseID,
		SessionToken: sessionToken,
		CreatedAt:    time.Now().UTC(),
		ExpiresAt:    expiresAt,
		IPAddr:       &opts.IPAddress,
		UserAgent:    &opts.UserAgent,
	}

	if err := s.repo.CreateSession(ctx, session); err != nil {
		return nil, wrapErr(err, "failed to create session")
	}

	return session, nil
}

// ValidateSession checks if a session is valid and not expired
func (s *DatabaseConsoleSessionService) ValidateSession(ctx context.Context, accountID string, sessionToken string) (*model.DatabaseConsoleSession, error) {
	sessionID := sessionToken

	session, err := s.repo.GetSession(ctx, accountID, sessionID)
	if err != nil {
		if err == repository.ErrSessionNotFound {
			return nil, ErrInvalidSession
		}
		return nil, wrapErr(err, "failed to validate session")
	}

	if time.Now().After(session.ExpiresAt) {
		_ = s.repo.RevokeSession(ctx, accountID, sessionID)
		return nil, ErrSessionExpired
	}

	_ = s.repo.UpdateLastActivity(ctx, accountID, sessionID)

	return session, nil
}

// RevokeSession terminates a session immediately
func (s *DatabaseConsoleSessionService) RevokeSession(ctx context.Context, accountID string, sessionID string) error {
	return s.repo.RevokeSession(ctx, accountID, sessionID)
}

// CleanupExpired runs cleanup of expired sessions
func (s *DatabaseConsoleSessionService) CleanupExpired(ctx context.Context) (int64, error) {
	return s.repo.CleanupExpiredSessions(ctx)
}

func calculateExpiration(ttlMinutes int) time.Time {
	minutes := 15
	if ttlMinutes > 15 && ttlMinutes <= 60 {
		minutes = ttlMinutes
	}
	return time.Now().Add(time.Minute * time.Duration(minutes))
}
