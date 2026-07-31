package queue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"

	"github.com/nazxf/opencloud/backend/internal/domainverify"
	"github.com/nazxf/opencloud/backend/internal/model"
	"github.com/nazxf/opencloud/backend/internal/provisioner"
	"github.com/nazxf/opencloud/backend/internal/repository"
)

const (
	domainUnlockTimeout     = 5 * time.Second
	domainReconcileInterval = 6 * time.Hour
	domainLifecycleAttempts = 12
	certificateWindow       = 30 * 24 * time.Hour
	domainClaimSafeError    = "domain verification could not be completed"
)

// DomainProcessor executes DNS/Caddy operations and commits every result with
// its audit event and job transition.
type DomainProcessor struct {
	db          *bun.DB
	domains     *repository.DomainRepo
	sites       *repository.SiteRepo
	jobs        *repository.JobRepo
	audit       *repository.AuditRepo
	dns         provisioner.DomainDNSProvisioner
	sitesPlane  provisioner.SiteProvisioner
	signer      *domainverify.Signer
	ingressIPv4 string
}

// NewDomainProcessor constructs a durable domain worker.
func NewDomainProcessor(
	db *bun.DB,
	domains *repository.DomainRepo,
	sites *repository.SiteRepo,
	jobs *repository.JobRepo,
	audit *repository.AuditRepo,
	dns provisioner.DomainDNSProvisioner,
	sitesPlane provisioner.SiteProvisioner,
	signer *domainverify.Signer,
	ingressIPv4 string,
) *DomainProcessor {
	return &DomainProcessor{
		db: db, domains: domains, sites: sites, jobs: jobs, audit: audit,
		dns: dns, sitesPlane: sitesPlane, signer: signer, ingressIPv4: ingressIPv4,
	}
}

type workerDomainPayload struct {
	DomainID uuid.UUID `json:"domain_id"`
}

// Handle executes one domain job.
func (p *DomainProcessor) Handle(ctx context.Context, job *model.Job, workerID string) error {
	domainID, err := parseDomainPayload(job)
	if err != nil {
		return err
	}
	domain, err := p.domains.GetForWorker(ctx, domainID)
	if err != nil {
		return fmt.Errorf("load domain job target: %w", err)
	}
	if err := validateDomainJobOwnership(job, domain); err != nil {
		return err
	}
	switch job.Kind {
	case model.JobVerifyDomain:
		return p.verify(ctx, job, workerID, domainID)
	case model.JobProvisionDomain:
		return p.provision(ctx, job, workerID, domainID)
	case model.JobDeprovisionDomain:
		return p.deprovision(ctx, job, workerID, domainID)
	case model.JobObserveDomainCertificate:
		return p.observeCertificate(ctx, job, workerID, domainID)
	case model.JobReconcileDomain:
		return p.reconcile(ctx, job, workerID, domainID)
	default:
		return fmt.Errorf("unsupported domain job kind %q", job.Kind)
	}
}

