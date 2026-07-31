package service_test

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/nazxf/opencloud/backend/internal/apperr"
	"github.com/nazxf/opencloud/backend/internal/domainverify"
	"github.com/nazxf/opencloud/backend/internal/handler"
	"github.com/nazxf/opencloud/backend/internal/model"
	"github.com/nazxf/opencloud/backend/internal/provisioner"
	"github.com/nazxf/opencloud/backend/internal/queue"
	"github.com/nazxf/opencloud/backend/internal/repository"
	"github.com/nazxf/opencloud/backend/internal/service"
)

type phase3Fixture struct {
	*phase2Fixture
	domainRepo *repository.DomainRepo
	dns        *provisioner.FakeDomainDNS
	sitePlane  *provisioner.Fake
	signer     *domainverify.Signer
	processor  *queue.DomainProcessor
	site       *model.Site
	domain     *model.Domain
}

type phase3DeleteFailureProvisioner struct {
	provisioner.SiteProvisioner
}

func (phase3DeleteFailureProvisioner) DeleteSite(context.Context, provisioner.SiteRef) error {
	return errors.New("forced site delete failure")
}

type phase3SetDomainsHookProvisioner struct {
	provisioner.SiteProvisioner
	once sync.Once
	hook func()
}

type phase3FixedCertificateProvisioner struct {
	provisioner.SiteProvisioner
	expiresAt time.Time
}

func (p phase3FixedCertificateProvisioner) CertificateStatus(
	context.Context,
	string,
	string,
) (provisioner.CertificateObservation, error) {
	return provisioner.CertificateObservation{ExpiresAt: p.expiresAt}, nil
}

func (p *phase3SetDomainsHookProvisioner) SetSiteDomains(
	ctx context.Context,
	ref provisioner.SiteRef,
	hostnames []string,
) error {
	if err := p.SiteProvisioner.SetSiteDomains(ctx, ref, hostnames); err != nil {
		return err
	}
	p.once.Do(p.hook)
	return nil
}

func newPhase3Fixture(t *testing.T, siteStatus, domainStatus string) *phase3Fixture {
	t.Helper()
	base := newPhase2Fixture(t, 5)
	ctx := context.Background()

	var domainTableCount int
	err := base.db.NewRaw(`
		SELECT count(*)
		FROM information_schema.tables
		WHERE table_schema = 'public' AND table_name = 'domains'
	`).Scan(ctx, &domainTableCount)
	require.NoError(t, err)
	if domainTableCount != 1 {
		t.Skip("Phase 3 domains table missing; run migrations first")
	}

	site := &model.Site{
		ID:           uuid.New(),
		AccountID:    base.account.ID,
		NodeID:       base.nodes[0].ID,
		Domain:       "primary-" + uuid.NewString() + ".example.com",
		Image:        "opencloud/site-static:phase2",
		InternalPort: 8080,
		MemoryBytes:  256 * 1024 * 1024,
		NanoCPUs:     500_000_000,
		Status:       siteStatus,
	}
	require.NoError(t, base.sites.Create(ctx, site))

	sitePlane := provisioner.NewFake()
	require.NoError(t, sitePlane.CreateSite(ctx, provisioner.SiteSpec{
		SiteID: site.ID, AccountID: site.AccountID, NodeID: site.NodeID,
		Domain: site.Domain, Image: site.Image, InternalPort: uint16(site.InternalPort),
		MemoryBytes: site.MemoryBytes, NanoCPUs: site.NanoCPUs,
	}))
	if siteStatus == model.SiteSuspended {
		require.NoError(t, sitePlane.SuspendSite(ctx, provisioner.SiteRef{
			SiteID: site.ID, AccountID: site.AccountID, NodeID: site.NodeID,
		}))
	}

	key := base64.StdEncoding.EncodeToString([]byte("01234567890123456789012345678901"))
	signer, err := domainverify.New(key)
	require.NoError(t, err)
	domainID := uuid.New()
	expiresAt, digest, err := signer.Issue(
		time.Now().UTC(), time.Hour, domainID, base.account.ID,
		"custom-"+uuid.NewString()+".example.com",
	)
	require.NoError(t, err)
	domain := &model.Domain{
		ID: domainID, AccountID: base.account.ID, SiteID: site.ID,
		Hostname: "custom-" + uuid.NewString() + ".example.com",
		Status:   domainStatus, VerificationTokenDigest: digest,
		VerificationExpiresAt: expiresAt, DNSProvider: model.DNSProviderManual,
		CertStatus: model.CertNone, CertAutoRenew: true,
	}
	// The digest is bound to the hostname, so issue once more after assigning it.
	expiresAt, digest, err = signer.Issue(
		time.Now().UTC(), time.Hour, domain.ID, domain.AccountID, domain.Hostname,
	)
	require.NoError(t, err)
	domain.VerificationExpiresAt = expiresAt
	domain.VerificationTokenDigest = digest
	if domainStatus == model.DomainActive || domainStatus == model.DomainDNSPending ||
		domainStatus == model.DomainProvisioning {
		verifiedAt := time.Now().UTC().Add(-time.Minute)
		domain.VerifiedAt = &verifiedAt
		domain.VerificationConsumedAt = &verifiedAt
	}
	domainRepo := repository.NewDomainRepo(base.db)
	require.NoError(t, domainRepo.Create(ctx, domain))

	dns := provisioner.NewFakeDomainDNS()
	processor := queue.NewDomainProcessor(
		base.db, domainRepo, base.sites, base.jobs, base.audit,
		dns, sitePlane, signer, "203.0.113.10",
	)
	return &phase3Fixture{
		phase2Fixture: base, domainRepo: domainRepo, dns: dns,
		sitePlane: sitePlane, signer: signer, processor: processor,
		site: site, domain: domain,
	}
}

func (f *phase3Fixture) claimDomainJob(
	t *testing.T,
	accountID uuid.UUID,
	kind string,
) (*model.Job, string) {
	t.Helper()
	ctx := context.Background()
	_, err := f.jobs.EnqueueWithMaxAttempts(
		ctx, &accountID, kind, map[string]uuid.UUID{"domain_id": f.domain.ID}, 3,
	)
	require.NoError(t, err)
	workerID := "phase3-worker-" + uuid.NewString()
	job, err := f.jobs.Claim(ctx, workerID)
	require.NoError(t, err)
	require.Equal(t, kind, job.Kind)
	return job, workerID
}

func TestPhase3DomainJobRejectsMismatchedTenantBeforeProviderSideEffects(t *testing.T) {
	fx := newPhase3Fixture(t, model.SiteActive, model.DomainDNSPending)
	ctx := context.Background()
	otherAccount := &model.Account{
		ID: uuid.New(), Name: "Other tenant", Status: model.AccountActive,
		CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}
	_, err := fx.db.NewInsert().Model(otherAccount).Exec(ctx)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = fx.db.NewDelete().Model((*model.Job)(nil)).Where("account_id = ?", otherAccount.ID).Exec(ctx)
		_, _ = fx.db.NewDelete().Model((*model.Account)(nil)).Where("id = ?", otherAccount.ID).Exec(ctx)
	})

	job, workerID := fx.claimDomainJob(t, otherAccount.ID, model.JobProvisionDomain)
	err = fx.processor.Handle(ctx, job, workerID)
	require.ErrorContains(t, err, "ownership mismatch")
	require.Equal(t, provisioner.DomainDNSCallCounts{}, fx.dns.CallCounts())
	require.Empty(t, fx.dns.RecordIDs(fx.domain.ID))
	require.Equal(t, []string{fx.site.Domain}, fx.sitePlane.SiteDomains(fx.site.ID))

	require.NoError(t, fx.processor.Exhaust(ctx, job, workerID, "provisioner operation failed"))
	reloaded, err := fx.domainRepo.GetForWorker(ctx, fx.domain.ID)
	require.NoError(t, err)
	require.Equal(t, model.DomainDNSPending, reloaded.Status)
	var status string
	require.NoError(t, fx.db.NewRaw(`SELECT status FROM jobs WHERE id = ?`, job.ID).Scan(ctx, &status))
	require.Equal(t, model.JobFailed, status)
}

func TestPhase3SuspendedSiteDeniesTLSAndCannotRegainDomainRouting(t *testing.T) {
	fx := newPhase3Fixture(t, model.SiteSuspended, model.DomainActive)
	ctx := context.Background()
	job, workerID := fx.claimDomainJob(t, fx.account.ID, model.JobProvisionDomain)

	require.NoError(t, fx.processor.Handle(ctx, job, workerID))
	require.Equal(t, provisioner.DomainDNSCallCounts{}, fx.dns.CallCounts())
	require.Empty(t, fx.dns.RecordIDs(fx.domain.ID), "inactive site must stop before DNS/provider work")
	require.Empty(t, fx.sitePlane.SiteDomains(fx.site.ID), "suspended site route must remain absent")
	authorized, err := fx.domainRepo.HostnameAuthorizedForTLS(ctx, fx.domain.Hostname, "sites.example.com")
	require.NoError(t, err)
	require.False(t, authorized)
	hostnames, err := fx.domainRepo.RouteHostnames(ctx, fx.site.ID)
	require.NoError(t, err)
	require.Empty(t, hostnames)
	var status string
	require.NoError(t, fx.db.NewRaw(`SELECT status FROM jobs WHERE id = ?`, job.ID).Scan(ctx, &status))
	require.Equal(t, model.JobSucceeded, status)
}

