package repository

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"

	"github.com/nazxf/opencloud/backend/internal/model"
)

// ManagedDatabaseRepo owns control-plane persistence for tenant database
// resources. Customer methods always require accountID; explicitly named
// worker methods are the only unscoped lookups.
type ManagedDatabaseRepo struct {
	db bun.IDB
}

// NewManagedDatabaseRepo constructs a ManagedDatabaseRepo.
func NewManagedDatabaseRepo(db bun.IDB) *ManagedDatabaseRepo {
	return &ManagedDatabaseRepo{db: db}
}

// WithDB returns a copy using db.
func (r *ManagedDatabaseRepo) WithDB(db bun.IDB) *ManagedDatabaseRepo {
	return &ManagedDatabaseRepo{db: db}
}

// LockCreateRequest serializes concurrent retries for one account/key.
func (r *ManagedDatabaseRepo) LockCreateRequest(
	ctx context.Context,
	accountID uuid.UUID,
	idempotencyKey string,
) error {
	scope := "database:" + accountID.String() + ":" + idempotencyKey
	_, err := r.db.NewRaw(`SELECT pg_advisory_xact_lock(hashtextextended(?, 0))`, scope).Exec(ctx)
	return err
}

// LockProviderOperation serializes data-plane calls for one managed database.
// This is a session-scoped lock: callers must use a dedicated connection and
// release the lock on that same connection.
func (r *ManagedDatabaseRepo) LockProviderOperation(
	ctx context.Context,
	databaseID uuid.UUID,
) error {
	scope := "managed-database-provider:" + databaseID.String()
	_, err := r.db.NewRaw(`SELECT pg_advisory_lock(hashtextextended(?, 0))`, scope).Exec(ctx)
	return err
}

// UnlockProviderOperation releases a session-scoped data-plane operation lock.
func (r *ManagedDatabaseRepo) UnlockProviderOperation(
	ctx context.Context,
	databaseID uuid.UUID,
) (bool, error) {
	scope := "managed-database-provider:" + databaseID.String()
	var unlocked bool
	err := r.db.NewRaw(`SELECT pg_advisory_unlock(hashtextextended(?, 0))`, scope).
		Scan(ctx, &unlocked)
	return unlocked, err
}

// Create inserts one provisioning intent.
func (r *ManagedDatabaseRepo) Create(ctx context.Context, database *model.ManagedDatabase) error {
	now := time.Now().UTC()
	if database.ID == uuid.Nil {
		database.ID = uuid.New()
	}
	database.CreatedAt = now
	database.UpdatedAt = now
	_, err := r.db.NewInsert().Model(database).Exec(ctx)
	return err
}

// ListByAccount returns live tenant-owned rows and whether encrypted
// credentials are still available. Physical identifiers are not selected.
func (r *ManagedDatabaseRepo) ListByAccount(
	ctx context.Context,
	accountID uuid.UUID,
	limit, offset int,
) ([]model.ManagedDatabase, int, error) {
	if limit <= 0 {
		limit = 25
	}
	if limit > 100 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}

	total, err := r.db.NewSelect().
		Model((*model.ManagedDatabase)(nil)).
		Where("account_id = ?", accountID).
		Where("deleted_at IS NULL").
		Count(ctx)
	if err != nil {
		return nil, 0, err
	}

	var rows []model.ManagedDatabase
	err = r.db.NewSelect().
		Model(&rows).
		Column(
			"d.id",
			"d.name",
			"d.engine",
			"d.status",
			"d.last_error",
			"d.created_at",
			"d.updated_at",
			"d.deleted_at",
		).
		ColumnExpr(`EXISTS (
			SELECT 1 FROM database_credentials AS dc
			WHERE dc.database_id = d.id
		) AS credential_available`).
		Where("d.account_id = ?", accountID).
		Where("d.deleted_at IS NULL").
		Order("d.created_at DESC").
		Limit(limit).
		Offset(offset).
		Scan(ctx)
	return rows, total, err
}