func (p *DomainProcessor) verify(
	ctx context.Context,
	job *model.Job,
	workerID string,
	domainID uuid.UUID,
) error {
	domain, err := p.domains.GetForWorker(ctx, domainID)
	if err != nil {
		return fmt.Errorf("load domain verification job: %w", err)
	}
	if domain.Status != model.DomainVerifying {
		return p.completeNoop(ctx, job, workerID)
	}
	now := time.Now().UTC()
	if !now.Before(domain.VerificationExpiresAt) {
		return errors.New("domain verification challenge expired")
	}
	token := p.signer.Token(
		domain.ID, domain.AccountID, domain.Hostname, domain.VerificationExpiresAt,
	)
	if !domainverify.Matches(token, domain.VerificationTokenDigest) {
		return errors.New("domain verification challenge digest mismatch")
	}
	verified, err := p.dns.VerifyOwnership(ctx, p.dnsSpec(domain, token))
	if err != nil {
		return err
	}
	if !verified {
		return provisioner.ErrDNSNotReady
	}

	return p.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		domains := p.domains.WithDB(tx)
		jobs := p.jobs.WithDB(tx)
		audit := p.audit.WithDB(tx)
		current, err := domains.GetForWorkerForUpdate(ctx, domainID)
		if err != nil {
			return err
		}
		if current.Status != model.DomainVerifying {
			return p.completeNoopWithRepos(ctx, jobs, job, workerID)
		}
		if err := validateDomainJobOwnership(job, current); err != nil {
			return err
		}
		claimed, err := domains.TryClaimHostname(ctx, current.ID, current.Hostname)
		if err != nil {
			return err
		}
		if !claimed {
			if err := domains.SetWorkerStatus(
				ctx, domainID, model.DomainFailed, stringPointer(domainClaimSafeError),
			); err != nil {
				return err
			}
			if err := p.appendAudit(ctx, audit, current, model.AuditDomainFailed, job, map[string]any{
				"error_class": "domain_hostname_claim_unavailable",
			}); err != nil {
				return err
			}
			return jobs.Complete(ctx, job.ID, workerID)
		}
		verifiedAt := time.Now().UTC()
		if err := domains.MarkVerified(ctx, domainID, verifiedAt); err != nil {
			return err
		}
		if _, err := jobs.EnqueueUniqueDomain(
			ctx, current.AccountID, model.JobProvisionDomain, domainID, 12,
		); err != nil {
			return err
		}
		if err := p.appendAudit(ctx, audit, current, model.AuditDomainVerified, job, map[string]any{
			"status": model.DomainDNSPending,
		}); err != nil {
			return err
		}
		return jobs.Complete(ctx, job.ID, workerID)
	})
}

