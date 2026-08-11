package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"

	"github.com/nazxf/opencloud/backend/internal/apperr"
	"github.com/nazxf/opencloud/backend/internal/domainverify"
	"github.com/nazxf/opencloud/backend/internal/model"
	"github.com/nazxf/opencloud/backend/internal/provisioner"
	"github.com/nazxf/opencloud/backend/internal/repository"
)

const (
	domainChallengeTTL         = time.Hour
	domainVerificationAttempts = 20
	domainLifecycleAttempts    = 12
)

// DomainService owns tenant-scoped custom-domain intent and transactions.
type DomainService struct {
	db          *bun.DB
	domains     *repository.DomainRepo
	sites       *repository.SiteRepo
	jobs        *repository.JobRepo
	audit       *repository.AuditRepo
	dns         provisioner.DomainDNSProvisioner
	signer      *domainverify.Signer
	ingressIPv4 string
	enabled     bool
}

// NewDomainService constructs a DomainService.
func NewDomainService(
	db *bun.DB,
	domains *repository.DomainRepo,
	sites *repository.SiteRepo,
	jobs *repository.JobRepo,
	audit *repository.AuditRepo,
	dns provisioner.DomainDNSProvisioner,
	signer *domainverify.Signer,
	ingressIPv4 string,
	enabled bool,
) *DomainService {
	return &DomainService{
		db: db, domains: domains, sites: sites, jobs: jobs, audit: audit,
		dns: dns, signer: signer, ingressIPv4: ingressIPv4, enabled: enabled,
	}
}

// AttachDomainRequest is the customer-facing create payload. Cloudflare OAuth
// automation is feature-gated separately; the universal flow defaults manual.
type AttachDomainRequest struct {
	Hostname string `json:"hostname"`
}

// Attach creates one pending challenge and its audit row atomically.
func (s *DomainService) Attach(
	ctx context.Context,
	actorUserID string,
	accountID, siteID uuid.UUID,
	idempotencyKey string,
	req AttachDomainRequest,
) (*model.Domain, error) {
	if !s.enabled || s.signer == nil || s.dns == nil {
		return nil, apperr.Unavailable("custom domains are not enabled")
	}
	hostname, err := normalizeCustomHostname(req.Hostname)
	if err != nil {
		return nil, err
	}
	idempotencyKey = strings.TrimSpace(idempotencyKey)
	if len(idempotencyKey) > 128 {
		return nil, apperr.Validation(
			"idempotency key is too long",
			apperr.FieldIssue{Field: "Idempotency-Key", Issue: "max 128"},
		)
	}

	var created *model.Domain
	err = s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		domains := s.domains.WithDB(tx)
		sites := s.sites.WithDB(tx)
		audit := s.audit.WithDB(tx)

		site, err := sites.GetByAccountForUpdate(ctx, accountID, siteID)
		if err != nil {
			return err
		}
		if site.Status != model.SiteActive {
			return apperr.Conflict("site must be active before attaching a domain")
		}
		if err := domains.LockCreateRequest(ctx, accountID, hostname, idempotencyKey); err != nil {
			return err
		}
		if idempotencyKey != "" {
			prior, priorErr := domains.GetByIdempotencyKey(ctx, accountID, idempotencyKey)
			if priorErr == nil {
				if prior.SiteID != siteID || prior.Hostname != hostname {
					return apperr.Conflict("idempotency key was already used for another domain")
				}
				created = prior
				return nil
			}
			if !errors.Is(priorErr, sql.ErrNoRows) {
				return priorErr
			}
		}
		inUse, err := domains.AccountHostnameInUse(ctx, accountID, hostname)
		if err != nil {
			return err
		}
		if inUse {
			return apperr.Conflict("hostname is unavailable")
		}

		domainID := uuid.New()
		expiresAt, digest, err := s.signer.Issue(
			time.Now().UTC(), domainChallengeTTL, domainID, accountID, hostname,
		)
		if err != nil {
			return err
		}
		domain := &model.Domain{
			ID: domainID, AccountID: accountID, SiteID: siteID,
			Hostname: hostname, Status: model.DomainPending,
			VerificationTokenDigest: digest,
			VerificationExpiresAt:   expiresAt,
			DNSProvider:             model.DNSProviderManual,
			CertStatus:              model.CertNone, CertAutoRenew: true,
			IdempotencyKey: optionalString(idempotencyKey),
		}
		if err := domains.Create(ctx, domain); err != nil {
			return err
		}
		actor := actorUserID
		aid := accountID
		if err := audit.Append(ctx, repository.Entry{
			AccountID: &aid, ActorID: &actor,
			Action: model.AuditDomainAttached,
			Target: strPtr(domain.ID.String()),
			Metadata: map[string]any{
				"hostname": hostname,
				"site_id":  siteID.String(),
				"provider": model.DNSProviderManual,
			},
		}); err != nil {
			return err
		}
		created = domain
		return nil
	})
	if err != nil {
		return nil, s.mapDomainWriteError(err, "failed to attach domain")
	}
	return created, nil
}