func TestPhase3TLSPrimaryHostnameMustUsePlatformSuffix(t *testing.T) {
	fx := newPhase3Fixture(t, model.SiteActive, model.DomainActive)
	ctx := context.Background()

	authorized, err := fx.domainRepo.HostnameAuthorizedForTLS(
		ctx,
		fx.site.Domain,
		"sites.example.com",
	)
	require.NoError(t, err)
	require.False(t, authorized,
		"an arbitrary customer FQDN stored as a primary site must not authorize TLS")

	authorized, err = fx.domainRepo.HostnameAuthorizedForTLS(
		ctx,
		fx.site.Domain,
		"example.com",
	)
	require.NoError(t, err)
	require.True(t, authorized, "strict children of the configured platform suffix remain valid")

	authorized, err = fx.domainRepo.HostnameAuthorizedForTLS(
		ctx,
		fx.domain.Hostname,
		"sites.example.com",
	)
	require.NoError(t, err)
	require.True(t, authorized,
		"verified custom-domain authorization is independent of the platform suffix")
}

func TestPhase3ActiveDomainProvisionNoopsAfterCustomerEnablesProxy(t *testing.T) {
	fx := newPhase3Fixture(t, model.SiteActive, model.DomainActive)
	ctx := context.Background()
	expiresAt := time.Now().UTC().Add(60 * 24 * time.Hour).Truncate(time.Microsecond)
	observedAt := time.Now().UTC().Add(-time.Hour).Truncate(time.Microsecond)
	_, err := fx.db.NewUpdate().Model((*model.Domain)(nil)).
		Set("cert_status = ?", model.CertActive).
		Set("cert_expires_at = ?", expiresAt).
		Set("cert_observed_at = ?", observedAt).
		Where("id = ?", fx.domain.ID).
		Exec(ctx)
	require.NoError(t, err)
	fx.dns.SetRouting(fx.domain.Hostname, false)
	require.NoError(t, fx.sitePlane.SetSiteDomains(
		ctx,
		provisioner.SiteRef{
			SiteID: fx.site.ID, AccountID: fx.site.AccountID, NodeID: fx.site.NodeID,
		},
		[]string{fx.site.Domain},
	))

	for attempt := 0; attempt < 2; attempt++ {
		job, workerID := fx.claimDomainJob(t, fx.account.ID, model.JobProvisionDomain)
		require.NoError(t, fx.processor.Handle(ctx, job, workerID))
	}

	reloaded, err := fx.domainRepo.GetForWorker(ctx, fx.domain.ID)
	require.NoError(t, err)
	require.Equal(t, model.DomainActive, reloaded.Status)
	require.Equal(t, model.CertActive, reloaded.CertStatus)
	require.WithinDuration(t, expiresAt, *reloaded.CertExpiresAt, time.Microsecond)
	require.WithinDuration(t, observedAt, *reloaded.CertObservedAt, time.Microsecond)
	require.ElementsMatch(t, []string{fx.site.Domain, fx.domain.Hostname}, fx.sitePlane.SiteDomains(fx.site.ID))
	require.Equal(t, provisioner.DomainDNSCallCounts{}, fx.dns.CallCounts(),
		"direct routing must not be re-required after initial activation")
	require.ElementsMatch(t, []string{fx.site.Domain, fx.domain.Hostname},
		fx.sitePlane.SiteDomains(fx.site.ID),
		"active jobs still restore exact aliases after resume or route drift")
	var provisionedAudits int
	require.NoError(t, fx.db.NewRaw(`
		SELECT count(*) FROM audit_logs
		WHERE target = ? AND action = ?
	`, fx.domain.ID.String(), model.AuditDomainProvisioned).Scan(ctx, &provisionedAudits))
	require.Zero(t, provisionedAudits, "active reconciliation must not replay activation audit/state")
	var dnsAudits int
	require.NoError(t, fx.db.NewRaw(`
		SELECT count(*) FROM audit_logs
		WHERE target = ? AND action = ?
	`, fx.domain.ID.String(), model.AuditDomainDNSRecordsEnsured).Scan(ctx, &dnsAudits))
	require.Zero(t, dnsAudits, "active no-op jobs must not churn provider audits")
}

func TestPhase3DNSRecordAuditFailureCompensatesProviderState(t *testing.T) {
	fx := newPhase3Fixture(t, model.SiteActive, model.DomainDNSPending)
	ctx := context.Background()
	fx.dns.SetRouting(fx.domain.Hostname, true)
	_, err := fx.db.ExecContext(ctx, `
		CREATE OR REPLACE FUNCTION fail_phase3_dns_record_audit() RETURNS trigger AS $$
		BEGIN
			IF NEW.action = 'domain.dns_records_ensured' THEN
				RAISE EXCEPTION 'forced phase3 DNS record audit failure';
			END IF;
			RETURN NEW;
		END;
		$$ LANGUAGE plpgsql;
		CREATE TRIGGER fail_phase3_dns_record_audit_trigger
		BEFORE INSERT ON audit_logs
		FOR EACH ROW EXECUTE FUNCTION fail_phase3_dns_record_audit();
	`)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = fx.db.ExecContext(ctx, `DROP TRIGGER IF EXISTS fail_phase3_dns_record_audit_trigger ON audit_logs`)
		_, _ = fx.db.ExecContext(ctx, `DROP FUNCTION IF EXISTS fail_phase3_dns_record_audit()`)
	})

	job, workerID := fx.claimDomainJob(t, fx.account.ID, model.JobProvisionDomain)
	err = fx.processor.Handle(ctx, job, workerID)
	require.Error(t, err)
	require.Equal(t, provisioner.DomainDNSCallCounts{
		EnsureRecords: 1,
		DeleteRecords: 1,
	}, fx.dns.CallCounts())
	require.Empty(t, fx.dns.RecordIDs(fx.domain.ID), "failed durable audit must compensate provider records")
	reloaded, err := fx.domainRepo.GetForWorker(ctx, fx.domain.ID)
	require.NoError(t, err)
	require.Equal(t, model.DomainDNSPending, reloaded.Status)
	require.JSONEq(t, `[]`, string(reloaded.DNSRecordIDs))
	require.Equal(t, []string{fx.site.Domain}, fx.sitePlane.SiteDomains(fx.site.ID))
}

func TestPhase3DetachDuringProvisionPreservesPrimaryAndOtherAliases(t *testing.T) {
	fx := newPhase3Fixture(t, model.SiteActive, model.DomainDNSPending)
	ctx := context.Background()
	otherID := uuid.New()
	otherHostname := "other-alias-" + uuid.NewString() + ".example.com"
	otherExpiry, otherDigest, err := fx.signer.Issue(
		time.Now().UTC(), time.Hour, otherID, fx.account.ID, otherHostname,
	)
	require.NoError(t, err)
	otherVerifiedAt := time.Now().UTC().Add(-time.Minute)
	other := &model.Domain{
		ID: otherID, AccountID: fx.account.ID, SiteID: fx.site.ID,
		Hostname: otherHostname, Status: model.DomainActive,
		VerificationTokenDigest: otherDigest,
		VerificationExpiresAt:   otherExpiry,
		VerificationConsumedAt:  &otherVerifiedAt,
		VerifiedAt:              &otherVerifiedAt,
		DNSProvider:             model.DNSProviderManual,
		CertStatus:              model.CertNone, CertAutoRenew: true,
	}
	require.NoError(t, fx.domainRepo.Create(ctx, other))
	fx.dns.SetRouting(fx.domain.Hostname, true)

	hooked := &phase3SetDomainsHookProvisioner{
		SiteProvisioner: fx.sitePlane,
		hook: func() {
			_, hookErr := fx.db.NewUpdate().Model((*model.Domain)(nil)).
				Set("status = ?", model.DomainDeleting).
				Where("id = ?", fx.domain.ID).
				Exec(ctx)
			require.NoError(t, hookErr)
		},
	}
	processor := queue.NewDomainProcessor(
		fx.db, fx.domainRepo, fx.sites, fx.jobs, fx.audit,
		fx.dns, hooked, fx.signer, "203.0.113.10",
	)
	job, workerID := fx.claimDomainJob(t, fx.account.ID, model.JobProvisionDomain)
	require.NoError(t, processor.Handle(ctx, job, workerID))

	require.ElementsMatch(
		t,
		[]string{fx.site.Domain, other.Hostname},
		fx.sitePlane.SiteDomains(fx.site.ID),
		"superseded provisioning must remove only the detaching alias",
	)
	reloaded, err := fx.domainRepo.GetForWorker(ctx, fx.domain.ID)
	require.NoError(t, err)
	require.Equal(t, model.DomainDeleting, reloaded.Status)
}