// GetByAccount returns a live row only when it belongs to accountID.
func (r *ManagedDatabaseRepo) GetByAccount(
	ctx context.Context,
	accountID, databaseID uuid.UUID,
) (*model.ManagedDatabase, error) {
	row := new(model.ManagedDatabase)
	err := r.db.NewSelect().
		Model(row).
		Column(
			"d.id",
			"d.name",
			"d.engine",
			"d.status",
			"d.last_error",
			"d.created_at",
			"d.updated_at",
			"d.deleted_at",
			// The console verifies the decrypted credential against the row's
			// physical identifiers; omitting them leaves both empty and every
			// console query fails the payload check.
			"d.physical_database_name",
			"d.physical_username",
		).
		ColumnExpr(`EXISTS (
			SELECT 1 FROM database_credentials AS dc
			WHERE dc.database_id = d.id
		) AS credential_available`).
		Where("d.account_id = ?", accountID).
		Where("d.id = ?", databaseID).
		Where("d.deleted_at IS NULL").
		Scan(ctx)
	if err != nil {
		return nil, err
	}
	return row, nil
}

// GetByAccountForUpdate locks a live tenant-owned row.
func (r *ManagedDatabaseRepo) GetByAccountForUpdate(
	ctx context.Context,
	accountID, databaseID uuid.UUID,
) (*model.ManagedDatabase, error) {
	row := new(model.ManagedDatabase)
	err := r.db.NewSelect().
		Model(row).
		Where("account_id = ?", accountID).
		Where("id = ?", databaseID).
		Where("deleted_at IS NULL").
		For("UPDATE").
		Scan(ctx)
	if err != nil {
		return nil, err
	}
	return row, nil
}

// GetByAccountForUpdateIncludingDeleted locks a tenant row for idempotent
// deletion.
func (r *ManagedDatabaseRepo) GetByAccountForUpdateIncludingDeleted(
	ctx context.Context,
	accountID, databaseID uuid.UUID,
) (*model.ManagedDatabase, error) {
	row := new(model.ManagedDatabase)
	err := r.db.NewSelect().
		Model(row).
		Where("account_id = ?", accountID).
		Where("id = ?", databaseID).
		For("UPDATE").
		Scan(ctx)
	if err != nil {
		return nil, err
	}
	return row, nil
}

// GetByIdempotencyKey returns a prior create intent for a safe retry.
func (r *ManagedDatabaseRepo) GetByIdempotencyKey(
	ctx context.Context,
	accountID uuid.UUID,
	idempotencyKey string,
) (*model.ManagedDatabase, error) {
	row := new(model.ManagedDatabase)
	err := r.db.NewSelect().
		Model(row).
		Where("account_id = ?", accountID).
		Where("idempotency_key = ?", idempotencyKey).
		Scan(ctx)
	if err != nil {
		return nil, err
	}
	return row, nil
}

// GetForWorker is the deliberate unscoped lookup used only after a durable job
// supplies a server-generated managed database ID.
func (r *ManagedDatabaseRepo) GetForWorker(
	ctx context.Context,
	databaseID uuid.UUID,
) (*model.ManagedDatabase, error) {
	row := new(model.ManagedDatabase)
	err := r.db.NewSelect().Model(row).Where("id = ?", databaseID).Scan(ctx)
	if err != nil {
		return nil, err
	}
	return row, nil
}

// GetForWorkerForUpdate locks a worker-owned transition.
func (r *ManagedDatabaseRepo) GetForWorkerForUpdate(
	ctx context.Context,
	databaseID uuid.UUID,
) (*model.ManagedDatabase, error) {
	row := new(model.ManagedDatabase)
	err := r.db.NewSelect().
		Model(row).
		Where("id = ?", databaseID).
		For("UPDATE").
		Scan(ctx)
	if err != nil {
		return nil, err
	}
	return row, nil
}