// ListBySite returns one bounded tenant-scoped collection.
func (s *DomainService) ListBySite(
	ctx context.Context,
	accountID, siteID uuid.UUID,
	page, perPage int,
) ([]model.Domain, int, error) {
	if _, err := s.sites.GetByAccount(ctx, accountID, siteID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, 0, apperr.NotFound("site not found")
		}
		return nil, 0, apperr.Internal("failed to load site").Wrap(err)
	}
	if page < 1 {
		page = 1
	}
	if perPage <= 0 {
		perPage = 25
	}
	if perPage > 100 {
		perPage = 100
	}
	domains, total, err := s.domains.ListBySite(
		ctx, accountID, siteID, perPage, (page-1)*perPage,
	)
	if err != nil {
		return nil, 0, apperr.Internal("failed to list domains").Wrap(err)
	}
	return domains, total, nil
}

// Get returns one tenant-owned live domain.
func (s *DomainService) Get(
	ctx context.Context,
	accountID, domainID uuid.UUID,
) (*model.Domain, error) {
	domain, err := s.domains.GetByAccount(ctx, accountID, domainID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, apperr.NotFound("domain not found")
		}
		return nil, apperr.Internal("failed to load domain").Wrap(err)
	}
	return domain, nil
}

// Instructions reconstructs the challenge from the external HMAC key. The raw
// token is never read from PostgreSQL because it is never stored there.
func (s *DomainService) Instructions(
	ctx context.Context,
	accountID, domainID uuid.UUID,
) (provisioner.DomainInstructions, error) {
	if !s.enabled || s.signer == nil || s.dns == nil {
		return provisioner.DomainInstructions{}, apperr.Unavailable("custom domains are not enabled")
	}
	domain, err := s.Get(ctx, accountID, domainID)
	if err != nil {
		return provisioner.DomainInstructions{}, err
	}
	if domain.VerificationConsumedAt != nil {
		return provisioner.DomainInstructions{
			VerificationExpiresAt: domain.VerificationExpiresAt,
			Records: []provisioner.DNSRecord{{
				Type: "A", Name: domain.Hostname, Content: s.ingressIPv4, TTL: 300,
			}},
		}, nil
	}
	if !time.Now().UTC().Before(domain.VerificationExpiresAt) {
		return provisioner.DomainInstructions{}, apperr.Conflict(
			"verification challenge expired; rotate it before retrying",
		)
	}
	token := s.signer.Token(
		domain.ID, domain.AccountID, domain.Hostname, domain.VerificationExpiresAt,
	)
	if !domainverify.Matches(token, domain.VerificationTokenDigest) {
		return provisioner.DomainInstructions{}, apperr.Internal(
			"domain verification challenge is inconsistent",
		)
	}
	instructions, err := s.dns.Instructions(ctx, s.dnsSpec(domain, token))
	if err != nil {
		return provisioner.DomainInstructions{}, apperr.Internal(
			"failed to build DNS instructions",
		).Wrap(err)
	}
	instructions.VerificationExpiresAt = domain.VerificationExpiresAt
	return instructions, nil
}

