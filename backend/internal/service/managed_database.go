package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"regexp"
	"strings"

	"github.com/google/uuid"
	"github.com/uptrace/bun"

	"github.com/nazxf/opencloud/backend/internal/apperr"
	"github.com/nazxf/opencloud/backend/internal/credential"
	"github.com/nazxf/opencloud/backend/internal/model"
	"github.com/nazxf/opencloud/backend/internal/provisioner"
	"github.com/nazxf/opencloud/backend/internal/repository"
)

var managedDatabaseNamePattern = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,62}$`)

// ManagedDatabaseService owns tenant-scoped customer database lifecycle
// transitions and one-time credential reveal.
type ManagedDatabaseService struct {
	db      *bun.DB
	rows    *repository.ManagedDatabaseRepo
	jobs    *repository.JobRepo
	audit   *repository.AuditRepo
	enabled bool
	cipher  *credential.Cipher
}

// NewManagedDatabaseService constructs a ManagedDatabaseService.
func NewManagedDatabaseService(
	db *bun.DB,
	rows *repository.ManagedDatabaseRepo,
	jobs *repository.JobRepo,
	audit *repository.AuditRepo,
	enabled bool,
	cipher *credential.Cipher,
) *ManagedDatabaseService {
	return &ManagedDatabaseService{
		db:      db,
		rows:    rows,
		jobs:    jobs,
		audit:   audit,
		enabled: enabled,
		cipher:  cipher,
	}
}

// CreateDatabaseRequest is the approved tenant-facing create contract.
type CreateDatabaseRequest struct {
	Name   string `json:"name"`
	Engine string `json:"engine"`
}

type managedDatabaseJobPayload struct {
	DatabaseID uuid.UUID `json:"database_id"`
}

// Create records one durable provisioning intent, job, and audit atomically.
func (s *ManagedDatabaseService) Create(
	ctx context.Context,
	actorUserID string,
	accountID uuid.UUID,
	idempotencyKey string,
	req CreateDatabaseRequest,
) (*model.ManagedDatabase, error) {
	if !s.enabled {
		return nil, apperr.Unavailable("customer database provisioning is not enabled")
	}
	name, engine, key, err := validateDatabaseCreate(req, idempotencyKey)
	if err != nil {
		return nil, err
	}

	var created *model.ManagedDatabase
	err = s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		rows := s.rows.WithDB(tx)
		jobs := s.jobs.WithDB(tx)
		audit := s.audit.WithDB(tx)

		if err := rows.LockCreateRequest(ctx, accountID, key); err != nil {
			return err
		}
		prior, priorErr := rows.GetByIdempotencyKey(ctx, accountID, key)
		if priorErr == nil {
			if prior.Name != name || prior.Engine != engine {
				return apperr.Conflict("idempotency key was already used for another database")
			}
			available, err := rows.CredentialExists(ctx, prior.ID)
			if err != nil {
				return err
			}
			prior.CredentialAvailable = available
			created = prior
			return nil
		}
		if !errors.Is(priorErr, sql.ErrNoRows) {
			return priorErr
		}

		databaseID := uuid.New()
		compactID := strings.ReplaceAll(databaseID.String(), "-", "")
		row := &model.ManagedDatabase{
			ID:                   databaseID,
			AccountID:            accountID,
			Name:                 name,
			Engine:               engine,
			PhysicalDatabaseName: "ocdb_" + compactID,
			PhysicalUsername:     "ocu_" + compactID,
			Status:               model.DatabaseProvisioning,
			IdempotencyKey:       key,
		}
		if err := rows.Create(ctx, row); err != nil {
			return err
		}
		aid := accountID
		if _, err := jobs.Enqueue(
			ctx,
			&aid,
			model.JobProvisionDatabase,
			managedDatabaseJobPayload{DatabaseID: databaseID},
		); err != nil {
			return err
		}
		actor := actorUserID
		if err := audit.Append(ctx, repository.Entry{
			AccountID: &aid,
			ActorID:   &actor,
			Action:    model.AuditDatabaseCreateQueued,
			Target:    strPtr(databaseID.String()),
			Metadata:  map[string]any{"name": name, "engine": engine},
		}); err != nil {
			return err
		}
		created = row
		return nil
	})
	if err != nil {
		if apperr.As(err) != nil {
			return nil, err
		}
		if uniqueViolation(err) {
			return nil, apperr.Conflict("database name or idempotency key is already in use")
		}
		return nil, apperr.Internal("failed to queue database creation").Wrap(err)
	}
	return created, nil
}

// List returns a paginated tenant-scoped database collection.
func (s *ManagedDatabaseService) List(
	ctx context.Context,
	accountID uuid.UUID,
	page, perPage int,
) ([]model.ManagedDatabase, int, error) {
	if page < 1 {
		page = 1
	}
	if perPage <= 0 {
		perPage = 25
	}
	if perPage > 100 {
		perPage = 100
	}
	rows, total, err := s.rows.ListByAccount(ctx, accountID, perPage, (page-1)*perPage)
	if err != nil {
		return nil, 0, apperr.Internal("failed to list databases").Wrap(err)
	}
	return rows, total, nil
}

// Get returns one database without revealing cross-tenant existence.
func (s *ManagedDatabaseService) Get(
	ctx context.Context,
	accountID, databaseID uuid.UUID,
) (*model.ManagedDatabase, error) {
	row, err := s.rows.GetByAccount(ctx, accountID, databaseID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, apperr.NotFound("database not found")
		}
		return nil, apperr.Internal("failed to load database").Wrap(err)
	}
	return row, nil
}

// Delete records delete intent, revokes any unrevealed credential, and queues
// data-plane deletion in the same transaction.
func (s *ManagedDatabaseService) Delete(
	ctx context.Context,
	actorUserID string,
	accountID, databaseID uuid.UUID,
) (*model.ManagedDatabase, error) {
	if !s.enabled {
		return nil, apperr.Unavailable("customer database provisioning is not enabled")
	}
	var result *model.ManagedDatabase
	err := s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		rows := s.rows.WithDB(tx)
		jobs := s.jobs.WithDB(tx)
		audit := s.audit.WithDB(tx)

		row, err := rows.GetByAccountForUpdateIncludingDeleted(ctx, accountID, databaseID)
		if err != nil {
			return err
		}
		if row.Status == model.DatabaseDeleting || row.Status == model.DatabaseDeleted {
			available, err := rows.CredentialExists(ctx, row.ID)
			if err != nil {
				return err
			}
			row.CredentialAvailable = available
			result = row
			return nil
		}
		if row.Status != model.DatabaseProvisioning &&
			row.Status != model.DatabaseActive &&
			row.Status != model.DatabaseFailed {
			return apperr.Conflict("database cannot be deleted from its current state")
		}

		if err := rows.SetStatus(ctx, accountID, databaseID, model.DatabaseDeleting); err != nil {
			return err
		}
		if err := rows.DeleteCredential(ctx, databaseID); err != nil {
			return err
		}
		aid := accountID
		if _, err := jobs.Enqueue(
			ctx,
			&aid,
			model.JobDeleteDatabase,
			managedDatabaseJobPayload{DatabaseID: databaseID},
		); err != nil {
			return err
		}
		actor := actorUserID
		if err := audit.Append(ctx, repository.Entry{
			AccountID: &aid,
			ActorID:   &actor,
			Action:    model.AuditDatabaseDeleteQueued,
			Target:    strPtr(databaseID.String()),
			Metadata: map[string]any{
				"name":   row.Name,
				"engine": row.Engine,
				"from":   row.Status,
				"to":     model.DatabaseDeleting,
			},
		}); err != nil {
			return err
		}
		row.Status = model.DatabaseDeleting
		row.LastError = nil
		row.CredentialAvailable = false
		result = row
		return nil
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, apperr.NotFound("database not found")
		}
		if apperr.As(err) != nil {
			return nil, err
		}
		return nil, apperr.Internal("failed to queue database deletion").Wrap(err)
	}
	return result, nil
}

// RevealCredential consumes an encrypted credential exactly once. Row lock,
// audit append, and ciphertext deletion commit together.
func (s *ManagedDatabaseService) RevealCredential(
	ctx context.Context,
	actorUserID string,
	accountID, databaseID uuid.UUID,
) (*provisioner.DatabaseCredentials, error) {
	if !s.enabled || s.cipher == nil {
		return nil, apperr.Unavailable("customer database credentials are not configured")
	}
	var revealed *provisioner.DatabaseCredentials
	err := s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		rows := s.rows.WithDB(tx)
		audit := s.audit.WithDB(tx)

		row, err := rows.GetByAccountForUpdate(ctx, accountID, databaseID)
		if err != nil {
			return err
		}
		if row.Status != model.DatabaseActive {
			return apperr.Conflict("database credentials are available only after provisioning completes")
		}
		envelope, err := rows.GetCredentialForUpdate(ctx, databaseID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return apperr.Conflict("database credentials were already revealed or revoked")
			}
			return err
		}
		plaintext, err := s.cipher.Decrypt(databaseID, envelope.Ciphertext)
		if err != nil {
			return err
		}
		defer clear(plaintext)
		var credentials provisioner.DatabaseCredentials
		if err := json.Unmarshal(plaintext, &credentials); err != nil {
			return err
		}
		if credentials.Engine != row.Engine ||
			credentials.Database != row.PhysicalDatabaseName ||
			credentials.Username != row.PhysicalUsername ||
			credentials.Host == "" ||
			credentials.Port <= 0 ||
			credentials.Password == "" {
			return errors.New("database credential payload does not match its resource")
		}

		aid := accountID
		actor := actorUserID
		if err := audit.Append(ctx, repository.Entry{
			AccountID: &aid,
			ActorID:   &actor,
			Action:    model.AuditDatabaseCredentialRevealed,
			Target:    strPtr(databaseID.String()),
			Metadata:  map[string]any{"name": row.Name, "engine": row.Engine},
		}); err != nil {
			return err
		}
		if err := rows.DeleteCredential(ctx, databaseID); err != nil {
			return err
		}
		revealed = &credentials
		return nil
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, apperr.NotFound("database not found")
		}
		if apperr.As(err) != nil {
			return nil, err
		}
		return nil, apperr.Internal("failed to reveal database credentials").Wrap(err)
	}
	return revealed, nil
}

// ConsoleCredentials decrypts the customer database credential for a console
// session without consuming it. Unlike RevealCredential, the envelope stays in
// place so the console can keep executing across queries in the session.
func (s *ManagedDatabaseService) ConsoleCredentials(
	ctx context.Context,
	actorUserID string,
	accountID, databaseID uuid.UUID,
) (*provisioner.DatabaseCredentials, error) {
	if !s.enabled || s.cipher == nil {
		return nil, apperr.Unavailable("customer database credentials are not configured")
	}
	var revealed *provisioner.DatabaseCredentials
	err := s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		rows := s.rows.WithDB(tx)

		row, err := rows.GetByAccountForUpdate(ctx, accountID, databaseID)
		if err != nil {
			return err
		}
		if row.Status != model.DatabaseActive {
			return apperr.Conflict("database credentials are available only after provisioning completes")
		}
		envelope, err := rows.GetCredentialForUpdate(ctx, databaseID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return apperr.Conflict("database credentials are unavailable")
			}
			return err
		}
		plaintext, err := s.cipher.Decrypt(databaseID, envelope.Ciphertext)
		if err != nil {
			return err
		}
		defer clear(plaintext)
		var credentials provisioner.DatabaseCredentials
		if err := json.Unmarshal(plaintext, &credentials); err != nil {
			return err
		}
		if credentials.Engine != row.Engine ||
			credentials.Database != row.PhysicalDatabaseName ||
			credentials.Username != row.PhysicalUsername ||
			credentials.Host == "" ||
			credentials.Port <= 0 ||
			credentials.Password == "" {
			return errors.New("database credential payload does not match its resource")
		}
		revealed = &credentials
		return nil
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, apperr.NotFound("database not found")
		}
		if apperr.As(err) != nil {
			return nil, err
		}
		return nil, apperr.Internal("failed to load database credentials").Wrap(err)
	}
	return revealed, nil
}

func validateDatabaseCreate(
	req CreateDatabaseRequest,
	idempotencyKey string,
) (string, string, string, error) {
	name := strings.ToLower(strings.TrimSpace(req.Name))
	if !managedDatabaseNamePattern.MatchString(name) {
		return "", "", "", apperr.Validation(
			"invalid database name",
			apperr.FieldIssue{
				Field: "name",
				Issue: "must start with a letter and contain only lowercase letters, digits, underscore, or hyphen (max 63)",
			},
		)
	}
	engine := strings.ToLower(strings.TrimSpace(req.Engine))
	if engine != model.DatabaseEnginePostgres && engine != model.DatabaseEngineMariaDB {
		return "", "", "", apperr.Validation(
			"invalid database engine",
			apperr.FieldIssue{Field: "engine", Issue: "must be postgres or mariadb"},
		)
	}
	key := strings.TrimSpace(idempotencyKey)
	if key == "" || len(key) > 128 {
		return "", "", "", apperr.Validation(
			"invalid idempotency key",
			apperr.FieldIssue{Field: "Idempotency-Key", Issue: "required, max 128"},
		)
	}
	return name, engine, key, nil
}
