package queue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"

	"github.com/nazxf/opencloud/backend/internal/model"
	"github.com/nazxf/opencloud/backend/internal/provisioner"
	"github.com/nazxf/opencloud/backend/internal/repository"
)

// DomainProcessor executes one provider operation for a domain job and
// atomically commits its control-plane transition plus audit event.
type DomainProcessor struct {
	db      *bun.DB
	domains *repository.DomainRepo
	jobs    *repository.JobRepo
	audit   *repository.AuditRepo
	dns     provisioner.DNSProvisioner
}

// NewDomainProcessor constructs a DomainProcessor.
func NewDomainProcessor(
	db *bun.DB,
	domains *repository.DomainRepo,
	jobs *repository.JobRepo,
	audit *repository.AuditRepo,
	dns provisioner.DNSProvisioner,
) *DomainProcessor {
	return &DomainProcessor{
		db:      db,
		domains: domains,
		jobs:    jobs,
		audit:   audit,
		dns:     dns,
	}
}

type domainPayload struct {
	DomainID uuid.UUID `json:"domain_id"`
}

// Handle executes an idempotent provider call, then atomically persists its
// domain state, audit row, and successful job status.
func (p *DomainProcessor) Handle(ctx context.Context, job *model.Job, workerID string) error {
	var payload domainPayload
	if err := json.Unmarshal(job.Payload, &payload); err != nil || payload.DomainID == uuid.Nil {
		return errors.New("invalid domain job payload")
	}
	domain, err := p.domains.GetForWorker(ctx, payload.DomainID)
	if err != nil {
		return fmt.Errorf("load domain job: %w", err)
	}

	var nextStatus, auditAction string
	switch job.Kind {
	case model.JobVerifyDomain:
		verified, err := p.verifyOwnership(ctx, domain)
		if err != nil {
			return err
		}
		if verified {
			nextStatus = model.DomainVerified
			auditAction = model.AuditDomainVerified
		} else {
			// Stay in verifying; retry later via backoff.
			return errors.New("domain ownership not yet verified")
		}
	case model.JobProvisionDNS:
		if err := p.provisionDNS(ctx, domain); err != nil {
			return err
		}
		nextStatus = model.DomainActive
		auditAction = model.AuditDomainProvisioned
	case model.JobDeprovisionDNS:
		if err := p.deprovisionDNS(ctx, domain); err != nil {
			return err
		}
		nextStatus = model.DomainDeleted
		auditAction = model.AuditDomainDetached
	default:
		return fmt.Errorf("unsupported domain job kind %q", job.Kind)
	}

	return p.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		domains := p.domains.WithDB(tx)
		jobs := p.jobs.WithDB(tx)
		audit := p.audit.WithDB(tx)

		current, err := domains.GetForWorkerForUpdate(ctx, domain.ID)
		if err != nil {
			return err
		}

		if job.Kind == model.JobDeprovisionDNS {
			if err := domains.MarkDeleted(ctx, domain.ID); err != nil {
				return err
			}
		} else if job.Kind == model.JobVerifyDomain {
			now := time.Now().UTC()
			if err := domains.SetVerified(ctx, domain.ID, now); err != nil {
				return err
			}
			// Enqueue DNS provisioning after successful verification.
			aid := current.AccountID
			if _, err := jobs.Enqueue(ctx, &aid, model.JobProvisionDNS, domainPayload{DomainID: domain.ID}); err != nil {
				return err
			}
		} else if job.Kind == model.JobProvisionDNS {
			if err := domains.SetWorkerStatus(ctx, domain.ID, model.DomainActive, nil); err != nil {
				return err
			}
		}

		aid := current.AccountID
		if err := audit.Append(ctx, repository.Entry{
			AccountID: &aid,
			Action:    auditAction,
			Target:    stringPointer(domain.ID.String()),
			Metadata: map[string]any{
				"hostname":  current.Hostname,
				"job_id":    job.ID.String(),
				"job_kind":  job.Kind,
				"status":    nextStatus,
			},
		}); err != nil {
			return err
		}
		return jobs.Complete(ctx, job.ID, workerID)
	})
}

// Exhaust atomically fails a domain job, marks the resource failed, appends
// an audit, and does NOT enqueue cleanup (verification can be retried by the
// customer manually).
func (p *DomainProcessor) Exhaust(ctx context.Context, job *model.Job, workerID, safeError string) error {
	var payload domainPayload
	if err := json.Unmarshal(job.Payload, &payload); err != nil {
		return err
	}
	return p.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		domains := p.domains.WithDB(tx)
		jobs := p.jobs.WithDB(tx)
		audit := p.audit.WithDB(tx)

		domain, err := domains.GetForWorkerForUpdate(ctx, payload.DomainID)
		if err != nil {
			return err
		}
		if domain.Status != model.DomainDeleting {
			if err := domains.SetWorkerStatus(ctx, domain.ID, model.DomainFailed, &safeError); err != nil {
				return err
			}
		}
		if err := jobs.Fail(ctx, job.ID, workerID, safeError); err != nil {
			return err
		}
		aid := domain.AccountID
		return audit.Append(ctx, repository.Entry{
			AccountID: &aid,
			Action:    model.AuditDomainFailed,
			Target:    stringPointer(domain.ID.String()),
			Metadata:  map[string]any{"job_id": job.ID.String(), "job_kind": job.Kind},
		})
	})
}

func (p *DomainProcessor) verifyOwnership(ctx context.Context, domain *model.Domain) (bool, error) {
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
	return p.dns.VerifyOwnership(ctx, spec)
}

func (p *DomainProcessor) provisionDNS(ctx context.Context, domain *model.Domain) error {
	// For manual DNS, provisioning is a no-op (customer manages their DNS).
	// For Cloudflare, we would set A/CNAME records here.
	if domain.DNSProvider == model.DNSProviderManual {
		return nil
	}
	return errors.New("cloudflare dns provisioning not yet configured for this domain")
}

func (p *DomainProcessor) deprovisionDNS(ctx context.Context, domain *model.Domain) error {
	// Cleanup is currently a no-op for manual DNS.
	if domain.DNSProvider == model.DNSProviderManual {
		return nil
	}
	return errors.New("cloudflare dns deprovisioning not yet configured for this domain")
}