func TestPhase3SiteResumeQueuesOnlyVerifiedDomainRestorationAfterActiveCommit(t *testing.T) {
	fx := newPhase3Fixture(t, model.SiteSuspended, model.DomainActive)
	ctx := context.Background()
	createDomain := func(status string, verified bool) *model.Domain {
		t.Helper()
		domain := &model.Domain{
			ID: uuid.New(), AccountID: fx.account.ID, SiteID: fx.site.ID,
			Hostname: "resume-" + uuid.NewString() + ".example.com",
			Status:   status, VerificationExpiresAt: time.Now().UTC().Add(time.Hour),
			VerificationTokenDigest: make([]byte, 32), DNSProvider: model.DNSProviderManual,
			CertStatus: model.CertNone, CertAutoRenew: true,
		}
		if verified {
			verifiedAt := time.Now().UTC().Add(-time.Minute)
			domain.VerifiedAt = &verifiedAt
			domain.VerificationConsumedAt = &verifiedAt
		}
		require.NoError(t, fx.domainRepo.Create(ctx, domain))
		return domain
	}
	pending := createDomain(model.DomainPending, false)
	deleting := createDomain(model.DomainDeleting, true)

	_, err := fx.db.NewUpdate().Model((*model.Site)(nil)).
		Set("status = ?", model.SiteResuming).
		Where("id = ?", fx.site.ID).
		Exec(ctx)
	require.NoError(t, err)
	_, err = fx.jobs.Enqueue(
		ctx, &fx.account.ID, model.JobResumeSite, map[string]uuid.UUID{"site_id": fx.site.ID},
	)
	require.NoError(t, err)
	workerID := "phase3-resume-" + uuid.NewString()
	resumeJob, err := fx.jobs.Claim(ctx, workerID)
	require.NoError(t, err)

	processor := queue.NewProcessor(
		fx.db, fx.sites, fx.domainRepo, fx.nodeRepo, fx.jobs, fx.audit, fx.sitePlane,
		repository.NewManagedDatabaseRepo(fx.db), nil, nil,
	)
	processor.SetDomainProcessor(fx.processor)
	require.NoError(t, processor.Handle(ctx, resumeJob, workerID))
	require.Equal(t, []string{fx.site.Domain}, fx.sitePlane.SiteDomains(fx.site.ID),
		"custom routing must wait until the site-active transaction commits")

	var restoredIDs []uuid.UUID
	require.NoError(t, fx.db.NewRaw(`
		SELECT (payload ->> 'domain_id')::uuid
		FROM jobs
		WHERE account_id = ? AND kind = ? AND status = ?
		ORDER BY created_at
	`, fx.account.ID, model.JobProvisionDomain, model.JobQueued).Scan(ctx, &restoredIDs))
	require.Equal(t, []uuid.UUID{fx.domain.ID}, restoredIDs)
	require.NotContains(t, restoredIDs, pending.ID)
	require.NotContains(t, restoredIDs, deleting.ID)

	fx.dns.SetRouting(fx.domain.Hostname, true)
	restoreWorkerID := "phase3-domain-restore-" + uuid.NewString()
	restoreJob, err := fx.jobs.Claim(ctx, restoreWorkerID)
	require.NoError(t, err)
	require.NoError(t, processor.Handle(ctx, restoreJob, restoreWorkerID))
	require.ElementsMatch(t, []string{fx.site.Domain, fx.domain.Hostname}, fx.sitePlane.SiteDomains(fx.site.ID))
}

func TestPhase3SiteLifecycleFailsClosedWithoutDomainProcessor(t *testing.T) {
	t.Run("resume preserves a routable domain and suspended runtime", func(t *testing.T) {
		fx := newPhase3Fixture(t, model.SiteSuspended, model.DomainActive)
		ctx := context.Background()
		_, err := fx.db.NewUpdate().Model((*model.Site)(nil)).
			Set("status = ?", model.SiteResuming).
			Where("id = ?", fx.site.ID).
			Exec(ctx)
		require.NoError(t, err)
		_, err = fx.jobs.Enqueue(
			ctx, &fx.account.ID, model.JobResumeSite, map[string]uuid.UUID{"site_id": fx.site.ID},
		)
		require.NoError(t, err)
		workerID := "phase3-resume-disabled-" + uuid.NewString()
		job, err := fx.jobs.Claim(ctx, workerID)
		require.NoError(t, err)

		processor := queue.NewProcessor(
			fx.db, fx.sites, fx.domainRepo, fx.nodeRepo, fx.jobs, fx.audit, fx.sitePlane,
			repository.NewManagedDatabaseRepo(fx.db), nil, nil,
		)
		err = processor.Handle(ctx, job, workerID)
		require.ErrorContains(t, err, "domain processor is not configured for site resume")

		observed, err := fx.sitePlane.SiteStatus(ctx, provisioner.SiteRef{
			SiteID: fx.site.ID, AccountID: fx.site.AccountID, NodeID: fx.site.NodeID,
		})
		require.NoError(t, err)
		require.Equal(t, provisioner.SiteStateSuspended, observed,
			"resume must stop before any provider side effect")
		require.Empty(t, fx.sitePlane.SiteDomains(fx.site.ID))
		reloadedSite, err := fx.sites.GetForWorker(ctx, fx.site.ID)
		require.NoError(t, err)
		require.Equal(t, model.SiteResuming, reloadedSite.Status)
		reloadedDomain, err := fx.domainRepo.GetForWorker(ctx, fx.domain.ID)
		require.NoError(t, err)
		require.Equal(t, model.DomainActive, reloadedDomain.Status)
		var provisionJobs int
		require.NoError(t, fx.db.NewRaw(`
			SELECT count(*) FROM jobs
			WHERE kind = ? AND payload ->> 'domain_id' = ?
		`, model.JobProvisionDomain, fx.domain.ID.String()).Scan(ctx, &provisionJobs))
		require.Zero(t, provisionJobs)
	})

	t.Run("delete preserves live domain claim and running runtime", func(t *testing.T) {
		fx := newPhase3Fixture(t, model.SiteActive, model.DomainActive)
		ctx := context.Background()
		_, err := fx.db.NewUpdate().Model((*model.Site)(nil)).
			Set("status = ?", model.SiteDeleting).
			Where("id = ?", fx.site.ID).
			Exec(ctx)
		require.NoError(t, err)
		_, err = fx.jobs.Enqueue(
			ctx, &fx.account.ID, model.JobDeleteSite, map[string]uuid.UUID{"site_id": fx.site.ID},
		)
		require.NoError(t, err)
		workerID := "phase3-delete-disabled-" + uuid.NewString()
		job, err := fx.jobs.Claim(ctx, workerID)
		require.NoError(t, err)

		processor := queue.NewProcessor(
			fx.db, fx.sites, fx.domainRepo, fx.nodeRepo, fx.jobs, fx.audit, fx.sitePlane,
			repository.NewManagedDatabaseRepo(fx.db), nil, nil,
		)
		err = processor.Handle(ctx, job, workerID)
		require.ErrorContains(t, err, "domain processor is not configured for site cleanup")

		observed, err := fx.sitePlane.SiteStatus(ctx, provisioner.SiteRef{
			SiteID: fx.site.ID, AccountID: fx.site.AccountID, NodeID: fx.site.NodeID,
		})
		require.NoError(t, err)
		require.Equal(t, provisioner.SiteStateRunning, observed,
			"delete must stop before any provider side effect")
		require.Equal(t, []string{fx.site.Domain}, fx.sitePlane.SiteDomains(fx.site.ID))
		reloadedSite, err := fx.sites.GetForWorker(ctx, fx.site.ID)
		require.NoError(t, err)
		require.Equal(t, model.SiteDeleting, reloadedSite.Status)
		reloadedDomain, err := fx.domainRepo.GetForWorker(ctx, fx.domain.ID)
		require.NoError(t, err)
		require.Equal(t, model.DomainActive, reloadedDomain.Status)
		var claims, deprovisionJobs int
		require.NoError(t, fx.db.NewRaw(
			`SELECT count(*) FROM hostname_claims WHERE hostname = ?`, fx.domain.Hostname,
		).Scan(ctx, &claims))
		require.Equal(t, 1, claims)
		require.NoError(t, fx.db.NewRaw(`
			SELECT count(*) FROM jobs
			WHERE kind = ? AND payload ->> 'domain_id' = ?
		`, model.JobDeprovisionDomain, fx.domain.ID.String()).Scan(ctx, &deprovisionJobs))
		require.Zero(t, deprovisionJobs)
	})
}