// RotateChallenge invalidates the old challenge and returns the domain to a
// pending state. Verified domains cannot be rotated silently.
func (s *DomainService) RotateChallenge(
	ctx context.Context,
	actorUserID string,
	accountID, domainID uuid.UUID,
) (*model.Domain, error) {
	if !s.enabled || s.signer == nil {
		return nil, apperr.Unavailable("custom domains are not enabled")
	}
	var result *model.Domain
	err := s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		domains := s.domains.WithDB(tx)
		audit := s.audit.WithDB(tx)
		domain, err := domains.GetByAccountForUpdate(ctx, accountID, domainID)
		if err != nil {
			return err
		}
		if domain.VerifiedAt != nil || domain.VerificationConsumedAt != nil {
			return apperr.Conflict("verified domains cannot rotate ownership challenges")
		}
		expiresAt, digest, err := s.signer.Issue(
			time.Now().UTC(), domainChallengeTTL,
			domain.ID, domain.AccountID, domain.Hostname,
		)
		if err != nil {
			return err
		}
		if err := domains.RotateChallenge(ctx, accountID, domainID, digest, expiresAt); err != nil {
			return err
		}
		actor := actorUserID
		aid := accountID
		if err := audit.Append(ctx, repository.Entry{
			AccountID: &aid, ActorID: &actor,
			Action:   model.AuditDomainChallengeRotated,
			Target:   strPtr(domainID.String()),
			Metadata: map[string]any{"hostname": domain.Hostname},
		}); err != nil {
			return err
		}
		domain.Status = model.DomainPending
		domain.VerificationExpiresAt = expiresAt
		domain.VerificationTokenDigest = digest
		result = domain
		return nil
	})
	if err != nil {
		return nil, s.mapDomainWriteError(err, "failed to rotate challenge")
	}
	return result, nil
}

// Verify queues a durable public-DNS ownership check.
func (s *DomainService) Verify(
	ctx context.Context,
	actorUserID string,
	accountID, domainID uuid.UUID,
) (*model.Domain, error) {
	if !s.enabled || s.signer == nil || s.dns == nil {
		return nil, apperr.Unavailable("custom domains are not enabled")
	}
	var result *model.Domain
	err := s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		domains := s.domains.WithDB(tx)
		jobs := s.jobs.WithDB(tx)
		audit := s.audit.WithDB(tx)
		domain, err := domains.GetByAccountForUpdate(ctx, accountID, domainID)
		if err != nil {
			return err
		}
		if domain.VerifiedAt != nil && domain.Status == model.DomainFailed {
			if err := domains.SetCustomerStatus(ctx, accountID, domainID, model.DomainDNSPending); err != nil {
				return err
			}
			aid := accountID
			if _, err := jobs.EnqueueUniqueDomain(
				ctx, aid, model.JobProvisionDomain, domainID, domainLifecycleAttempts,
			); err != nil {
				return err
			}
			actor := actorUserID
			if err := audit.Append(ctx, repository.Entry{
				AccountID: &aid, ActorID: &actor,
				Action: model.AuditDomainVerificationQueued,
				Target: strPtr(domainID.String()),
				Metadata: map[string]any{
					"hostname": domain.Hostname,
					"retry":    "routing",
				},
			}); err != nil {
				return err
			}
			domain.Status = model.DomainDNSPending
			result = domain
			return nil
		}
		if domain.VerifiedAt != nil || contains([]string{
			model.DomainDNSPending, model.DomainProvisioning, model.DomainActive,
		}, domain.Status) {
			result = domain
			return nil
		}
		if !contains([]string{model.DomainPending, model.DomainFailed, model.DomainVerifying}, domain.Status) {
			return apperr.Conflict("domain cannot be verified from its current state")
		}
		if !time.Now().UTC().Before(domain.VerificationExpiresAt) {
			return apperr.Conflict("verification challenge expired; rotate it before retrying")
		}
		statusChanged := domain.Status != model.DomainVerifying
		if statusChanged {
			if err := domains.SetCustomerStatus(ctx, accountID, domainID, model.DomainVerifying); err != nil {
				return err
			}
		}
		aid := accountID
		inserted, err := jobs.EnqueueUniqueDomain(
			ctx, aid, model.JobVerifyDomain, domainID, domainVerificationAttempts,
		)
		if err != nil {
			return err
		}
		if !statusChanged && !inserted {
			domain.Status = model.DomainVerifying
			result = domain
			return nil
		}
		actor := actorUserID
		if err := audit.Append(ctx, repository.Entry{
			AccountID: &aid, ActorID: &actor,
			Action:   model.AuditDomainVerificationQueued,
			Target:   strPtr(domainID.String()),
			Metadata: map[string]any{"hostname": domain.Hostname},
		}); err != nil {
			return err
		}
		domain.Status = model.DomainVerifying
		result = domain
		return nil
	})
	if err != nil {
		return nil, s.mapDomainWriteError(err, "failed to queue domain verification")
	}
	return result, nil
}

