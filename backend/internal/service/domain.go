package service

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/uptrace/bun"

	"github.com/nazxf/opencloud/backend/internal/apperr"
	"github.com/nazxf/opencloud/backend/internal/model"
	"github.com/nazxf/opencloud/backend/internal/provisioner"
	"github.com/nazxf/opencloud/backend/internal/repository"
)

// DomainService owns tenant-scoped domain lifecycle transitions.
type DomainService struct {
	db        *bun.DB
	domains   *repository.DomainRepo
	sites     *repository.SiteRepo
	jobs      *repository.JobRepo
	audit     *repository.AuditRepo
	dns       provisioner.DNSProvisioner
}

// NewDomainService constructs a DomainService.
func NewDomainService(
	db *bun.DB,
	domains *repository.DomainRepo,
	sites *repository.SiteRepo,
	jobs *repository.JobRepo,
	audit *repository.AuditRepo,
	dns provisioner.DNSProvisioner,
) *DomainService {
	return &DomainService{
		db:      db,
		domains: domains,
		sites:   sites,
		jobs:    jobs,
		audit:   audit,
		dns:     dns,
	}
}

// AttachDomainRequest is the customer-facing attach payload.
type AttachDomainRequest struct {
	Hostname string `json:"hostname"`
}

type domainJobPayload struct {
	DomainID uuid.UUID `json:"domain_id"`
}

// Attach atomically validates the hostname, verifies site ownership, inserts a
// domain row with a verification token, enqueues a verify job, and audits.
func (s *DomainService) Attach(
	ctx context.Context,
	actorUserID string,
	accountID, siteID uuid.UUID,
	req AttachDomainRequest,
) (*model.Domain, error) {
	hostname, err := normalizeDomain(req.Hostname)
	if err != nil {
		return nil, err
	}

	// Verify the site belongs to the caller.
	_, err = s.sites.GetByAccount(ctx, accountID, siteID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, apperr.NotFound("site not found")
		}
		return nil, apperr.Internal("failed to load site").Wrap(err)
	}

	verificationType := model.VerificationTXT
	token := provisioner.NewToken()

	var created *model.Domain
	err = s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		domains := s.domains.WithDB(tx)
		jobs := s.jobs.WithDB(tx)
		audit := s.audit.WithDB(tx)

		// Check hostname uniqueness explicitly before insert for a clear error.
		existing, existingErr := domains.GetByHostname(ctx, hostname)
		if existingErr == nil && existing != nil {
			if existing.AccountID != accountID {
				return apperr.Conflict("hostname is already owned by another tenant")
			}
			return apperr.Conflict("hostname is already attached")
		}

		domain := &model.Domain{
			ID:                uuid.New(),
			AccountID:         accountID,
			SiteID:            &siteID,
			Hostname:          hostname,
			Status:            model.DomainPending,
			VerificationType:  &verificationType,
			VerificationToken: &token,
			DNSProvider:       model.DNSProviderManual,
			CertStatus:        model.CertNone,
			CertAutoRenew:     true,
		}
		if err := domains.Create(ctx, domain); err != nil {
			return err
		}
		aid := accountID
		if _, err := jobs.Enqueue(ctx, &aid, model.JobVerifyDomain, domainJobPayload{DomainID: domain.ID}); err != nil {
			return err
		}
		actor := actorUserID
		if err := audit.Append(ctx, repository.Entry{
			AccountID: &aid,
			ActorID:   &actor,
			Action:    model.AuditDomainAttachQueued,
			Target:    strPtr(domain.ID.String()),
			Metadata: map[string]any{
				"hostname": hostname,
				"site_id":  siteID.String(),
			},
		}); err != nil {
			return err
		}
		created = domain
		return nil
	})
	if err != nil {
		if apperr.As(err) != nil {
			return nil, err
		}
		if uniqueViolation(err) {
			return nil, apperr.Conflict("hostname is already in use")
		}
		return nil, apperr.Internal("failed to attach domain").Wrap(err)
	}
	return created, nil
}

// Get returns one domain without revealing cross-tenant existence.
func (s *DomainService) Get(ctx context.Context, accountID, domainID uuid.UUID) (*model.Domain, error) {
	domain, err := s.domains.GetByAccount(ctx, accountID, domainID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, apperr.NotFound("domain not found")
		}
		return nil, apperr.Internal("failed to load domain").Wrap(err)
	}
	return domain, nil
}

// ListBySite returns domains attached to a site owned by the caller.
func (s *DomainService) ListBySite(ctx context.Context, accountID, siteID uuid.UUID) ([]model.Domain, error) {
	_, err := s.sites.GetByAccount(ctx, accountID, siteID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, apperr.NotFound("site not found")
		}
		return nil, apperr.Internal("failed to load site").Wrap(err)
	}
	domains, err := s.domains.ListBySite(ctx, accountID, siteID)
	if err != nil {
		return nil, apperr.Internal("failed to list domains").Wrap(err)
	}
	return domains, nil
}