func TestPhase3SiteReconcileIsDomainAwareBeforeProviderMutation(t *testing.T) {
	t.Run("active custom domain blocks missing and suspended repair without processor", func(t *testing.T) {
		for _, observedState := range []provisioner.SiteState{
			provisioner.SiteStateMissing,
			provisioner.SiteStateSuspended,
		} {
			t.Run(string(observedState), func(t *testing.T) {
				fx := newPhase3Fixture(t, model.SiteActive, model.DomainActive)
				ctx := context.Background()
				ref := provisioner.SiteRef{
					SiteID: fx.site.ID, AccountID: fx.site.AccountID, NodeID: fx.site.NodeID,
				}
				if observedState == provisioner.SiteStateMissing {
					require.NoError(t, fx.sitePlane.DeleteSite(ctx, ref))
				} else {
					require.NoError(t, fx.sitePlane.SuspendSite(ctx, ref))
				}
				_, err := fx.jobs.EnqueueWithMaxAttempts(
					ctx, &fx.account.ID, model.JobReconcileSite,
					map[string]uuid.UUID{"site_id": fx.site.ID}, 1,
				)
				require.NoError(t, err)
				workerID := "phase3-reconcile-disabled-" + uuid.NewString()
				job, err := fx.jobs.Claim(ctx, workerID)
				require.NoError(t, err)

				processor := queue.NewProcessor(
					fx.db, fx.sites, fx.domainRepo, fx.nodeRepo, fx.jobs, fx.audit,
					fx.sitePlane, repository.NewManagedDatabaseRepo(fx.db), nil, nil,
				)
				require.NoError(t, processor.Handle(ctx, job, workerID),
					"periodic reconcile must defer without retry churn")

				observed, err := fx.sitePlane.SiteStatus(ctx, ref)
				require.NoError(t, err)
				require.Equal(t, observedState, observed,
					"reconcile must not create or resume a primary-only runtime")
				reloadedSite, err := fx.sites.GetForWorker(ctx, fx.site.ID)
				require.NoError(t, err)
				require.Equal(t, model.SiteActive, reloadedSite.Status,
					"terminal reconcile failure must preserve desired site state")
				require.NotNil(t, reloadedSite.LastReconciledAt,
					"deferred work must advance the fair reconciliation cursor")
				reloadedDomain, err := fx.domainRepo.GetForWorker(ctx, fx.domain.ID)
				require.NoError(t, err)
				require.Equal(t, model.DomainActive, reloadedDomain.Status)
				var status string
				require.NoError(t, fx.db.NewRaw(
					`SELECT status FROM jobs WHERE id = ?`, job.ID,
				).Scan(ctx, &status))
				require.Equal(t, model.JobSucceeded, status)
				var deferredAudits int
				require.NoError(t, fx.db.NewRaw(`
					SELECT count(*) FROM audit_logs
					WHERE target = ? AND action = ?
					  AND metadata ->> 'deferred_reason' = 'domain_processor_unavailable'
				`, fx.site.ID.String(), model.AuditSiteReconcileDeferred).Scan(ctx, &deferredAudits))
				require.Equal(t, 1, deferredAudits)
			})
		}
	})

	t.Run("deleting reconcile durably queues cleanup before a failed delete", func(t *testing.T) {
		fx := newPhase3Fixture(t, model.SiteActive, model.DomainActive)
		ctx := context.Background()
		_, err := fx.db.NewUpdate().Model((*model.Site)(nil)).
			Set("status = ?", model.SiteDeleting).
			Where("id = ?", fx.site.ID).
			Exec(ctx)
		require.NoError(t, err)
		_, err = fx.jobs.EnqueueWithMaxAttempts(
			ctx, &fx.account.ID, model.JobReconcileSite,
			map[string]uuid.UUID{"site_id": fx.site.ID}, 3,
		)
		require.NoError(t, err)
		workerID := "phase3-delete-reconcile-" + uuid.NewString()
		job, err := fx.jobs.Claim(ctx, workerID)
		require.NoError(t, err)

		processor := queue.NewProcessor(
			fx.db, fx.sites, fx.domainRepo, fx.nodeRepo, fx.jobs, fx.audit,
			phase3DeleteFailureProvisioner{SiteProvisioner: fx.sitePlane},
			repository.NewManagedDatabaseRepo(fx.db), nil, nil,
		)
		processor.SetDomainProcessor(fx.processor)
		err = processor.Handle(ctx, job, workerID)
		require.ErrorContains(t, err, "forced site delete failure")

		reloadedDomain, err := fx.domainRepo.GetForWorker(ctx, fx.domain.ID)
		require.NoError(t, err)
		require.Equal(t, model.DomainDeleting, reloadedDomain.Status)
		var deprovisionJobs, detachAudits int
		require.NoError(t, fx.db.NewRaw(`
			SELECT count(*) FROM jobs
			WHERE kind = ? AND status = ? AND payload ->> 'domain_id' = ?
		`, model.JobDeprovisionDomain, model.JobQueued, fx.domain.ID.String()).Scan(ctx, &deprovisionJobs))
		require.Equal(t, 1, deprovisionJobs)
		require.NoError(t, fx.db.NewRaw(`
			SELECT count(*) FROM audit_logs WHERE target = ? AND action = ?
		`, fx.domain.ID.String(), model.AuditDomainDetachQueued).Scan(ctx, &detachAudits))
		require.Equal(t, 1, detachAudits)
		observed, err := fx.sitePlane.SiteStatus(ctx, provisioner.SiteRef{
			SiteID: fx.site.ID, AccountID: fx.site.AccountID, NodeID: fx.site.NodeID,
		})
		require.NoError(t, err)
		require.Equal(t, provisioner.SiteStateRunning, observed)
	})

	t.Run("deleting reconcile blocks a live domain before delete when processor is absent", func(t *testing.T) {
		fx := newPhase3Fixture(t, model.SiteActive, model.DomainActive)
		ctx := context.Background()
		_, err := fx.db.NewUpdate().Model((*model.Site)(nil)).
			Set("status = ?", model.SiteDeleting).
			Where("id = ?", fx.site.ID).
			Exec(ctx)
		require.NoError(t, err)
		_, err = fx.jobs.EnqueueWithMaxAttempts(
			ctx, &fx.account.ID, model.JobReconcileSite,
			map[string]uuid.UUID{"site_id": fx.site.ID}, 1,
		)
		require.NoError(t, err)
		workerID := "phase3-delete-reconcile-disabled-" + uuid.NewString()
		job, err := fx.jobs.Claim(ctx, workerID)
		require.NoError(t, err)
		processor := queue.NewProcessor(
			fx.db, fx.sites, fx.domainRepo, fx.nodeRepo, fx.jobs, fx.audit,
			fx.sitePlane, repository.NewManagedDatabaseRepo(fx.db), nil, nil,
		)
		err = processor.Handle(ctx, job, workerID)
		require.ErrorContains(t, err, "domain processor is not configured for site cleanup")

		observed, err := fx.sitePlane.SiteStatus(ctx, provisioner.SiteRef{
			SiteID: fx.site.ID, AccountID: fx.site.AccountID, NodeID: fx.site.NodeID,
		})
		require.NoError(t, err)
		require.Equal(t, provisioner.SiteStateRunning, observed)
		reloadedDomain, err := fx.domainRepo.GetForWorker(ctx, fx.domain.ID)
		require.NoError(t, err)
		require.Equal(t, model.DomainActive, reloadedDomain.Status)
		var deprovisionJobs int
		require.NoError(t, fx.db.NewRaw(`
			SELECT count(*) FROM jobs WHERE kind = ? AND payload ->> 'domain_id' = ?
		`, model.JobDeprovisionDomain, fx.domain.ID.String()).Scan(ctx, &deprovisionJobs))
		require.Zero(t, deprovisionJobs)
	})

	t.Run("zero-domain active reconcile still recreates a missing runtime", func(t *testing.T) {
		fx := newPhase2Fixture(t, 2)
		ctx := context.Background()
		site := &model.Site{
			ID: uuid.New(), AccountID: fx.account.ID, NodeID: fx.nodes[0].ID,
			Domain: "phase2-reconcile-" + uuid.NewString() + ".example.com",
			Image:  "opencloud/site-static:phase2", InternalPort: 8080,
			MemoryBytes: 256 * 1024 * 1024, NanoCPUs: 500_000_000,
			Status: model.SiteActive,
		}
		require.NoError(t, fx.sites.Create(ctx, site))
		_, err := fx.jobs.EnqueueWithMaxAttempts(
			ctx, &fx.account.ID, model.JobReconcileSite,
			map[string]uuid.UUID{"site_id": site.ID}, 3,
		)
		require.NoError(t, err)
		workerID := "phase2-zero-domain-reconcile-" + uuid.NewString()
		job, err := fx.jobs.Claim(ctx, workerID)
		require.NoError(t, err)
		sitePlane := provisioner.NewFake()
		processor := queue.NewProcessor(
			fx.db, fx.sites, repository.NewDomainRepo(fx.db), fx.nodeRepo, fx.jobs, fx.audit,
			sitePlane, repository.NewManagedDatabaseRepo(fx.db), nil, nil,
		)
		require.NoError(t, processor.Handle(ctx, job, workerID))
		observed, err := sitePlane.SiteStatus(ctx, provisioner.SiteRef{
			SiteID: site.ID, AccountID: site.AccountID, NodeID: site.NodeID,
		})
		require.NoError(t, err)
		require.Equal(t, provisioner.SiteStateRunning, observed)
		require.Equal(t, []string{site.Domain}, sitePlane.SiteDomains(site.ID))
		reloaded, err := fx.sites.GetForWorker(ctx, site.ID)
		require.NoError(t, err)
		require.NotNil(t, reloaded.LastReconciledAt)
	})
}

func TestPhase3SiteReconciliationCursorIsFairBoundedAndLifecycleAware(t *testing.T) {
	fx := newPhase3Fixture(t, model.SiteActive, model.DomainPending)
	ctx := context.Background()
	second := &model.Site{
		ID: uuid.New(), AccountID: fx.account.ID, NodeID: fx.nodes[0].ID,
		Domain: "phase3-reconcile-second-" + uuid.NewString() + ".example.com",
		Image:  "opencloud/site-static:phase2", InternalPort: 8080,
		MemoryBytes: 256 * 1024 * 1024, NanoCPUs: 500_000_000,
		Status: model.SiteActive,
	}
	require.NoError(t, fx.sites.Create(ctx, second))
	cutoff := time.Now().UTC().Add(-6 * time.Hour)

	firstBatch, err := fx.sites.ListReconciliationCandidates(ctx, 1, cutoff)
	require.NoError(t, err)
	require.Len(t, firstBatch, 1, "the repository must honor its bounded scan")
	firstID := firstBatch[0].ID
	require.NoError(t, fx.sites.MarkReconciled(ctx, firstID, time.Now().UTC()))

	secondBatch, err := fx.sites.ListReconciliationCandidates(ctx, 1, cutoff)
	require.NoError(t, err)
	require.Len(t, secondBatch, 1)
	require.NotEqual(t, firstID, secondBatch[0].ID,
		"advancing the cursor must prevent the oldest site from starving later rows")
	secondID := secondBatch[0].ID

	activeJob, err := fx.jobs.EnqueueWithMaxAttempts(
		ctx, &fx.account.ID, model.JobReconcileSite,
		map[string]uuid.UUID{"site_id": secondID}, 3,
	)
	require.NoError(t, err)
	candidates, err := fx.sites.ListReconciliationCandidates(ctx, 100, cutoff)
	require.NoError(t, err)
	for _, candidate := range candidates {
		require.NotEqual(t, secondID, candidate.ID,
			"queued site work must suppress duplicate reconciliation for that site")
	}
	_, err = fx.db.NewDelete().Model((*model.Job)(nil)).Where("id = ?", activeJob.ID).Exec(ctx)
	require.NoError(t, err)

	require.NoError(t, fx.sites.MarkReconciled(ctx, secondID, time.Now().UTC()))
	_, err = fx.db.NewUpdate().Model((*model.Site)(nil)).
		Set("status = ?", model.SiteDeleting).
		Where("id = ?", secondID).
		Exec(ctx)
	require.NoError(t, err)
	candidates, err = fx.sites.ListReconciliationCandidates(ctx, 100, cutoff)
	require.NoError(t, err)
	var foundDeleting bool
	for _, candidate := range candidates {
		if candidate.ID == secondID {
			foundDeleting = true
			require.Equal(t, model.SiteDeleting, candidate.Status)
		}
	}
	require.True(t, foundDeleting,
		"deleting lifecycle work must bypass the steady-state throttle")
}