func (p *DomainProcessor) provision(
	ctx context.Context,
	job *model.Job,
	workerID string,
	domainID uuid.UUID,
) error {
	domain, err := p.domains.GetForWorker(ctx, domainID)
	if err != nil {
		return err
	}
	return p.withSiteRoutingLock(ctx, domain.SiteID, func(conn bun.Conn) error {
		domains := p.domains.WithDB(conn)
		sites := p.sites.WithDB(conn)
		current, err := domains.GetForWorker(ctx, domainID)
		if err != nil {
			return err
		}
		if err := validateDomainJobOwnership(job, current); err != nil {
			return err
		}
		if current.Status == model.DomainDeleting || current.Status == model.DomainDeleted {
			return p.cleanupSupersededProvision(ctx, conn, job, workerID, current)
		}
		if !containsDomainStatus(
			current.Status, model.DomainDNSPending, model.DomainProvisioning, model.DomainActive,
		) {
			return fmt.Errorf("domain cannot be provisioned from status %q", current.Status)
		}
		site, err := sites.GetForWorker(ctx, current.SiteID)
		if err != nil {
			return err
		}
		if site.AccountID != current.AccountID {
			return errors.New("domain site ownership mismatch")
		}
		if site.Status != model.SiteActive {
			return p.completeNoopOnConn(ctx, conn, job, workerID)
		}
		if current.Status == model.DomainActive {
			// A provision job may survive a retry/reconcile after activation.
			// Direct ingress was already proven before activation and is not a
			// permanent requirement because customers may later enable an HTTP
			// proxy. Still converge the full exact-host route set: resume uses
			// these active-domain jobs to restore aliases.
			hostnames, err := domains.RouteHostnames(ctx, current.SiteID)
			if err != nil {
				return err
			}
			if err := p.sitesPlane.SetSiteDomains(ctx, siteRef(site), hostnames); err != nil {
				return err
			}
			return p.completeNoopOnConn(ctx, conn, job, workerID)
		}

		recordIDs, err := p.dns.EnsureRecords(ctx, p.dnsSpec(current, ""))
		if err != nil {
			return err
		}
		if err := p.persistDNSRecordIDs(ctx, conn, job, current, recordIDs); err != nil {
			cleanupErr := p.dns.DeleteRecords(ctx, p.dnsSpec(current, ""), recordIDs)
			return errors.Join(err, cleanupErr)
		}

		routed, err := p.dns.VerifyRouting(ctx, p.dnsSpec(current, ""))
		if err != nil {
			return err
		}
		if !routed {
			return provisioner.ErrDNSNotReady
		}

		hostnames, err := domains.RouteHostnames(ctx, current.SiteID)
		if err != nil {
			return err
		}
		if err := p.sitesPlane.SetSiteDomains(ctx, siteRef(site), hostnames); err != nil {
			return err
		}
		reloaded, err := domains.GetForWorker(ctx, domainID)
		if err != nil {
			return err
		}
		if err := validateDomainJobOwnership(job, reloaded); err != nil {
			return err
		}
		reloadedSite, err := sites.GetForWorker(ctx, current.SiteID)
		if err != nil {
			return err
		}
		if reloaded.Status == model.DomainDeleting || reloaded.Status == model.DomainDeleted ||
			reloadedSite.Status != model.SiteActive {
			if reloaded.Status == model.DomainDeleting || reloaded.Status == model.DomainDeleted {
				if err := p.dns.DeleteRecords(ctx, p.dnsSpec(reloaded, ""), recordIDs); err != nil {
					return err
				}
			}
			desiredHostnames, err := domains.RouteHostnames(ctx, current.SiteID)
			if err != nil {
				return err
			}
			if err := p.sitesPlane.SetSiteDomains(
				ctx, siteRef(reloadedSite), desiredHostnames,
			); err != nil {
				return err
			}
			return p.completeNoopOnConn(ctx, conn, job, workerID)
		}
		return conn.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
			domains := p.domains.WithDB(tx)
			jobs := p.jobs.WithDB(tx)
			audit := p.audit.WithDB(tx)
			current, err := domains.GetForWorkerForUpdate(ctx, domainID)
			if err != nil {
				return err
			}
			if err := validateDomainJobOwnership(job, current); err != nil {
				return err
			}
			if current.Status == model.DomainDeleting || current.Status == model.DomainDeleted {
				return p.completeNoopWithRepos(ctx, jobs, job, workerID)
			}
			if err := domains.SetWorkerStatus(ctx, domainID, model.DomainActive, nil); err != nil {
				return err
			}
			if err := domains.SetCertificate(ctx, domainID, model.CertIssuing, nil, nil, nil); err != nil {
				return err
			}
			if _, err := jobs.EnqueueUniqueDomain(
				ctx, current.AccountID, model.JobObserveDomainCertificate, domainID, 12,
			); err != nil {
				return err
			}
			if err := p.appendAudit(ctx, audit, current, model.AuditDomainProvisioned, job, map[string]any{
				"status":      model.DomainActive,
				"cert_status": model.CertIssuing,
			}); err != nil {
				return err
			}
			return jobs.Complete(ctx, job.ID, workerID)
		})
	})
}

func (p *DomainProcessor) persistDNSRecordIDs(
	ctx context.Context,
	conn bun.Conn,
	job *model.Job,
	domain *model.Domain,
	recordIDs []string,
) error {
	return conn.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		domains := p.domains.WithDB(tx)
		audit := p.audit.WithDB(tx)
		current, err := domains.GetForWorkerForUpdate(ctx, domain.ID)
		if err != nil {
			return err
		}
		if err := validateDomainJobOwnership(job, current); err != nil {
			return err
		}
		changed, err := domains.SetDNSRecordIDs(ctx, domain.ID, recordIDs)
		if err != nil || !changed {
			return err
		}
		return p.appendAudit(ctx, audit, current, model.AuditDomainDNSRecordsEnsured, job, map[string]any{
			"provider":     current.DNSProvider,
			"record_count": len(recordIDs),
		})
	})
}

func (p *DomainProcessor) cleanupSupersededProvision(
	ctx context.Context,
	conn bun.Conn,
	job *model.Job,
	workerID string,
	domain *model.Domain,
) error {
	recordIDs, err := p.dns.EnsureRecords(ctx, p.dnsSpec(domain, ""))
	if err != nil {
		return err
	}
	if err := p.dns.DeleteRecords(ctx, p.dnsSpec(domain, ""), recordIDs); err != nil {
		return err
	}
	return p.completeNoopOnConn(ctx, conn, job, workerID)
}