// Verify triggers a re-check of domain ownership. It enqueues a verification
// job if the domain is in a checkable state.
func (s *DomainService) Verify(
	ctx context.Context,
	actorUserID string,
	accountID, domainID uuid.UUID,
) (*model.Domain, error) {
	var result *model.Domain
	err := s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		domains := s.domains.WithDB(tx)
		jobs := s.jobs.WithDB(tx)
		audit := s.audit.WithDB(tx)

		domain, err := domains.GetByAccountForUpdate(ctx, accountID, domainID)
		if err != nil {
			return err
		}
		if domain.Status == model.DomainVerified || domain.Status == model.DomainActive {
			result = domain
			return nil
		}
		if domain.Status != model.DomainPending && domain.Status != model.DomainVerifying && domain.Status != model.DomainFailed {
			return apperr.Conflict("domain cannot be verified from its current state")
		}
		if err := domains.SetStatus(ctx, accountID, domainID, model.DomainVerifying); err != nil {
			return err
		}
		aid := accountID
		if _, err := jobs.Enqueue(ctx, &aid, model.JobVerifyDomain, domainJobPayload{DomainID: domainID}); err != nil {
			return err
		}
		actor := actorUserID
		if err := audit.Append(ctx, repository.Entry{
			AccountID: &aid,
			ActorID:   &actor,
			Action:    model.AuditDomainAttachQueued,
			Target:    strPtr(domainID.String()),
			Metadata:  map[string]any{"hostname": domain.Hostname, "from": domain.Status, "to": model.DomainVerifying},
		}); err != nil {
			return err
		}
		domain.Status = model.DomainVerifying
		result = domain
		return nil
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, apperr.NotFound("domain not found")
		}
		if apperr.As(err) != nil {
			return nil, err
		}
		return nil, apperr.Internal("failed to verify domain").Wrap(err)
	}
	return result, nil
}

// Detach queues idempotent domain removal. It transitions the domain to
// deleting and enqueues a deprovision job.
func (s *DomainService) Detach(
	ctx context.Context,
	actorUserID string,
	accountID, domainID uuid.UUID,
) (*model.Domain, error) {
	var result *model.Domain
	err := s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		domains := s.domains.WithDB(tx)
		jobs := s.jobs.WithDB(tx)
		audit := s.audit.WithDB(tx)

		domain, err := domains.GetByAccountForUpdate(ctx, accountID, domainID)
		if err != nil {
			return err
		}
		if domain.Status == model.DomainDeleting || domain.Status == model.DomainDeleted {
			result = domain
			return nil
		}
		if err := domains.SetStatus(ctx, accountID, domainID, model.DomainDeleting); err != nil {
			return err
		}
		aid := accountID
		if _, err := jobs.Enqueue(ctx, &aid, model.JobDeprovisionDNS, domainJobPayload{DomainID: domainID}); err != nil {
			return err
		}
		actor := actorUserID
		if err := audit.Append(ctx, repository.Entry{
			AccountID: &aid,
			ActorID:   &actor,
			Action:    model.AuditDomainDetachQueued,
			Target:    strPtr(domainID.String()),
			Metadata:  map[string]any{"hostname": domain.Hostname, "from": domain.Status, "to": model.DomainDeleting},
		}); err != nil {
			return err
		}
		domain.Status = model.DomainDeleting
		result = domain
		return nil
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, apperr.NotFound("domain not found")
		}
		if apperr.As(err) != nil {
			return nil, err
		}
		return nil, apperr.Internal("failed to detach domain").Wrap(err)
	}
	return result, nil
}

// GetInstructions returns verification instructions for a domain.
func (s *DomainService) GetInstructions(
	ctx context.Context,
	accountID, domainID uuid.UUID,
) (provisioner.VerificationInstructions, error) {
	domain, err := s.domains.GetByAccount(ctx, accountID, domainID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return provisioner.VerificationInstructions{}, apperr.NotFound("domain not found")
		}
		return provisioner.VerificationInstructions{}, apperr.Internal("failed to load domain").Wrap(err)
	}

	token := ""
	if domain.VerificationToken != nil {
		token = *domain.VerificationToken
	}
	dnsZoneID := ""
	if domain.DNSZoneID != nil {
		dnsZoneID = *domain.DNSZoneID
	}
	spec := provisioner.DomainSpec{
		DomainID:          domain.ID,
		AccountID:         domain.AccountID,
		Hostname:          domain.Hostname,
		VerificationToken: token,
		DNSProvider:       domain.DNSProvider,
		DNSZoneID:         &dnsZoneID,
	}
	return s.dns.GenerateVerification(ctx, spec)
}