func TestPhase3FailingDeletesCannotStarveSteadySiteReconciliation(t *testing.T) {
	fx := newPhase3Fixture(t, model.SiteActive, model.DomainActive)
	ctx := context.Background()
	for index := range 101 {
		require.NoError(t, fx.sites.Create(ctx, &model.Site{
			ID:           uuid.New(),
			AccountID:    fx.account.ID,
			NodeID:       fx.nodes[0].ID,
			Domain:       fmt.Sprintf("stuck-delete-%03d-%s.example.com", index, uuid.NewString()),
			Image:        "opencloud/site-static:phase2",
			InternalPort: 8080,
			MemoryBytes:  256 * 1024 * 1024,
			NanoCPUs:     500_000_000,
			Status:       model.SiteDeleting,
		}))
	}

	candidates, err := fx.sites.ListReconciliationCandidates(
		ctx,
		100,
		time.Now().UTC().Add(-6*time.Hour),
	)
	require.NoError(t, err)
	var ownCandidates int
	var foundActive bool
	var deletingCount int
	for _, candidate := range candidates {
		if candidate.AccountID != fx.account.ID {
			continue
		}
		ownCandidates++
		if candidate.ID == fx.site.ID {
			foundActive = true
		}
		if candidate.Status == model.SiteDeleting {
			deletingCount++
		}
	}
	require.LessOrEqual(t, len(candidates), 100)
	require.GreaterOrEqual(t, ownCandidates, 1)
	require.True(t, foundActive, "the batch must reserve progress for overdue active sites")
	require.Greater(t, deletingCount, 0)
	require.LessOrEqual(t, deletingCount, 50)
}

func TestPhase3SiteDeleteRetainsDomainClaimUntilDurableCleanupCompletes(t *testing.T) {
	fx := newPhase3Fixture(t, model.SiteActive, model.DomainActive)
	ctx := context.Background()
	_, err := fx.db.NewUpdate().Model((*model.Node)(nil)).
		Set("used_sites = 1").
		Where("id = ?", fx.site.NodeID).
		Exec(ctx)
	require.NoError(t, err)
	_, err = fx.db.NewUpdate().Model((*model.Site)(nil)).
		Set("status = ?", model.SiteDeleting).
		Where("id = ?", fx.site.ID).
		Exec(ctx)
	require.NoError(t, err)
	_, err = fx.jobs.Enqueue(
		ctx, &fx.account.ID, model.JobDeleteSite, map[string]uuid.UUID{"site_id": fx.site.ID},
	)
	require.NoError(t, err)

	failingProcessor := queue.NewProcessor(
		fx.db, fx.sites, fx.domainRepo, fx.nodeRepo, fx.jobs, fx.audit,
		phase3DeleteFailureProvisioner{SiteProvisioner: fx.sitePlane},
		repository.NewManagedDatabaseRepo(fx.db), nil, nil,
	)
	failingProcessor.SetDomainProcessor(fx.processor)
	workerID := "phase3-site-delete-" + uuid.NewString()
	siteJob, err := fx.jobs.Claim(ctx, workerID)
	require.NoError(t, err)
	require.Equal(t, model.JobDeleteSite, siteJob.Kind)
	err = failingProcessor.Handle(ctx, siteJob, workerID)
	require.ErrorContains(t, err, "forced site delete failure")
	reloadedDomain, err := fx.domainRepo.GetForWorker(ctx, fx.domain.ID)
	require.NoError(t, err)
	require.Equal(t, model.DomainDeleting, reloadedDomain.Status)
	var queuedCleanup int
	require.NoError(t, fx.db.NewRaw(`
		SELECT count(*) FROM jobs
		WHERE account_id = ? AND kind = ? AND status = ?
		  AND payload ->> 'domain_id' = ?
	`, fx.account.ID, model.JobDeprovisionDomain, model.JobQueued, fx.domain.ID.String()).Scan(ctx, &queuedCleanup))
	require.Equal(t, 1, queuedCleanup, "domain cleanup must commit before site runtime deletion")
	observed, err := fx.sitePlane.SiteStatus(ctx, provisioner.SiteRef{
		SiteID: fx.site.ID, AccountID: fx.site.AccountID, NodeID: fx.site.NodeID,
	})
	require.NoError(t, err)
	require.Equal(t, provisioner.SiteStateRunning, observed,
		"failed provider deletion must leave the site runtime while cleanup remains recoverable")

	processor := queue.NewProcessor(
		fx.db, fx.sites, fx.domainRepo, fx.nodeRepo, fx.jobs, fx.audit, fx.sitePlane,
		repository.NewManagedDatabaseRepo(fx.db), nil, nil,
	)
	processor.SetDomainProcessor(fx.processor)
	require.NoError(t, processor.Handle(ctx, siteJob, workerID))

	reloadedSite, err := fx.sites.GetForWorker(ctx, fx.site.ID)
	require.NoError(t, err)
	require.Equal(t, model.SiteDeleted, reloadedSite.Status)
	reloadedDomain, err = fx.domainRepo.GetForWorker(ctx, fx.domain.ID)
	require.NoError(t, err)
	require.Equal(t, model.DomainDeleting, reloadedDomain.Status)
	var primaryClaims, customClaims int
	require.NoError(t, fx.db.NewRaw(
		`SELECT count(*) FROM hostname_claims WHERE hostname = ?`, fx.site.Domain,
	).Scan(ctx, &primaryClaims))
	require.NoError(t, fx.db.NewRaw(
		`SELECT count(*) FROM hostname_claims WHERE hostname = ?`, fx.domain.Hostname,
	).Scan(ctx, &customClaims))
	require.Zero(t, primaryClaims, "site tombstone releases its primary hostname claim")
	require.Equal(t, 1, customClaims, "custom claim stays reserved until DNS cleanup succeeds")

	domainWorkerID := "phase3-domain-delete-" + uuid.NewString()
	domainJob, err := fx.jobs.Claim(ctx, domainWorkerID)
	require.NoError(t, err)
	require.Equal(t, model.JobDeprovisionDomain, domainJob.Kind)
	require.NoError(t, processor.Handle(ctx, domainJob, domainWorkerID))
	reloadedDomain, err = fx.domainRepo.GetForWorker(ctx, fx.domain.ID)
	require.NoError(t, err)
	require.Equal(t, model.DomainDeleted, reloadedDomain.Status)
	require.NoError(t, fx.db.NewRaw(
		`SELECT count(*) FROM hostname_claims WHERE hostname = ?`, fx.domain.Hostname,
	).Scan(ctx, &customClaims))
	require.Zero(t, customClaims, "domain tombstone releases its claim after provider cleanup")
	var queuedAudits, completedAudits int
	require.NoError(t, fx.db.NewRaw(`
		SELECT count(*) FROM audit_logs WHERE target = ? AND action = ?
	`, fx.domain.ID.String(), model.AuditDomainDetachQueued).Scan(ctx, &queuedAudits))
	require.NoError(t, fx.db.NewRaw(`
		SELECT count(*) FROM audit_logs WHERE target = ? AND action = ?
	`, fx.domain.ID.String(), model.AuditDomainDetached).Scan(ctx, &completedAudits))
	require.Equal(t, 1, queuedAudits)
	require.Equal(t, 1, completedAudits)
}

