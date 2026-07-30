package repository

import (
	"context"
	"database/sql"
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"

	"github.com/nazxf/opencloud/backend/internal/model"
)

// DomainRepo manages tenant-owned domains. Customer methods always require an
// account ID; explicitly named Worker methods are reserved for async jobs.
type DomainRepo struct {
	db bun.IDB
}

// NewDomainRepo constructs a DomainRepo.
func NewDomainRepo(db bun.IDB) *DomainRepo {
	return &DomainRepo{db: db}
}

// WithDB returns a copy using db.
func (r *DomainRepo) WithDB(db bun.IDB) *DomainRepo {
	return &DomainRepo{db: db}
}

// Create inserts a domain row.
func (r *DomainRepo) Create(ctx context.Context, domain *model.Domain) error {
	now := time.Now().UTC()
	if domain.ID == uuid.Nil {
		domain.ID = uuid.New()
	}
	domain.CreatedAt = now
	domain.UpdatedAt = now
	_, err := r.db.NewInsert().Model(domain).Exec(ctx)
	return err
}

// GetByAccount returns a live domain only when it belongs to accountID.
func (r *DomainRepo) GetByAccount(ctx context.Context, accountID, domainID uuid.UUID) (*model.Domain, error) {
	domain := new(model.Domain)
	err := r.db.NewSelect().
		Model(domain).
		Where("account_id = ?", accountID).
		Where("id = ?", domainID).
		Where("deleted_at IS NULL").
		Scan(ctx)
	if err != nil {
		return nil, err
	}
	return domain, nil
}

// GetByAccountForUpdate locks a tenant-owned live domain.
func (r *DomainRepo) GetByAccountForUpdate(ctx context.Context, accountID, domainID uuid.UUID) (*model.Domain, error) {
	domain := new(model.Domain)
	err := r.db.NewSelect().
		Model(domain).
		Where("account_id = ?", accountID).
		Where("id = ?", domainID).
		Where("deleted_at IS NULL").
		For("UPDATE").
		Scan(ctx)
	if err != nil {
		return nil, err
	}
	return domain, nil
}

// ListByAccount returns paginated live domains owned by accountID.
func (r *DomainRepo) ListByAccount(ctx context.Context, accountID uuid.UUID, limit, offset int) ([]model.Domain, int, error) {
	if limit <= 0 {
		limit = 25
	}
	if limit > 100 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}

	query := r.db.NewSelect().
		Model((*model.Domain)(nil)).
		Where("account_id = ?", accountID).
		Where("deleted_at IS NULL")
	total, err := query.Count(ctx)
	if err != nil {
		return nil, 0, err
	}

	var domains []model.Domain
	err = r.db.NewSelect().
		Model(&domains).
		Where("account_id = ?", accountID).
		Where("deleted_at IS NULL").
		Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Scan(ctx)
	return domains, total, err
}

// ListBySite returns live domains attached to a site that belongs to accountID.
func (r *DomainRepo) ListBySite(ctx context.Context, accountID, siteID uuid.UUID) ([]model.Domain, error) {
	var domains []model.Domain
	err := r.db.NewSelect().
		Model(&domains).
		Where("account_id = ?", accountID).
		Where("site_id = ?", siteID).
		Where("deleted_at IS NULL").
		Order("created_at DESC").
		Scan(ctx)
	return domains, err
}

// GetByHostname returns a domain by hostname for worker-only cross-account
// lookup (used by the Caddy permission endpoint). Returns nil with no error
// when no verified+active domain exists.
func (r *DomainRepo) GetByHostname(ctx context.Context, hostname string) (*model.Domain, error) {
	domain := new(model.Domain)
	err := r.db.NewSelect().
		Model(domain).
		Where("hostname = ?", hostname).
		Where("deleted_at IS NULL").
		Scan(ctx)
	if err != nil {
		return nil, err
	}
	return domain, nil
}

// GetVerifiedActiveByHostname returns a domain only when it is verified and
// active. This is the security boundary for the Caddy on-demand TLS permission
// endpoint — arbitrary hostnames must never pass.
func (r *DomainRepo) GetVerifiedActiveByHostname(ctx context.Context, hostname string) (*model.Domain, error) {
	domain := new(model.Domain)
	err := r.db.NewSelect().
		Model(domain).
		Where("hostname = ?", hostname).
		Where("status = ?", model.DomainActive).
		Where("verified_at IS NOT NULL").
		Where("deleted_at IS NULL").
		Scan(ctx)
	if err != nil {
		return nil, err
	}
	return domain, nil
}