func (p *DomainProcessor) deprovision(
	ctx context.Context,
	job *model.Job,
	workerID string,
	domainID uuid.UUID,
) error {
	domain, err := p.domains.GetForWorker(ctx, domainID)
	if err != nil {
		return err
	}
	return p.withSiteRoutingLock(ctx, domain.SiteID, func(conn bun.Conn) error {
		domains := p.domains.WithDB(conn)
		sites := p.sites.WithDB(conn)
		current, err := domains.GetForWorker(ctx, domainID)
		if err != nil {
			return err
		}
		if err := validateDomainJobOwnership(job, current); err != nil {
			return err
		}
		if current.Status == model.DomainDeleted {
			return p.completeNoopOnConn(ctx, conn, job, workerID)
		}
		if current.Status != model.DomainDeleting {
			return p.completeNoopOnConn(ctx, conn, job, workerID)
		}
		recordIDs, err := repository.DecodeDNSRecordIDs(current.DNSRecordIDs)
		if err != nil {
			return err
		}
		if len(recordIDs) == 0 {
			recordIDs, err = p.dns.EnsureRecords(ctx, p.dnsSpec(current, ""))
			if err != nil {
				return err
			}
		}
		if err := p.dns.DeleteRecords(ctx, p.dnsSpec(current, ""), recordIDs); err != nil {
			return err
		}
		site, err := sites.GetForWorker(ctx, current.SiteID)
		if err != nil {
			return err
		}
		if site.AccountID != current.AccountID {
			return errors.New("domain site ownership mismatch")
		}
		if site.Status == model.SiteActive {
			hostnames, err := domains.RouteHostnames(ctx, current.SiteID)
			if err != nil {
				return err
			}
			if err := p.sitesPlane.SetSiteDomains(ctx, siteRef(site), hostnames); err != nil {
				return err
			}
		}
		return conn.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
			domains := p.domains.WithDB(tx)
			jobs := p.jobs.WithDB(tx)
			audit := p.audit.WithDB(tx)
			current, err := domains.GetForWorkerForUpdate(ctx, domainID)
			if err != nil {
				return err
			}
			if err := validateDomainJobOwnership(job, current); err != nil {
				return err
			}
			if err := domains.MarkDeleted(ctx, domainID); err != nil {
				return err
			}
			if err := p.appendAudit(ctx, audit, current, model.AuditDomainDetached, job, map[string]any{
				"status": model.DomainDeleted,
			}); err != nil {
				return err
			}
			return jobs.Complete(ctx, job.ID, workerID)
		})
	})
}

func (p *DomainProcessor) observeCertificate(
	ctx context.Context,
	job *model.Job,
	workerID string,
	domainID uuid.UUID,
) error {
	domain, err := p.domains.GetForWorker(ctx, domainID)
	if err != nil {
		return err
	}
	if domain.Status != model.DomainActive {
		return p.completeNoop(ctx, job, workerID)
	}
	observation, err := p.sitesPlane.CertificateStatus(ctx, domain.Hostname, p.ingressIPv4)
	if err != nil {
		return err
	}
	status := model.CertActive
	if observation.ExpiresAt.Before(time.Now().UTC().Add(certificateWindow)) {
		status = model.CertExpiring
	}
	return p.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		domains := p.domains.WithDB(tx)
		jobs := p.jobs.WithDB(tx)
		audit := p.audit.WithDB(tx)
		current, err := domains.GetForWorkerForUpdate(ctx, domainID)
		if err != nil {
			return err
		}
		if current.Status != model.DomainActive {
			return p.completeNoopWithRepos(ctx, jobs, job, workerID)
		}
		if err := validateDomainJobOwnership(job, current); err != nil {
			return err
		}
		expiresAt := observation.ExpiresAt.UTC()
		observedAt := time.Now().UTC()
		if current.CertStatus == status &&
			current.CertExpiresAt != nil &&
			current.CertExpiresAt.Equal(expiresAt) &&
			current.LastError == nil {
			if err := domains.TouchCertificateObservation(ctx, domainID, observedAt); err != nil {
				return err
			}
			return jobs.Complete(ctx, job.ID, workerID)
		}
		if err := domains.SetCertificate(ctx, domainID, status, &expiresAt, nil, &observedAt); err != nil {
			return err
		}
		if err := p.appendAudit(ctx, audit, current, model.AuditDomainCertificateObserved, job, map[string]any{
			"cert_status": status,
			"expires_at":  expiresAt.Format(time.RFC3339),
		}); err != nil {
			return err
		}
		return jobs.Complete(ctx, job.ID, workerID)
	})
}

