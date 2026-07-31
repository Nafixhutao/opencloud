package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"

	"github.com/nazxf/opencloud/backend/internal/model"
)

// DomainRepo owns all customer-domain persistence. Customer methods require an
// account ID; explicitly named worker methods accept server-generated IDs.
type DomainRepo struct {
	db bun.IDB
}

// NewDomainRepo constructs a DomainRepo.
func NewDomainRepo(db bun.IDB) *DomainRepo { return &DomainRepo{db: db} }

// WithDB returns a copy bound to a transaction or dedicated connection.
func (r *DomainRepo) WithDB(db bun.IDB) *DomainRepo { return &DomainRepo{db: db} }

// LockCreateRequest serializes hostname and idempotency decisions.
func (r *DomainRepo) LockCreateRequest(
	ctx context.Context,
	accountID uuid.UUID,
	hostname, idempotencyKey string,
) error {
	scopes := []string{"domain-hostname:" + hostname}
	if idempotencyKey != "" {
		scopes = append(scopes, "domain-idempotency:"+accountID.String()+":"+idempotencyKey)
	}
	for _, scope := range scopes {
		if _, err := r.db.NewRaw(
			`SELECT pg_advisory_xact_lock(hashtextextended(?, 0))`,
			scope,
		).Exec(ctx); err != nil {
			return err
		}
	}
	return nil
}

// LockSiteRouting serializes Caddy/DNS provider calls affecting one site. It
// must run on a dedicated session and must be paired with UnlockSiteRouting.
func (r *DomainRepo) LockSiteRouting(ctx context.Context, siteID uuid.UUID) error {
	_, err := r.db.NewRaw(
		`SELECT pg_advisory_lock(hashtextextended(?, 0))`,
		siteRoutingLockScope(siteID),
	).Exec(ctx)
	return err
}

// UnlockSiteRouting releases the session-scoped site routing lock.
func (r *DomainRepo) UnlockSiteRouting(ctx context.Context, siteID uuid.UUID) (bool, error) {
	var unlocked bool
	err := r.db.NewRaw(
		`SELECT pg_advisory_unlock(hashtextextended(?, 0))`,
		siteRoutingLockScope(siteID),
	).Scan(ctx, &unlocked)
	return unlocked, err
}

// AccountHostnameInUse rejects duplicate intent inside one tenant without
// revealing another tenant's pending, verified, or primary hostname state.
func (r *DomainRepo) AccountHostnameInUse(
	ctx context.Context,
	accountID uuid.UUID,
	hostname string,
) (bool, error) {
	var inUse bool
	err := r.db.NewRaw(`
		SELECT EXISTS (
			SELECT 1 FROM sites
			WHERE account_id = ? AND domain = ? AND deleted_at IS NULL
			UNION ALL
			SELECT 1 FROM domains
			WHERE account_id = ? AND hostname = ? AND deleted_at IS NULL
		)`,
		accountID,
		hostname,
		accountID,
		hostname,
	).Scan(ctx, &inUse)
	return inUse, err
}

// Create inserts a customer domain.
func (r *DomainRepo) Create(ctx context.Context, domain *model.Domain) error {
	now := time.Now().UTC()
	if domain.ID == uuid.Nil {
		domain.ID = uuid.New()
	}
	if len(domain.DNSRecordIDs) == 0 {
		domain.DNSRecordIDs = json.RawMessage(`[]`)
	}
	domain.CreatedAt = now
	domain.UpdatedAt = now
	_, err := r.db.NewInsert().Model(domain).Exec(ctx)
	return err
}

// GetByAccount returns one live domain only for its owning tenant.
func (r *DomainRepo) GetByAccount(
	ctx context.Context,
	accountID, domainID uuid.UUID,
) (*model.Domain, error) {
	domain := new(model.Domain)
	err := r.db.NewSelect().Model(domain).
		Where("account_id = ?", accountID).
		Where("id = ?", domainID).
		Where("deleted_at IS NULL").
		Scan(ctx)
	if err != nil {
		return nil, err
	}
	return domain, nil
}

