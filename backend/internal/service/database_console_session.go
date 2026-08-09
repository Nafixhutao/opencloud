package service

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"

	"github.com/nazxf/opencloud/backend/internal/apperr"
	"github.com/nazxf/opencloud/backend/internal/model"
	"github.com/nazxf/opencloud/backend/internal/repository"
)

var (
	ErrDatabaseNotFound       = errors.New("database not found")
	ErrAccountMismatch        = errors.New("account does not own this database")
	ErrDatabaseNotManaged     = errors.New("database is not managed by OpenCloud")
	ErrSessionAlreadyExists   = errors.New("active console session already exists")
	ErrSessionNotFound        = errors.New("console session not found")
	ErrSessionExpired         = errors.New("console session has expired")
	ErrSessionRevoked         = errors.New("console session has been revoked")
	ErrInvalidEngine          = errors.New("invalid database engine type")
)

// ConsoleSessionDurationRequest holds the requested session TTL.
type ConsoleSessionDurationRequest struct {
	Duration time.Duration `json:"duration,omitempty"`
}

// ConsoleSessionResponse represents the data returned when creating a session.
type ConsoleSessionResponse struct {
	ID        uuid.UUID `json:"id"`
	ExpiresAt time.Time `json:"expires_at"`
	Token     string    `json:"token"` // short-lived opaque token for gateway handshake
}

// DatabaseConsoleService manages database console sessions.
type DatabaseConsoleService struct {
	repo   *repository.DatabaseConsoleSessionRepository
	dbRepo *repository.ManagedDatabaseRepository
	audit  *repository.AuditRepository
}

// NewDatabaseConsoleService constructs a DatabaseConsoleService.
func NewDatabaseConsoleService(
	dbRepo *repository.ManagedDatabaseRepository,
	sessionRepo *repository.DatabaseConsoleSessionRepository,
	auditRepo *repository.AuditRepository,
) *DatabaseConsoleService {
	return &DatabaseConsoleService{
		repo:   sessionRepo,
		dbRepo: dbRepo,
		audit:  auditRepo,
	}
}

// CreateSession creates a new console session for accessing a database.
func (s *DatabaseConsoleService) CreateSession(
	ctx context.Context,
	actorID string,
	accountID uuid.UUID,
	databaseID uuid.UUID,
	duration time.Duration,
) (*ConsoleSessionResponse, error) {
	// Validate duration
	if duration < model.MinSessionTTL {
		duration = model.MinSessionTTL
	} else if duration > model.MaxSessionTTL {
		duration = model.MaxSessionTTL
	}

	// Validate database ownership and existence
	db, err := s.dbRepo.Get(ctx, accountID, databaseID)
	if err != nil {
		if errors.Is(err, apperr.NotFound) {
			return nil, apperr.ResourceNotFound("database")
		}
		return nil, err
	}

	// Verify database is managed
	if !db.IsManaged() {
		return nil, ErrDatabaseNotManaged
	}

	// Validate engine
	switch db.Engine {
	case model.DatabaseEnginePostgres, model.DatabaseEngineMariaDB:
	default:
		return nil, ErrInvalidEngine
	}

	// Check for existing active session
	existing, _ := s.repo.GetActiveByDatabase(ctx, accountID, databaseID)
	if existing != nil {
		return nil, ErrSessionAlreadyExists
	}

	// Create new session
	now := time.Now().UTC()
	expiresAt := now.Add(duration).UTC()

	session := &model.DatabaseConsoleSession{
		AccountID:  accountID,
		DatabaseID: databaseID,
		ActorID:    actorID,
		Engine:     db.Engine,
		Status:     model.ConsoleSessionActive,
		ExpiresAt:  expiresAt,
		CreatedAt:  now,
	}

	if err := s.repo.Create(ctx, session); err != nil {
		return nil, err
	}

	// Append audit log
	if err := s.audit.Append(ctx, repository.AuditParams{
		AccountID: accountID,
		ActorID:   actorID,
		Action:    "database_console_session_created",
		Target:    session.ID.String(),
		Metadata: map[string]interface{}{
			"database_id": databaseID.String(),
			"engine":      db.Engine,
			"expires_at":  expiresAt,
		},
	}); err != nil {
		// Log failure but don't fail the session creation
	}

	return &ConsoleSessionResponse{
		ID:        session.ID,
		ExpiresAt: expiresAt,
		Token:     session.ID.String(), // Use UUID as opaque token
	}, nil
}

// RevokeSession revokes an active console session.
func (s *DatabaseConsoleService) RevokeSession(
	ctx context.Context,
	actorID string,
	accountID uuid.UUID,
	sessionID uuid.UUID,
) error {
	err := s.repo.Revoke(ctx, sessionID, actorID)
	if err != nil {
		if errors.Is(err, bun.ErrNotFound) {
			return apperr.NotFound("session")
		}
		return err
	}

	// Append audit log
	if err := s.audit.Append(ctx, repository.AuditParams{
		AccountID: accountID,
		ActorID:   actorID,
		Action:    "database_console_session_revoked",
		Target:    sessionID.String(),
		Metadata:  nil,
	}); err != nil {
		// Log failure but don't fail the revoke
	}

	return nil
}

// ValidateSession validates that a session exists and is active for the given database.
// This is called by the gateway to authenticate console access.
func (s *DatabaseConsoleService) ValidateSession(
	ctx context.Context,
	databaseID uuid.UUID,
	token string,
) (*model.DatabaseConsoleSession, error) {
	sessionID, err := uuid.Parse(token)
	if err != nil {
		return nil, apperr.Unauthenticated("invalid token format")
	}

	// Fetch session by ID directly
	var session model.DatabaseConsoleSession
	err = s.repo.db.NewSelect().Model(&session).
		Where("id = ?", sessionID).
		Where("database_id = ?", databaseID).
		Where("status = ?", model.ConsoleSessionActive).
		Where("expires_at > ?", time.Now()).
		Scan(ctx)
	if err != nil {
		if errors.Is(err, bun.ErrNotFound) {
			return nil, apperr.Forbidden("session not found or expired")
		}
		return nil, err
	}

	return &session, nil
}