func (p *DomainProcessor) reconcile(
	ctx context.Context,
	job *model.Job,
	workerID string,
	domainID uuid.UUID,
) error {
	return p.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		domains := p.domains.WithDB(tx)
		jobs := p.jobs.WithDB(tx)
		audit := p.audit.WithDB(tx)
		domain, err := domains.GetForWorkerForUpdate(ctx, domainID)
		if err != nil {
			return err
		}
		if err := validateDomainJobOwnership(job, domain); err != nil {
			return err
		}
		var next []string
		switch domain.Status {
		case model.DomainVerifying:
			next = []string{model.JobVerifyDomain}
		case model.DomainDNSPending, model.DomainProvisioning:
			next = []string{model.JobProvisionDomain}
		case model.DomainActive:
			// Direct A-record validation is an activation gate, not a permanent
			// requirement: customers may enable an HTTP proxy afterwards.
			// Site reconciliation restores exact routes; active-domain
			// reconciliation only refreshes public certificate evidence.
			next = []string{model.JobObserveDomainCertificate}
		case model.DomainDeleting:
			next = []string{model.JobDeprovisionDomain}
		}
		var enqueued []string
		for _, kind := range next {
			inserted, err := jobs.EnqueueUniqueDomain(ctx, domain.AccountID, kind, domainID, 12)
			if err != nil {
				return err
			}
			if inserted {
				enqueued = append(enqueued, kind)
			}
		}
		if len(enqueued) == 0 {
			return jobs.Complete(ctx, job.ID, workerID)
		}
		reconciledAt := time.Now().UTC()
		if err := domains.MarkReconciled(ctx, domainID, reconciledAt); err != nil {
			return err
		}
		if err := p.appendAudit(ctx, audit, domain, model.AuditDomainReconciled, job, map[string]any{
			"enqueued_kinds": enqueued,
		}); err != nil {
			return err
		}
		return jobs.Complete(ctx, job.ID, workerID)
	})
}

// Exhaust records a terminal retry failure without claiming provider success.
func (p *DomainProcessor) Exhaust(
	ctx context.Context,
	job *model.Job,
	workerID, safeError string,
) error {
	domainID, err := parseDomainPayload(job)
	if err != nil {
		return err
	}
	domain, err := p.domains.GetForWorker(ctx, domainID)
	if err != nil {
		return err
	}
	if err := validateDomainJobOwnership(job, domain); err != nil {
		return p.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
			return p.jobs.WithDB(tx).Fail(ctx, job.ID, workerID, safeError)
		})
	}
	return p.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		domains := p.domains.WithDB(tx)
		jobs := p.jobs.WithDB(tx)
		audit := p.audit.WithDB(tx)
		domain, err := domains.GetForWorkerForUpdate(ctx, domainID)
		if err != nil {
			return err
		}
		if err := validateDomainJobOwnership(job, domain); err != nil {
			return err
		}
		if err := jobs.Fail(ctx, job.ID, workerID, safeError); err != nil {
			return err
		}
		switch job.Kind {
		case model.JobObserveDomainCertificate:
			observedAt := time.Now().UTC()
			if err := domains.SetCertificate(ctx, domainID, model.CertError, nil, &safeError, &observedAt); err != nil {
				return err
			}
		case model.JobDeprovisionDomain, model.JobReconcileDomain:
			// Preserve desired state; periodic reconciliation may enqueue recovery.
		default:
			if domain.Status != model.DomainDeleting && domain.Status != model.DomainDeleted {
				if err := domains.SetWorkerStatus(ctx, domainID, model.DomainFailed, &safeError); err != nil {
					return err
				}
				if job.Kind == model.JobProvisionDomain {
					if _, err := jobs.EnqueueUniqueSite(
						ctx, domain.AccountID, model.JobReconcileSite, domain.SiteID,
					); err != nil {
						return err
					}
				}
			}
		}
		return p.appendAudit(ctx, audit, domain, model.AuditDomainFailed, job, map[string]any{
			"error_class": "domain_provider_operation_failed",
		})
	})
}