// GetByAccountForUpdate locks one live tenant domain.
func (r *DomainRepo) GetByAccountForUpdate(
	ctx context.Context,
	accountID, domainID uuid.UUID,
) (*model.Domain, error) {
	domain := new(model.Domain)
	err := r.db.NewSelect().Model(domain).
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

// GetByAccountForUpdateIncludingDeleted supports idempotent DELETE polling.
func (r *DomainRepo) GetByAccountForUpdateIncludingDeleted(
	ctx context.Context,
	accountID, domainID uuid.UUID,
) (*model.Domain, error) {
	domain := new(model.Domain)
	err := r.db.NewSelect().Model(domain).
		Where("account_id = ?", accountID).
		Where("id = ?", domainID).
		For("UPDATE").
		Scan(ctx)
	if err != nil {
		return nil, err
	}
	return domain, nil
}

// GetByIdempotencyKey returns the original attach result.
func (r *DomainRepo) GetByIdempotencyKey(
	ctx context.Context,
	accountID uuid.UUID,
	idempotencyKey string,
) (*model.Domain, error) {
	domain := new(model.Domain)
	err := r.db.NewSelect().Model(domain).
		Where("account_id = ?", accountID).
		Where("idempotency_key = ?", idempotencyKey).
		Scan(ctx)
	if err != nil {
		return nil, err
	}
	return domain, nil
}

// ListBySite returns a bounded tenant-scoped collection.
func (r *DomainRepo) ListBySite(
	ctx context.Context,
	accountID, siteID uuid.UUID,
	limit, offset int,
) ([]model.Domain, int, error) {
	query := r.db.NewSelect().Model((*model.Domain)(nil)).
		Where("account_id = ?", accountID).
		Where("site_id = ?", siteID).
		Where("deleted_at IS NULL")
	total, err := query.Count(ctx)
	if err != nil {
		return nil, 0, err
	}
	var domains []model.Domain
	err = r.db.NewSelect().Model(&domains).
		Where("account_id = ?", accountID).
		Where("site_id = ?", siteID).
		Where("deleted_at IS NULL").
		Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Scan(ctx)
	return domains, total, err
}

// GetVerifiedActiveByHostname is the constant-query security boundary for
// Caddy's internal On-Demand TLS permission endpoint.
func (r *DomainRepo) GetVerifiedActiveByHostname(
	ctx context.Context,
	hostname string,
) (*model.Domain, error) {
	domain := new(model.Domain)
	err := r.db.NewSelect().Model(domain).
		Where("hostname = ?", hostname).
		Where("status = ?", model.DomainActive).
		Where("verified_at IS NOT NULL").
		Where("verification_consumed_at IS NOT NULL").
		Where("deleted_at IS NULL").
		Scan(ctx)
	if err != nil {
		return nil, err
	}
	return domain, nil
}

// HostnameAuthorizedForTLS allows active primary site hostnames below the
// configured platform suffix and verified, active custom domains. It returns
// only a boolean to avoid existence leaks. An empty suffix exists only for
// development and tests; production configuration rejects it.
func (r *DomainRepo) HostnameAuthorizedForTLS(
	ctx context.Context,
	hostname, siteDomainSuffix string,
) (bool, error) {
	var authorized bool
	err := r.db.NewRaw(`
		SELECT EXISTS (
			SELECT 1 FROM sites
			WHERE domain = ?
			  AND status = 'active'
			  AND deleted_at IS NULL
			  AND (? = '' OR domain LIKE '%.' || ?)
			UNION ALL
			SELECT 1
			FROM domains AS d
			JOIN sites AS s
			  ON s.id = d.site_id
			 AND s.account_id = d.account_id
			WHERE d.hostname = ?
			  AND d.status = 'active'
			  AND d.verified_at IS NOT NULL
			  AND d.verification_consumed_at IS NOT NULL
			  AND d.deleted_at IS NULL
			  AND s.status = 'active'
			  AND s.deleted_at IS NULL
		)`,
		hostname,
		siteDomainSuffix,
		siteDomainSuffix,
		hostname,
	).Scan(ctx, &authorized)
	return authorized, err
}

// GetForWorker is an intentional unscoped lookup for durable server-generated
// job payloads.
func (r *DomainRepo) GetForWorker(ctx context.Context, domainID uuid.UUID) (*model.Domain, error) {
	domain := new(model.Domain)
	err := r.db.NewSelect().Model(domain).Where("id = ?", domainID).Scan(ctx)
	if err != nil {
		return nil, err
	}
	return domain, nil
}

// GetForWorkerForUpdate locks a worker-owned transition.
func (r *DomainRepo) GetForWorkerForUpdate(
	ctx context.Context,
	domainID uuid.UUID,
) (*model.Domain, error) {
	domain := new(model.Domain)
	err := r.db.NewSelect().Model(domain).
		Where("id = ?", domainID).
		For("UPDATE").
		Scan(ctx)
	if err != nil {
		return nil, err
	}
	return domain, nil
}

// ListForSiteDeletion locks every live domain owned by a site so the site
// deletion transaction can retain verified hostname claims while durable DNS
// cleanup is queued. Claims are released only at each domain tombstone.
func (r *DomainRepo) ListForSiteDeletion(
	ctx context.Context,
	accountID, siteID uuid.UUID,
) ([]model.Domain, error) {
	var domains []model.Domain
	err := r.db.NewSelect().Model(&domains).
		Where("account_id = ?", accountID).
		Where("site_id = ?", siteID).
		Where("deleted_at IS NULL").
		For("UPDATE").
		Scan(ctx)
	return domains, err
}

// SiteHasLiveDomains reports whether site teardown still requires domain
// lifecycle work. Pending domains count because their durable intent must be
// tombstoned even though they do not hold a global hostname claim.
func (r *DomainRepo) SiteHasLiveDomains(
	ctx context.Context,
	accountID, siteID uuid.UUID,
) (bool, error) {
	var exists bool
	err := r.db.NewRaw(`
		SELECT EXISTS (
			SELECT 1 FROM domains
			WHERE account_id = ? AND site_id = ? AND deleted_at IS NULL
		)
	`, accountID, siteID).Scan(ctx, &exists)
	return exists, err
}

// SiteHasRoutableDomains reports whether resume must restore verified custom
// hostnames after the primary site runtime starts.
func (r *DomainRepo) SiteHasRoutableDomains(
	ctx context.Context,
	accountID, siteID uuid.UUID,
) (bool, error) {
	var exists bool
	err := r.db.NewRaw(`
		SELECT EXISTS (
			SELECT 1 FROM domains
			WHERE account_id = ?
			  AND site_id = ?
			  AND verified_at IS NOT NULL
			  AND verification_consumed_at IS NOT NULL
			  AND deleted_at IS NULL
			  AND status IN (?)
		)
	`, accountID, siteID, bun.List([]string{
		model.DomainDNSPending,
		model.DomainProvisioning,
		model.DomainActive,
	})).Scan(ctx, &exists)
	return exists, err
}

// SetCustomerStatus performs a tenant-scoped state change.
func (r *DomainRepo) SetCustomerStatus(
	ctx context.Context,
	accountID, domainID uuid.UUID,
	status string,
) error {
	result, err := r.db.NewUpdate().Model((*model.Domain)(nil)).
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
	return requireOneRow(result)
}

// RotateChallenge replaces the digest/expiry and returns the domain to pending.
func (r *DomainRepo) RotateChallenge(
	ctx context.Context,
	accountID, domainID uuid.UUID,
	digest []byte,
	expiresAt time.Time,
) error {
	result, err := r.db.NewUpdate().Model((*model.Domain)(nil)).
		Set("status = ?", model.DomainPending).
		Set("verification_token_digest = ?", digest).
		Set("verification_expires_at = ?", expiresAt).
		Set("verification_consumed_at = NULL").
		Set("verified_at = NULL").
		Set("last_error = NULL").
		Set("updated_at = now()").
		Where("account_id = ?", accountID).
		Where("id = ?", domainID).
		Where("deleted_at IS NULL").
		Exec(ctx)
	if err != nil {
		return err
	}
	return requireOneRow(result)
}

// TryClaimHostname atomically contends with primary-site and other verified
// custom-domain claims. A false result is a normal verification loser, not a
// database error that should enter the provider retry loop.
func (r *DomainRepo) TryClaimHostname(
	ctx context.Context,
	domainID uuid.UUID,
	hostname string,
) (bool, error) {
	result, err := r.db.NewRaw(`
		INSERT INTO hostname_claims (hostname, domain_id)
		VALUES (?, ?)
		ON CONFLICT (hostname) DO NOTHING
	`, hostname, domainID).Exec(ctx)
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	if rows == 1 {
		return true, nil
	}
	var owned bool
	err = r.db.NewRaw(`
		SELECT EXISTS (
			SELECT 1 FROM hostname_claims
			WHERE hostname = ? AND domain_id = ?
		)
	`, hostname, domainID).Scan(ctx, &owned)
	return owned, err
}

// MarkVerified consumes the challenge exactly once and advances DNS setup.
func (r *DomainRepo) MarkVerified(
	ctx context.Context,
	domainID uuid.UUID,
	verifiedAt time.Time,
) error {
	result, err := r.db.NewUpdate().Model((*model.Domain)(nil)).
		Set("status = ?", model.DomainDNSPending).
		Set("verified_at = ?", verifiedAt).
		Set("verification_consumed_at = ?", verifiedAt).
		Set("last_error = NULL").
		Set("updated_at = now()").
		Where("id = ?", domainID).
		Where("status = ?", model.DomainVerifying).
		Where("verification_consumed_at IS NULL").
		Where("verification_expires_at > ?", verifiedAt).
		Exec(ctx)
	if err != nil {
		return err
	}
	return requireOneRow(result)
}

// SetWorkerStatus updates a provider-driven transition.
func (r *DomainRepo) SetWorkerStatus(
	ctx context.Context,
	domainID uuid.UUID,
	status string,
	lastError *string,
) error {
	result, err := r.db.NewUpdate().Model((*model.Domain)(nil)).
		Set("status = ?", status).
		Set("last_error = ?", lastError).
		Set("updated_at = now()").
		Where("id = ?", domainID).
		Exec(ctx)
	if err != nil {
		return err
	}
	return requireOneRow(result)
}

// SetDNSRecordIDs stores canonical provider object identifiers only when their
// value changed. The caller owns the matching audit transaction.
func (r *DomainRepo) SetDNSRecordIDs(
	ctx context.Context,
	domainID uuid.UUID,
	recordIDs []string,

) (bool, error) {
	recordIDs = canonicalRecordIDs(recordIDs)
	raw, err := json.Marshal(recordIDs)
	if err != nil {
		return false, err
	}
	result, err := r.db.NewUpdate().Model((*model.Domain)(nil)).
		Set("dns_record_ids = ?::jsonb", string(raw)).
		Set("updated_at = now()").
		Where("id = ?", domainID).
		Where("dns_record_ids IS DISTINCT FROM ?::jsonb", string(raw)).
		Exec(ctx)
	if err != nil {
		return false, err
	}
	count, err := result.RowsAffected()
	return count == 1, err
}

// SetCertificate records the public certificate observation.
func (r *DomainRepo) SetCertificate(
	ctx context.Context,
	domainID uuid.UUID,
	status string,
	expiresAt *time.Time,
	lastError *string,
	observedAt *time.Time,
) error {
	result, err := r.db.NewUpdate().Model((*model.Domain)(nil)).
		Set("cert_status = ?", status).
		Set("cert_expires_at = ?", expiresAt).
		Set("cert_observed_at = ?", observedAt).
		Set("last_error = ?", lastError).
		Set("updated_at = now()").
		Where("id = ?", domainID).
		Exec(ctx)
	if err != nil {
		return err
	}
	return requireOneRow(result)
}

// TouchCertificateObservation records a successful probe that did not change
// certificate state. It intentionally leaves updated_at unchanged so normal
// reconciliation does not masquerade as a domain configuration mutation.
func (r *DomainRepo) TouchCertificateObservation(
	ctx context.Context,
	domainID uuid.UUID,
	observedAt time.Time,
) error {
	result, err := r.db.NewUpdate().Model((*model.Domain)(nil)).
		Set("cert_observed_at = ?", observedAt.UTC()).
		Where("id = ?", domainID).
		Exec(ctx)
	if err != nil {
		return err
	}
	return requireOneRow(result)
}

// MarkReconciled records a bounded reconciliation scan. The caller appends the
// corresponding audit and completes the reconciliation job in the same tx.
func (r *DomainRepo) MarkReconciled(ctx context.Context, domainID uuid.UUID, at time.Time) error {
	result, err := r.db.NewUpdate().Model((*model.Domain)(nil)).
		Set("last_reconciled_at = ?", at).
		Set("updated_at = now()").
		Where("id = ?", domainID).
		Exec(ctx)
	if err != nil {
		return err
	}
	return requireOneRow(result)
}

// MarkDeleted completes cleanup while retaining an auditable tombstone.
func (r *DomainRepo) MarkDeleted(ctx context.Context, domainID uuid.UUID) error {
	result, err := r.db.NewUpdate().Model((*model.Domain)(nil)).
		Set("status = ?", model.DomainDeleted).
		Set("dns_record_ids = '[]'::jsonb").
		Set("cert_status = ?", model.CertNone).
		Set("cert_expires_at = NULL").
		Set("last_error = NULL").
		Set("deleted_at = COALESCE(deleted_at, now())").
		Set("updated_at = now()").
		Where("id = ?", domainID).
		Exec(ctx)
	if err != nil {
		return err
	}
	return requireOneRow(result)
}

// RouteHostnames returns the complete desired Caddy matcher for an active site.
// A suspended or deleting site returns no hostnames, so domain work cannot
// recreate public routing after the site lifecycle removed it.
func (r *DomainRepo) RouteHostnames(ctx context.Context, siteID uuid.UUID) ([]string, error) {
	var hostnames []string
	err := r.db.NewRaw(`
		SELECT hostname
		FROM (
			SELECT domain AS hostname, 0 AS ordering
			FROM sites
			WHERE id = ?
			  AND status = 'active'
			  AND deleted_at IS NULL
			UNION ALL
			SELECT d.hostname, 1 AS ordering
			FROM domains AS d
			JOIN sites AS s ON s.id = d.site_id
			JOIN hostname_claims AS c
			  ON c.hostname = d.hostname
			 AND c.domain_id = d.id
			WHERE d.site_id = ?
			  AND d.verified_at IS NOT NULL
			  AND d.verification_consumed_at IS NOT NULL
			  AND d.deleted_at IS NULL
			  AND d.status IN ('dns_pending', 'provisioning', 'active')
			  AND s.status = 'active'
			  AND s.deleted_at IS NULL
		) AS desired
		ORDER BY ordering, hostname`,
		siteID,
		siteID,
	).Scan(ctx, &hostnames)
	return hostnames, err
}

// ListRoutableIDsBySite returns verified, non-deleting domains that should be
// restored only after their owning site has durably resumed to active.
func (r *DomainRepo) ListRoutableIDsBySite(
	ctx context.Context,
	accountID, siteID uuid.UUID,
) ([]uuid.UUID, error) {
	var ids []uuid.UUID
	err := r.db.NewSelect().Model((*model.Domain)(nil)).
		Column("id").
		Where("account_id = ?", accountID).
		Where("site_id = ?", siteID).
		Where("verified_at IS NOT NULL").
		Where("verification_consumed_at IS NOT NULL").
		Where("deleted_at IS NULL").
		Where("status IN (?)", bun.List([]string{
			model.DomainDNSPending,
			model.DomainProvisioning,
			model.DomainActive,
		})).
		Order("id ASC").
		Scan(ctx, &ids)
	return ids, err
}

// ListReconciliationCandidates returns bounded worker-only desired state.
func (r *DomainRepo) ListReconciliationCandidates(
	ctx context.Context,
	limit int,
	activeBefore time.Time,
) ([]model.Domain, error) {
	if limit <= 0 || limit > 100 {
		limit = 100
	}
	var domains []model.Domain
	err := r.db.NewSelect().Model(&domains).
		Where("deleted_at IS NULL").
		Where(`(
			status IN (?) OR
			(status = ? AND (last_reconciled_at IS NULL OR last_reconciled_at <= ?))
		)`, bun.List([]string{
			model.DomainVerifying,
			model.DomainDNSPending,
			model.DomainProvisioning,
			model.DomainDeleting,
		}), model.DomainActive, activeBefore).
		Where(`NOT EXISTS (
			SELECT 1
			FROM jobs AS j
			WHERE j.status IN ('queued', 'running')
			  AND j.payload ->> 'domain_id' = d.id::text
		)`).
		OrderExpr("COALESCE(last_reconciled_at, created_at) ASC").
		Limit(limit).
		Scan(ctx)
	return domains, err
}

func canonicalRecordIDs(values []string) []string {
	unique := make(map[string]struct{}, len(values))
	for _, value := range values {
		if value != "" {
			unique[value] = struct{}{}
		}
	}
	result := make([]string, 0, len(unique))
	for value := range unique {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

// DecodeDNSRecordIDs parses persisted provider object IDs.
func DecodeDNSRecordIDs(raw json.RawMessage) ([]string, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var ids []string
	if err := json.Unmarshal(raw, &ids); err != nil {
		return nil, err
	}
	return ids, nil
}

// IsNoRows is a small cross-layer helper for repository consumers.
func IsNoRows(err error) bool { return err == sql.ErrNoRows }