// Detach queues idempotent DNS and Caddy cleanup.
func (s *DomainService) Detach(
	ctx context.Context,
	actorUserID string,
	accountID, domainID uuid.UUID,
) (*model.Domain, error) {
	if !s.enabled || s.dns == nil {
		return nil, apperr.Unavailable("custom domains are not enabled")
	}
	var result *model.Domain
	err := s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		domains := s.domains.WithDB(tx)
		jobs := s.jobs.WithDB(tx)
		audit := s.audit.WithDB(tx)
		domain, err := domains.GetByAccountForUpdateIncludingDeleted(ctx, accountID, domainID)
		if err != nil {
			return err
		}
		if domain.Status == model.DomainDeleting || domain.Status == model.DomainDeleted {
			result = domain
			return nil
		}
		if err := domains.SetCustomerStatus(ctx, accountID, domainID, model.DomainDeleting); err != nil {
			return err
		}
		aid := accountID
		if _, err := jobs.EnqueueUniqueDomain(
			ctx, aid, model.JobDeprovisionDomain, domainID, domainLifecycleAttempts,
		); err != nil {
			return err
		}
		actor := actorUserID
		if err := audit.Append(ctx, repository.Entry{
			AccountID: &aid, ActorID: &actor,
			Action:   model.AuditDomainDetachQueued,
			Target:   strPtr(domainID.String()),
			Metadata: map[string]any{"hostname": domain.Hostname},
		}); err != nil {
			return err
		}
		domain.Status = model.DomainDeleting
		result = domain
		return nil
	})
	if err != nil {
		return nil, s.mapDomainWriteError(err, "failed to queue domain detachment")
	}
	return result, nil
}

func (s *DomainService) dnsSpec(domain *model.Domain, token string) provisioner.DomainDNSSpec {
	zoneID := ""
	if domain.DNSZoneID != nil {
		zoneID = *domain.DNSZoneID
	}
	return provisioner.DomainDNSSpec{
		DomainID: domain.ID, AccountID: domain.AccountID,
		Hostname: domain.Hostname, VerificationToken: token,
		IngressIPv4: s.ingressIPv4, Provider: domain.DNSProvider, ZoneID: zoneID,
	}
}

func (s *DomainService) mapDomainWriteError(err error, message string) error {
	if errors.Is(err, sql.ErrNoRows) {
		return apperr.NotFound("domain not found")
	}
	if apperr.As(err) != nil {
		return err
	}
	if uniqueViolation(err) {
		return apperr.Conflict("hostname or idempotency key is unavailable")
	}
	return apperr.Internal(message).Wrap(err)
}