func (p *DomainProcessor) withSiteRoutingLock(
	ctx context.Context,
	siteID uuid.UUID,
	operation func(bun.Conn) error,
) (err error) {
	conn, err := p.db.Conn(ctx)
	if err != nil {
		return err
	}
	domains := p.domains.WithDB(conn)
	if err := domains.LockSiteRouting(ctx, siteID); err != nil {
		return errors.Join(err, conn.Close())
	}
	defer func() {
		unlockContext, cancel := context.WithTimeout(context.WithoutCancel(ctx), domainUnlockTimeout)
		defer cancel()
		unlocked, unlockErr := domains.UnlockSiteRouting(unlockContext, siteID)
		if unlockErr != nil {
			err = errors.Join(err, fmt.Errorf("unlock domain routing: %w", unlockErr))
		} else if !unlocked {
			err = errors.Join(err, errors.New("domain routing lock was not held"))
		}
		err = errors.Join(err, conn.Close())
	}()
	return operation(conn)
}

func (p *DomainProcessor) completeNoop(
	ctx context.Context,
	job *model.Job,
	workerID string,
) error {
	return p.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		return p.completeNoopWithRepos(ctx, p.jobs.WithDB(tx), job, workerID)
	})
}

func (p *DomainProcessor) completeNoopOnConn(
	ctx context.Context,
	conn bun.Conn,
	job *model.Job,
	workerID string,
) error {
	return conn.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		return p.completeNoopWithRepos(ctx, p.jobs.WithDB(tx), job, workerID)
	})
}

func (p *DomainProcessor) completeNoopWithRepos(
	ctx context.Context,
	jobs *repository.JobRepo,
	job *model.Job,
	workerID string,
) error {
	return jobs.Complete(ctx, job.ID, workerID)
}

func (p *DomainProcessor) appendAudit(
	ctx context.Context,
	audit *repository.AuditRepo,
	domain *model.Domain,
	action string,
	job *model.Job,
	metadata map[string]any,
) error {
	metadata["hostname"] = domain.Hostname
	metadata["job_id"] = job.ID.String()
	metadata["job_kind"] = job.Kind
	aid := domain.AccountID
	return audit.Append(ctx, repository.Entry{
		AccountID: &aid,
		Action:    action,
		Target:    stringPointer(domain.ID.String()),
		Metadata:  metadata,
	})
}

func (p *DomainProcessor) dnsSpec(
	domain *model.Domain,
	token string,
) provisioner.DomainDNSSpec {
	zoneID := ""
	if domain.DNSZoneID != nil {
		zoneID = *domain.DNSZoneID
	}
	return provisioner.DomainDNSSpec{
		DomainID: domain.ID, AccountID: domain.AccountID,
		Hostname: domain.Hostname, VerificationToken: token,
		IngressIPv4: p.ingressIPv4, Provider: domain.DNSProvider, ZoneID: zoneID,
	}
}

func parseDomainPayload(job *model.Job) (uuid.UUID, error) {
	var payload workerDomainPayload
	if err := json.Unmarshal(job.Payload, &payload); err != nil || payload.DomainID == uuid.Nil {
		return uuid.Nil, errors.New("invalid domain job payload")
	}
	return payload.DomainID, nil
}

func validateDomainJobOwnership(job *model.Job, domain *model.Domain) error {
	if job.AccountID == nil || *job.AccountID != domain.AccountID {
		return errors.New("domain job ownership mismatch")
	}
	return nil
}

func siteRef(site *model.Site) provisioner.SiteRef {
	return provisioner.SiteRef{
		SiteID: site.ID, AccountID: site.AccountID, NodeID: site.NodeID,
	}
}

func containsDomainStatus(current string, allowed ...string) bool {
	for _, status := range allowed {
		if current == status {
			return true
		}
	}
	return false
}