func TestPhase3ReconciliationIsBoundedAndSkipsActiveDomainWork(t *testing.T) {
	fx := newPhase3Fixture(t, model.SiteActive, model.DomainActive)
	ctx := context.Background()
	cutoff := time.Now().UTC().Add(-6 * time.Hour)

	candidates, err := fx.domainRepo.ListReconciliationCandidates(ctx, 100, cutoff)
	require.NoError(t, err)
	require.Len(t, candidates, 1)
	require.Equal(t, fx.domain.ID, candidates[0].ID)

	activeJob, err := fx.jobs.EnqueueWithMaxAttempts(
		ctx, &fx.account.ID, model.JobProvisionDomain,
		map[string]uuid.UUID{"domain_id": fx.domain.ID}, 3,
	)
	require.NoError(t, err)
	candidates, err = fx.domainRepo.ListReconciliationCandidates(ctx, 100, cutoff)
	require.NoError(t, err)
	require.Empty(t, candidates, "an active domain job must suppress reconciliation fan-out")
	_, err = fx.db.NewDelete().Model((*model.Job)(nil)).Where("id = ?", activeJob.ID).Exec(ctx)
	require.NoError(t, err)

	reconcileJob, workerID := fx.claimDomainJob(t, fx.account.ID, model.JobReconcileDomain)
	require.NoError(t, fx.processor.Handle(ctx, reconcileJob, workerID))
	require.Equal(t, provisioner.DomainDNSCallCounts{}, fx.dns.CallCounts(),
		"active reconciliation must not re-require direct A routing after a customer enables an HTTP proxy")
	reloaded, err := fx.domainRepo.GetForWorker(ctx, fx.domain.ID)
	require.NoError(t, err)
	require.NotNil(t, reloaded.LastReconciledAt)
	var reconcileAudits int
	require.NoError(t, fx.db.NewRaw(`
		SELECT count(*) FROM audit_logs
		WHERE target = ? AND action = ?
	`, fx.domain.ID.String(), model.AuditDomainReconciled).Scan(ctx, &reconcileAudits))
	require.Equal(t, 1, reconcileAudits)
	var provisionJobs, certificateJobs int
	require.NoError(t, fx.db.NewRaw(`
		SELECT count(*) FROM jobs
		WHERE kind = ? AND payload ->> 'domain_id' = ?
	`, model.JobProvisionDomain, fx.domain.ID.String()).Scan(ctx, &provisionJobs))
	require.NoError(t, fx.db.NewRaw(`
		SELECT count(*) FROM jobs
		WHERE kind = ? AND payload ->> 'domain_id' = ?
	`, model.JobObserveDomainCertificate, fx.domain.ID.String()).Scan(ctx, &certificateJobs))
	require.Zero(t, provisionJobs)
	require.Equal(t, 1, certificateJobs)

	_, err = fx.db.NewDelete().Model((*model.Job)(nil)).
		Where("account_id = ?", fx.account.ID).
		Where("status = ?", model.JobQueued).
		Exec(ctx)
	require.NoError(t, err)
	candidates, err = fx.domainRepo.ListReconciliationCandidates(ctx, 100, cutoff)
	require.NoError(t, err)
	require.Empty(t, candidates, "recent active reconciliation must be throttled")
}

func TestPhase3UnchangedCertificateObservationDoesNotChurnStateOrAudit(t *testing.T) {
	fx := newPhase3Fixture(t, model.SiteActive, model.DomainActive)
	ctx := context.Background()
	expiresAt := time.Now().UTC().Add(90 * 24 * time.Hour).Truncate(time.Microsecond)
	observedAt := time.Now().UTC().Add(-time.Hour).Truncate(time.Microsecond)
	updatedAt := time.Now().UTC().Add(-30 * time.Minute).Truncate(time.Microsecond)
	_, err := fx.db.NewUpdate().Model((*model.Domain)(nil)).
		Set("cert_status = ?", model.CertActive).
		Set("cert_expires_at = ?", expiresAt).
		Set("cert_observed_at = ?", observedAt).
		Set("last_error = NULL").
		Set("updated_at = ?", updatedAt).
		Where("id = ?", fx.domain.ID).
		Exec(ctx)
	require.NoError(t, err)

	processor := queue.NewDomainProcessor(
		fx.db,
		fx.domainRepo,
		fx.sites,
		fx.jobs,
		fx.audit,
		fx.dns,
		phase3FixedCertificateProvisioner{
			SiteProvisioner: fx.sitePlane,
			expiresAt:       expiresAt,
		},
		fx.signer,
		"203.0.113.10",
	)
	job, workerID := fx.claimDomainJob(
		t,
		fx.account.ID,
		model.JobObserveDomainCertificate,
	)
	require.NoError(t, processor.Handle(ctx, job, workerID))

	reloaded, err := fx.domainRepo.GetForWorker(ctx, fx.domain.ID)
	require.NoError(t, err)
	require.WithinDuration(t, updatedAt, reloaded.UpdatedAt, time.Microsecond)
	require.WithinDuration(t, observedAt, *reloaded.CertObservedAt, time.Microsecond)
	var audits int
	require.NoError(t, fx.db.NewRaw(`
		SELECT count(*) FROM audit_logs
		WHERE target = ? AND action = ?
	`, fx.domain.ID.String(), model.AuditDomainCertificateObserved).Scan(ctx, &audits))
	require.Zero(t, audits)
	var status string
	require.NoError(t, fx.db.NewRaw(
		`SELECT status FROM jobs WHERE id = ?`, job.ID,
	).Scan(ctx, &status))
	require.Equal(t, model.JobSucceeded, status)
}

func TestPhase3InstructionResponseIsNeverCacheable(t *testing.T) {
	fx := newPhase3Fixture(t, model.SiteActive, model.DomainPending)
	domainService := service.NewDomainService(
		fx.db,
		fx.domainRepo,
		fx.sites,
		fx.jobs,
		fx.audit,
		fx.dns,
		fx.signer,
		"8.8.8.8",
		true,
	)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET(
		"/domains/:id/instructions",
		func(c *gin.Context) {
			c.Set("account_id", fx.account.ID)
			c.Next()
		},
		handler.NewDomainHandler(domainService).Instructions,
	)
	request := httptest.NewRequest(
		http.MethodGet,
		"/domains/"+fx.domain.ID.String()+"/instructions",
		nil,
	)
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	require.Equal(t, http.StatusOK, response.Code)
	require.Equal(t, "private, no-store", response.Header().Get("Cache-Control"))
	require.Equal(t, "no-cache", response.Header().Get("Pragma"))
	require.NotContains(t, response.Header().Get("Cache-Control"), "public")
}

func TestPhase3RepeatedVerifyQueuesAndAuditsOnce(t *testing.T) {
	fx := newPhase3Fixture(t, model.SiteActive, model.DomainPending)
	ctx := context.Background()
	svc := service.NewDomainService(
		fx.db, fx.domainRepo, fx.sites, fx.jobs, fx.audit,
		fx.dns, fx.signer, "8.8.8.8", true,
	)

	for attempt := 0; attempt < 2; attempt++ {
		domain, err := svc.Verify(
			ctx, "phase3-actor", fx.account.ID, fx.domain.ID,
		)
		require.NoError(t, err)
		require.Equal(t, model.DomainVerifying, domain.Status)
	}
	var jobCount, maxAttempts int
	require.NoError(t, fx.db.NewRaw(`
		SELECT count(*), max(max_attempts)
		FROM jobs
		WHERE account_id = ? AND kind = ? AND payload ->> 'domain_id' = ?
	`, fx.account.ID, model.JobVerifyDomain, fx.domain.ID.String()).Scan(ctx, &jobCount, &maxAttempts))
	require.Equal(t, 1, jobCount)
	require.Equal(t, 20, maxAttempts, "retry budget should cover the one-hour challenge window")
	var auditCount int
	require.NoError(t, fx.db.NewRaw(`
		SELECT count(*) FROM audit_logs
		WHERE target = ? AND action = ?
	`, fx.domain.ID.String(), model.AuditDomainVerificationQueued).Scan(ctx, &auditCount))
	require.Equal(t, 1, auditCount)
}

func TestPhase3ExpiredOwnershipChallengeStopsBeforeDNS(t *testing.T) {
	fx := newPhase3Fixture(t, model.SiteActive, model.DomainPending)
	ctx := context.Background()
	createdAt := time.Now().UTC().Add(-2 * time.Hour).Truncate(time.Microsecond)
	expiresAt := createdAt.Add(time.Hour)
	token := fx.signer.Token(fx.domain.ID, fx.domain.AccountID, fx.domain.Hostname, expiresAt)
	_, err := fx.db.NewUpdate().Model((*model.Domain)(nil)).
		Set("status = ?", model.DomainVerifying).
		Set("created_at = ?", createdAt).
		Set("verification_expires_at = ?", expiresAt).
		Set("verification_token_digest = ?", domainverify.Digest(token)).
		Where("id = ?", fx.domain.ID).
		Exec(ctx)
	require.NoError(t, err)
	fx.dns.SetOwnership(fx.domain.Hostname, token)
	job, workerID := fx.claimDomainJob(t, fx.account.ID, model.JobVerifyDomain)

	err = fx.processor.Handle(ctx, job, workerID)
	require.ErrorContains(t, err, "domain verification challenge expired")
	require.Zero(t, fx.dns.CallCounts().VerifyOwnership,
		"expired challenges must stop before the DNS provider")
	reloaded, err := fx.domainRepo.GetForWorker(ctx, fx.domain.ID)
	require.NoError(t, err)
	require.Equal(t, model.DomainVerifying, reloaded.Status)
	require.Nil(t, reloaded.VerificationConsumedAt)
	var provisionJobs, verifiedAudits int
	require.NoError(t, fx.db.NewRaw(`
		SELECT count(*) FROM jobs
		WHERE kind = ? AND payload ->> 'domain_id' = ?
	`, model.JobProvisionDomain, fx.domain.ID.String()).Scan(ctx, &provisionJobs))
	require.Zero(t, provisionJobs)
	require.NoError(t, fx.db.NewRaw(`
		SELECT count(*) FROM audit_logs WHERE target = ? AND action = ?
	`, fx.domain.ID.String(), model.AuditDomainVerified).Scan(ctx, &verifiedAudits))
	require.Zero(t, verifiedAudits)
}