// GetForWorker is the deliberate unscoped lookup used only by durable job
// handlers after a server-generated domain ID is supplied.
func (r *DomainRepo) GetForWorker(ctx context.Context, domainID uuid.UUID) (*model.Domain, error) {
	domain := new(model.Domain)
	err := r.db.NewSelect().Model(domain).Where("id = ?", domainID).Scan(ctx)
	if err != nil {
		return nil, err
	}
	return domain, nil
}

// GetForWorkerForUpdate locks a worker-owned domain transition.
func (r *DomainRepo) GetForWorkerForUpdate(ctx context.Context, domainID uuid.UUID) (*model.Domain, error) {
	domain := new(model.Domain)
	err := r.db.NewSelect().
		Model(domain).
		Where("id = ?", domainID).
		For("UPDATE").
		Scan(ctx)
	if err != nil {
		return nil, err
	}
	return domain, nil
}

// SetStatus updates a domain state scoped to the owning account.
func (r *DomainRepo) SetStatus(ctx context.Context, accountID, domainID uuid.UUID, status string) error {
	result, err := r.db.NewUpdate().
		Model((*model.Domain)(nil)).
		Set("status = ?", status).
		Set("last_error = NULL").
		Set("updated_at = now()").
		Where("account_id = ?", accountID).
		Where("id = ?", domainID).
		Where("deleted_at IS NULL").
		Exec(ctx)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// SetWorkerStatus updates state after a worker operation.
func (r *DomainRepo) SetWorkerStatus(ctx context.Context, domainID uuid.UUID, status string, lastError *string) error {
	result, err := r.db.NewUpdate().
		Model((*model.Domain)(nil)).
		Set("status = ?", status).
		Set("last_error = ?", lastError).
		Set("updated_at = now()").
		Where("id = ?", domainID).
		Exec(ctx)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// SetVerified marks a domain as verified with a timestamp.
func (r *DomainRepo) SetVerified(ctx context.Context, domainID uuid.UUID, verifiedAt time.Time) error {
	result, err := r.db.NewUpdate().
		Model((*model.Domain)(nil)).
		Set("status = ?", model.DomainVerified).
		Set("verified_at = ?", verifiedAt).
		Set("last_error = NULL").
		Set("updated_at = now()").
		Where("id = ?", domainID).
		Where("verified_at IS NULL").
		Exec(ctx)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// SetCertStatus updates the certificate state for a domain.
func (r *DomainRepo) SetCertStatus(ctx context.Context, domainID uuid.UUID, certStatus string, expiresAt *time.Time) error {
	result, err := r.db.NewUpdate().
		Model((*model.Domain)(nil)).
		Set("cert_status = ?", certStatus).
		Set("cert_expires_at = ?", expiresAt).
		Set("updated_at = now()").
		Where("id = ?", domainID).
		Exec(ctx)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// MarkDeleted makes a completed delete visible while excluding it from
// normal customer lists.
func (r *DomainRepo) MarkDeleted(ctx context.Context, domainID uuid.UUID) error {
	result, err := r.db.NewUpdate().
		Model((*model.Domain)(nil)).
		Set("status = ?", model.DomainDeleted).
		Set("last_error = NULL").
		Set("deleted_at = COALESCE(deleted_at, now())").
		Set("updated_at = now()").
		Where("id = ?", domainID).
		Exec(ctx)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// ListVerificationCandidates returns domains still awaiting verification for
// periodic retry scanning.
func (r *DomainRepo) ListVerificationCandidates(ctx context.Context, limit int) ([]model.Domain, error) {
	if limit <= 0 {
		limit = 50
	}
	var domains []model.Domain
	err := r.db.NewSelect().
		Model(&domains).
		Where("status IN (?)", bun.List([]string{
			model.DomainVerifying,
		})).
		Order("updated_at ASC").
		Limit(limit).
		Scan(ctx)
	return domains, err
}

// ListCertExpiringSoon returns active domains whose certificate is expiring
// within the given window.
func (r *DomainRepo) ListCertExpiringSoon(ctx context.Context, before time.Time, limit int) ([]model.Domain, error) {
	if limit <= 0 {
		limit = 50
	}
	var domains []model.Domain
	err := r.db.NewSelect().
		Model(&domains).
		Where("status = ?", model.DomainActive).
		Where("cert_status = ?", model.CertActive).
		Where("cert_expires_at < ?", before).
		Where("cert_auto_renew = true").
		Where("deleted_at IS NULL").
		Order("cert_expires_at ASC").
		Limit(limit).
		Scan(ctx)
	return domains, err
}