// SetStatus updates a tenant-scoped desired state.
func (r *ManagedDatabaseRepo) SetStatus(
	ctx context.Context,
	accountID, databaseID uuid.UUID,
	status string,
) error {
	result, err := r.db.NewUpdate().
		Model((*model.ManagedDatabase)(nil)).
		Set("status = ?", status).
		Set("last_error = NULL").
		Set("updated_at = now()").
		Where("account_id = ?", accountID).
		Where("id = ?", databaseID).
		Exec(ctx)
	if err != nil {
		return err
	}
	return requireOneRow(result)
}

// SetWorkerStatus updates data-plane completion state.
func (r *ManagedDatabaseRepo) SetWorkerStatus(
	ctx context.Context,
	databaseID uuid.UUID,
	status string,
	lastError *string,
) error {
	result, err := r.db.NewUpdate().
		Model((*model.ManagedDatabase)(nil)).
		Set("status = ?", status).
		Set("last_error = ?", lastError).
		Set("updated_at = now()").
		Where("id = ?", databaseID).
		Exec(ctx)
	if err != nil {
		return err
	}
	return requireOneRow(result)
}

// MarkDeleted completes deletion while retaining a pollable tombstone.
func (r *ManagedDatabaseRepo) MarkDeleted(ctx context.Context, databaseID uuid.UUID) error {
	result, err := r.db.NewUpdate().
		Model((*model.ManagedDatabase)(nil)).
		Set("status = ?", model.DatabaseDeleted).
		Set("last_error = NULL").
		Set("deleted_at = COALESCE(deleted_at, now())").
		Set("updated_at = now()").
		Where("id = ?", databaseID).
		Exec(ctx)
	if err != nil {
		return err
	}
	return requireOneRow(result)
}

// StoreCredential atomically publishes a new encrypted credential envelope.
func (r *ManagedDatabaseRepo) StoreCredential(
	ctx context.Context,
	databaseID uuid.UUID,
	ciphertext []byte,
) error {
	row := &model.DatabaseCredential{
		DatabaseID: databaseID,
		Ciphertext: ciphertext,
		CreatedAt:  time.Now().UTC(),
	}
	_, err := r.db.NewInsert().
		Model(row).
		On("CONFLICT (database_id) DO UPDATE").
		Set("ciphertext = EXCLUDED.ciphertext").
		Set("created_at = EXCLUDED.created_at").
		Exec(ctx)
	return err
}

// GetCredential returns the encrypted credential envelope without locking or
// consuming it. Used by the database console, which must reuse the credential.
func (r *ManagedDatabaseRepo) GetCredential(ctx context.Context, databaseID uuid.UUID) (*model.DatabaseCredential, error) {
	row := new(model.DatabaseCredential)
	err := r.db.NewSelect().
		Model(row).
		Where("database_id = ?", databaseID).
		Scan(ctx)
	if err != nil {
		return nil, err
	}
	return row, nil
}

// GetCredentialForUpdate locks an encrypted credential for one-time reveal.
func (r *ManagedDatabaseRepo) GetCredentialForUpdate(
	ctx context.Context,
	databaseID uuid.UUID,
) (*model.DatabaseCredential, error) {
	row := new(model.DatabaseCredential)
	err := r.db.NewSelect().
		Model(row).
		Where("database_id = ?", databaseID).
		For("UPDATE").
		Scan(ctx)
	if err != nil {
		return nil, err
	}
	return row, nil
}

// DeleteCredential consumes or revokes an encrypted credential.
func (r *ManagedDatabaseRepo) DeleteCredential(ctx context.Context, databaseID uuid.UUID) error {
	_, err := r.db.NewDelete().
		Model((*model.DatabaseCredential)(nil)).
		Where("database_id = ?", databaseID).
		Exec(ctx)
	return err
}

// CredentialExists is used by status projections and idempotent transitions.
func (r *ManagedDatabaseRepo) CredentialExists(ctx context.Context, databaseID uuid.UUID) (bool, error) {
	exists, err := r.db.NewSelect().
		Model((*model.DatabaseCredential)(nil)).
		Where("database_id = ?", databaseID).
		Exists(ctx)
	return exists, err
}