func TestPhase3ChallengeRotationReExpiresAndInvalidatesOldToken(t *testing.T) {
	fx := newPhase3Fixture(t, model.SiteActive, model.DomainPending)
	ctx := context.Background()
	oldCreatedAt := time.Now().UTC().Add(-2 * time.Hour).Truncate(time.Microsecond)
	oldExpiry := oldCreatedAt.Add(time.Hour)
	oldToken := fx.signer.Token(fx.domain.ID, fx.domain.AccountID, fx.domain.Hostname, oldExpiry)
	oldDigest := domainverify.Digest(oldToken)
	_, err := fx.db.NewUpdate().Model((*model.Domain)(nil)).
		Set("created_at = ?", oldCreatedAt).
		Set("verification_expires_at = ?", oldExpiry).
		Set("verification_token_digest = ?", oldDigest).
		Where("id = ?", fx.domain.ID).
		Exec(ctx)
	require.NoError(t, err)
	svc := service.NewDomainService(
		fx.db, fx.domainRepo, fx.sites, fx.jobs, fx.audit,
		fx.dns, fx.signer, "8.8.8.8", true,
	)

	rotated, err := svc.RotateChallenge(
		ctx, "phase3-actor", fx.account.ID, fx.domain.ID,
	)
	require.NoError(t, err)
	require.Equal(t, model.DomainPending, rotated.Status)
	require.True(t, rotated.VerificationExpiresAt.After(time.Now().UTC()))
	require.NotEqual(t, oldExpiry, rotated.VerificationExpiresAt)
	require.NotEqual(t, oldDigest, rotated.VerificationTokenDigest)
	require.False(t, domainverify.Matches(oldToken, rotated.VerificationTokenDigest),
		"rotation must invalidate the old ownership value")
	newToken := fx.signer.Token(
		rotated.ID, rotated.AccountID, rotated.Hostname, rotated.VerificationExpiresAt,
	)
	require.True(t, domainverify.Matches(newToken, rotated.VerificationTokenDigest))

	instructions, err := svc.Instructions(ctx, fx.account.ID, fx.domain.ID)
	require.NoError(t, err)
	require.True(t, rotated.VerificationExpiresAt.Equal(instructions.VerificationExpiresAt),
		"instruction expiry must represent the same instant regardless of driver time zone")
	require.Equal(t, []provisioner.DNSRecord{
		{
			Type: "TXT", Name: "_opencloud-verification." + fx.domain.Hostname,
			Content: newToken, TTL: 300,
		},
	}, instructions.Records, "pending instructions must expose TXT ownership proof only")
	require.False(t, domainverify.Matches(oldToken, domainverify.Digest(instructions.Records[0].Content)))
}

func TestPhase3ConcurrentVerificationConsumesChallengeOnce(t *testing.T) {
	fx := newPhase3Fixture(t, model.SiteActive, model.DomainPending)
	ctx := context.Background()
	_, err := fx.db.NewUpdate().Model((*model.Domain)(nil)).
		Set("status = ?", model.DomainVerifying).
		Where("id = ?", fx.domain.ID).
		Exec(ctx)
	require.NoError(t, err)
	token := fx.signer.Token(
		fx.domain.ID, fx.domain.AccountID, fx.domain.Hostname, fx.domain.VerificationExpiresAt,
	)
	fx.dns.SetOwnership(fx.domain.Hostname, token)
	job, workerID := fx.claimDomainJob(t, fx.account.ID, model.JobVerifyDomain)

	start := make(chan struct{})
	results := make(chan error, 2)
	var workers sync.WaitGroup
	for range 2 {
		workers.Add(1)
		go func() {
			defer workers.Done()
			<-start
			results <- fx.processor.Handle(ctx, job, workerID)
		}()
	}
	close(start)
	workers.Wait()
	close(results)
	var successes, failures int
	for result := range results {
		if result == nil {
			successes++
		} else {
			failures++
		}
	}
	require.Equal(t, 1, successes)
	require.Equal(t, 1, failures,
		"the duplicate worker cannot consume or complete the same leased job twice")

	reloaded, err := fx.domainRepo.GetForWorker(ctx, fx.domain.ID)
	require.NoError(t, err)
	require.Equal(t, model.DomainDNSPending, reloaded.Status)
	require.NotNil(t, reloaded.VerificationConsumedAt)
	consumedAt := *reloaded.VerificationConsumedAt
	var provisionJobs, verifiedAudits int
	require.NoError(t, fx.db.NewRaw(`
		SELECT count(*) FROM jobs
		WHERE kind = ? AND payload ->> 'domain_id' = ?
	`, model.JobProvisionDomain, fx.domain.ID.String()).Scan(ctx, &provisionJobs))
	require.Equal(t, 1, provisionJobs)
	require.NoError(t, fx.db.NewRaw(`
		SELECT count(*) FROM audit_logs WHERE target = ? AND action = ?
	`, fx.domain.ID.String(), model.AuditDomainVerified).Scan(ctx, &verifiedAudits))
	require.Equal(t, 1, verifiedAudits)

	err = fx.processor.Handle(ctx, job, workerID)
	require.Error(t, err, "a completed lease cannot be replayed")
	reloaded, err = fx.domainRepo.GetForWorker(ctx, fx.domain.ID)
	require.NoError(t, err)
	require.Equal(t, consumedAt, *reloaded.VerificationConsumedAt)
	require.NoError(t, fx.db.NewRaw(`
		SELECT count(*) FROM audit_logs WHERE target = ? AND action = ?
	`, fx.domain.ID.String(), model.AuditDomainVerified).Scan(ctx, &verifiedAudits))
	require.Equal(t, 1, verifiedAudits)
}

func TestPhase3AttachRejectsOnlySameAccountHostnameIntent(t *testing.T) {
	fx := newPhase3Fixture(t, model.SiteActive, model.DomainPending)
	ctx := context.Background()
	svc := service.NewDomainService(
		fx.db, fx.domainRepo, fx.sites, fx.jobs, fx.audit,
		fx.dns, fx.signer, "8.8.8.8", true,
	)
	assertUnavailable := func(hostname string) {
		t.Helper()
		_, err := svc.Attach(
			ctx, "phase3-actor", fx.account.ID, fx.site.ID, uuid.NewString(),
			service.AttachDomainRequest{Hostname: hostname},
		)
		require.Error(t, err)
		appError := apperr.As(err)
		require.NotNil(t, appError)
		require.Equal(t, "CONFLICT", appError.Code)
		require.Equal(t, "hostname is unavailable", appError.Message)
	}

	assertUnavailable(fx.site.Domain)
	assertUnavailable(fx.domain.Hostname)
	otherAccount := &model.Account{
		ID: uuid.New(), Name: "Other hostname tenant", Status: model.AccountActive,
		CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}
	_, err := fx.db.NewInsert().Model(otherAccount).Exec(ctx)
	require.NoError(t, err)
	otherSite := &model.Site{
		ID: uuid.New(), AccountID: otherAccount.ID, NodeID: fx.nodes[0].ID,
		Domain: "other-primary-" + uuid.NewString() + ".example.com",
		Image:  "opencloud/site-static:phase2", InternalPort: 8080,
		MemoryBytes: 256 * 1024 * 1024, NanoCPUs: 500_000_000,
		Status: model.SiteActive,
	}
	require.NoError(t, fx.sites.Create(ctx, otherSite))
	t.Cleanup(func() {
		_, _ = fx.db.NewDelete().Model((*model.Job)(nil)).Where("account_id = ?", otherAccount.ID).Exec(ctx)
		_, _ = fx.db.NewDelete().Model((*model.Site)(nil)).Where("id = ?", otherSite.ID).Exec(ctx)
		_, _ = fx.db.NewDelete().Model((*model.Account)(nil)).Where("id = ?", otherAccount.ID).Exec(ctx)
	})

	pending, err := svc.Attach(
		ctx, "phase3-actor", fx.account.ID, fx.site.ID, uuid.NewString(),
		service.AttachDomainRequest{Hostname: otherSite.Domain},
	)
	require.NoError(t, err, "another tenant's primary hostname must not be enumerable before proof")
	require.Equal(t, model.DomainPending, pending.Status)
	var primarySiteID *uuid.UUID
	var primaryDomainID *uuid.UUID
	require.NoError(t, fx.db.NewRaw(`
		SELECT site_id, domain_id FROM hostname_claims WHERE hostname = ?
	`, otherSite.Domain).Scan(ctx, &primarySiteID, &primaryDomainID))
	require.NotNil(t, primarySiteID)
	require.Equal(t, otherSite.ID, *primarySiteID)
	require.Nil(t, primaryDomainID)

	instructions, err := svc.Instructions(ctx, fx.account.ID, pending.ID)
	require.NoError(t, err)
	token := instructions.Records[0].Content
	fx.dns.SetOwnership(pending.Hostname, token)
	_, err = svc.Verify(ctx, "phase3-actor", fx.account.ID, pending.ID)
	require.NoError(t, err)
	workerID := "phase3-primary-claim-" + uuid.NewString()
	job, err := fx.jobs.Claim(ctx, workerID)
	require.NoError(t, err)
	require.NoError(t, fx.processor.Handle(ctx, job, workerID))
	reloaded, err := fx.domainRepo.GetForWorker(ctx, pending.ID)
	require.NoError(t, err)
	require.Equal(t, model.DomainFailed, reloaded.Status)
	require.NotNil(t, reloaded.LastError)
	require.Equal(t, "domain verification could not be completed", *reloaded.LastError)
	require.NoError(t, fx.db.NewRaw(`
		SELECT site_id, domain_id FROM hostname_claims WHERE hostname = ?
	`, otherSite.Domain).Scan(ctx, &primarySiteID, &primaryDomainID))
	require.Equal(t, otherSite.ID, *primarySiteID, "the primary-site claim must always win")
	require.Nil(t, primaryDomainID)
	var provisionJobs, verifiedAudits int
	require.NoError(t, fx.db.NewRaw(`
		SELECT count(*) FROM jobs WHERE kind = ? AND payload ->> 'domain_id' = ?
	`, model.JobProvisionDomain, pending.ID.String()).Scan(ctx, &provisionJobs))
	require.Zero(t, provisionJobs)
	require.NoError(t, fx.db.NewRaw(`
		SELECT count(*) FROM audit_logs WHERE target = ? AND action = ?
	`, pending.ID.String(), model.AuditDomainVerified).Scan(ctx, &verifiedAudits))
	require.Zero(t, verifiedAudits)
}

func TestPhase3ConcurrentTenantsProveOneHostnameButOnlyOneClaimsIt(t *testing.T) {
	fx := newPhase3Fixture(t, model.SiteActive, model.DomainPending)
	ctx := context.Background()
	hostname := fx.domain.Hostname
	firstService := service.NewDomainService(
		fx.db, fx.domainRepo, fx.sites, fx.jobs, fx.audit,
		fx.dns, fx.signer, "8.8.8.8", true,
	)

	otherAccount := &model.Account{
		ID: uuid.New(), Name: "Concurrent hostname tenant", Status: model.AccountActive,
		CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}
	_, err := fx.db.NewInsert().Model(otherAccount).Exec(ctx)
	require.NoError(t, err)
	otherSite := &model.Site{
		ID: uuid.New(), AccountID: otherAccount.ID, NodeID: fx.nodes[0].ID,
		Domain: "concurrent-primary-" + uuid.NewString() + ".example.com",
		Image:  "opencloud/site-static:phase2", InternalPort: 8080,
		MemoryBytes: 256 * 1024 * 1024, NanoCPUs: 500_000_000,
		Status: model.SiteActive,
	}
	require.NoError(t, fx.sites.Create(ctx, otherSite))
	t.Cleanup(func() {
		_, _ = fx.db.NewDelete().Model((*model.Job)(nil)).Where("account_id = ?", otherAccount.ID).Exec(ctx)
		_, _ = fx.db.NewDelete().Model((*model.Site)(nil)).Where("id = ?", otherSite.ID).Exec(ctx)
		_, _ = fx.db.NewDelete().Model((*model.Account)(nil)).Where("id = ?", otherAccount.ID).Exec(ctx)
	})
	secondService := service.NewDomainService(
		fx.db, fx.domainRepo, fx.sites, fx.jobs, fx.audit,
		fx.dns, fx.signer, "8.8.8.8", true,
	)
	second, err := secondService.Attach(
		ctx, "phase3-other-actor", otherAccount.ID, otherSite.ID, uuid.NewString(),
		service.AttachDomainRequest{Hostname: hostname},
	)
	require.NoError(t, err, "pending challenges for the same hostname may span tenants")

	var pendingRows, claims int
	require.NoError(t, fx.db.NewRaw(`
		SELECT count(*) FROM domains WHERE hostname = ? AND deleted_at IS NULL
	`, hostname).Scan(ctx, &pendingRows))
	require.Equal(t, 2, pendingRows)
	require.NoError(t, fx.db.NewRaw(`
		SELECT count(*) FROM hostname_claims WHERE hostname = ?
	`, hostname).Scan(ctx, &claims))
	require.Zero(t, claims, "pending challenges must not squat the global hostname")

	firstInstructions, err := firstService.Instructions(ctx, fx.account.ID, fx.domain.ID)
	require.NoError(t, err)
	secondInstructions, err := secondService.Instructions(ctx, otherAccount.ID, second.ID)
	require.NoError(t, err)
	require.Equal(t, firstInstructions.Records[0].Name, secondInstructions.Records[0].Name)
	require.Equal(t, "_opencloud-verification."+hostname, firstInstructions.Records[0].Name)
	require.NotEqual(t, firstInstructions.Records[0].Content, secondInstructions.Records[0].Content)
	fx.dns.SetOwnership(hostname, firstInstructions.Records[0].Content)
	fx.dns.SetOwnership(hostname, secondInstructions.Records[0].Content)

	_, err = firstService.Verify(ctx, "phase3-actor", fx.account.ID, fx.domain.ID)
	require.NoError(t, err)
	_, err = secondService.Verify(ctx, "phase3-other-actor", otherAccount.ID, second.ID)
	require.NoError(t, err)
	workerIDs := []string{
		"phase3-claim-a-" + uuid.NewString(),
		"phase3-claim-b-" + uuid.NewString(),
	}
	jobs := make([]*model.Job, 0, 2)
	for _, workerID := range workerIDs {
		job, err := fx.jobs.Claim(ctx, workerID)
		require.NoError(t, err)
		require.Equal(t, model.JobVerifyDomain, job.Kind)
		jobs = append(jobs, job)
	}

	start := make(chan struct{})
	results := make(chan error, 2)
	var workers sync.WaitGroup
	for index := range jobs {
		workers.Add(1)
		go func(index int) {
			defer workers.Done()
			<-start
			results <- fx.processor.Handle(ctx, jobs[index], workerIDs[index])
		}(index)
	}
	close(start)
	workers.Wait()
	close(results)
	for result := range results {
		require.NoError(t, result)
	}

	first, err := fx.domainRepo.GetForWorker(ctx, fx.domain.ID)
	require.NoError(t, err)
	second, err = fx.domainRepo.GetForWorker(ctx, second.ID)
	require.NoError(t, err)
	domains := []*model.Domain{first, second}
	var winner, loser *model.Domain
	for _, domain := range domains {
		switch domain.Status {
		case model.DomainDNSPending:
			winner = domain
		case model.DomainFailed:
			loser = domain
		}
	}
	require.NotNil(t, winner)
	require.NotNil(t, loser)
	require.NotNil(t, winner.VerificationConsumedAt)
	require.Nil(t, loser.VerificationConsumedAt)
	require.NotNil(t, loser.LastError)
	require.Equal(t, "domain verification could not be completed", *loser.LastError)

	var claimDomainID uuid.UUID
	require.NoError(t, fx.db.NewRaw(`
		SELECT domain_id FROM hostname_claims WHERE hostname = ? AND site_id IS NULL
	`, hostname).Scan(ctx, &claimDomainID))
	require.Equal(t, winner.ID, claimDomainID)
	var winnerProvisionJobs, loserProvisionJobs, verifiedAudits int
	require.NoError(t, fx.db.NewRaw(`
		SELECT count(*) FROM jobs WHERE kind = ? AND payload ->> 'domain_id' = ?
	`, model.JobProvisionDomain, winner.ID.String()).Scan(ctx, &winnerProvisionJobs))
	require.Equal(t, 1, winnerProvisionJobs)
	require.NoError(t, fx.db.NewRaw(`
		SELECT count(*) FROM jobs WHERE kind = ? AND payload ->> 'domain_id' = ?
	`, model.JobProvisionDomain, loser.ID.String()).Scan(ctx, &loserProvisionJobs))
	require.Zero(t, loserProvisionJobs)
	require.NoError(t, fx.db.NewRaw(`
		SELECT count(*) FROM audit_logs WHERE action = ? AND target IN (?, ?)
	`, model.AuditDomainVerified, first.ID.String(), second.ID.String()).Scan(ctx, &verifiedAudits))
	require.Equal(t, 1, verifiedAudits)
}

func TestPhase3DomainAttachRollsBackWhenAuditAppendFails(t *testing.T) {
	fx := newPhase3Fixture(t, model.SiteActive, model.DomainPending)
	ctx := context.Background()
	_, err := fx.db.ExecContext(ctx, `
		CREATE OR REPLACE FUNCTION fail_phase3_domain_audit() RETURNS trigger AS $$
		BEGIN
			IF NEW.action = 'domain.attached' THEN
				RAISE EXCEPTION 'forced phase3 domain audit failure';
			END IF;
			RETURN NEW;
		END;
		$$ LANGUAGE plpgsql;
		CREATE TRIGGER fail_phase3_domain_audit_trigger
		BEFORE INSERT ON audit_logs
		FOR EACH ROW EXECUTE FUNCTION fail_phase3_domain_audit();
	`)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = fx.db.ExecContext(ctx, `DROP TRIGGER IF EXISTS fail_phase3_domain_audit_trigger ON audit_logs`)
		_, _ = fx.db.ExecContext(ctx, `DROP FUNCTION IF EXISTS fail_phase3_domain_audit()`)
	})

	svc := service.NewDomainService(
		fx.db, fx.domainRepo, fx.sites, fx.jobs, fx.audit,
		fx.dns, fx.signer, "203.0.113.10", true,
	)
	hostname := "audit-fail-" + uuid.NewString() + ".example.com"
	var jobsBefore int
	require.NoError(t, fx.db.NewRaw(`SELECT count(*) FROM jobs WHERE account_id = ?`, fx.account.ID).Scan(ctx, &jobsBefore))
	_, err = svc.Attach(
		ctx, "phase3-actor", fx.account.ID, fx.site.ID, uuid.NewString(),
		service.AttachDomainRequest{Hostname: hostname},
	)
	require.Error(t, err)
	var count int
	require.NoError(t, fx.db.NewRaw(`SELECT count(*) FROM domains WHERE hostname = ?`, hostname).Scan(ctx, &count))
	require.Zero(t, count, "domain mutation must roll back with its failed audit")
	var jobsAfter int
	require.NoError(t, fx.db.NewRaw(`SELECT count(*) FROM jobs WHERE account_id = ?`, fx.account.ID).Scan(ctx, &jobsAfter))
	require.Equal(t, jobsBefore, jobsAfter, "failed attach must not leave durable work behind")
}
