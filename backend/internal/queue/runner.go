// Package queue implements the durable PostgreSQL job worker.
package queue

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
	"go.uber.org/zap"

	"github.com/nazxf/opencloud/backend/internal/credential"
	"github.com/nazxf/opencloud/backend/internal/model"
	"github.com/nazxf/opencloud/backend/internal/provisioner"
	"github.com/nazxf/opencloud/backend/internal/repository"
)

const (
	defaultPollInterval = 2 * time.Second
	defaultLeaseTimeout = 10 * time.Minute
	reconcileInterval   = 5 * time.Minute
	maxRetryBackoff     = 5 * time.Minute
)

// Runner claims and executes jobs until its context is cancelled.
type Runner struct {
	db           *bun.DB
	jobs         *repository.JobRepo
	processor    *Processor
	log          *zap.Logger
	workerID     string
	pollInterval time.Duration
	leaseTimeout time.Duration
}

// NewRunner constructs a production runner with a unique lease identity.
func NewRunner(
	db *bun.DB,
	jobs *repository.JobRepo,
	processor *Processor,
	log *zap.Logger,
) *Runner {
	return &Runner{
		db:           db,
		jobs:         jobs,
		processor:    processor,
		log:          log,
		workerID:     "worker-" + uuid.NewString(),
		pollInterval: defaultPollInterval,
		leaseTimeout: defaultLeaseTimeout,
	}
}

// Run polls for work. Job execution is synchronous per runner; horizontal
// workers provide concurrency through SKIP LOCKED.
func (r *Runner) Run(ctx context.Context) {
	poll := time.NewTicker(r.pollInterval)
	defer poll.Stop()
	reap := time.NewTicker(r.leaseTimeout / 2)
	defer reap.Stop()
	reconcile := time.NewTicker(reconcileInterval)
	defer reconcile.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-reap.C:
			r.reap(ctx)
		case <-reconcile.C:
			r.enqueueReconciliation(ctx)
		case <-poll.C:
			for {
				worked, err := r.ProcessOne(ctx)
				if err != nil {
					r.log.Error("job processing failed", zap.String("error_class", "queue_state_update_failed"))
					break
				}
				if !worked {
					break
				}
			}
		}
	}
}

func (r *Runner) enqueueReconciliation(ctx context.Context) {
	sites, err := r.processor.sites.ListReconciliationCandidates(ctx, 100)
	if err != nil {
		r.log.Error("reconciliation scan failed", zap.String("error_class", "reconciliation_scan_failed"))
		return
	}
	var queued int
	for _, site := range sites {
		inserted, err := r.jobs.EnqueueUniqueSite(ctx, site.AccountID, model.JobReconcileSite, site.ID)
		if err != nil {
			r.log.Error(
				"reconciliation enqueue failed",
				zap.String("site_id", site.ID.String()),
				zap.String("error_class", "reconciliation_enqueue_failed"),
			)
			continue
		}
		if inserted {
			queued++
		}
	}
	if queued > 0 {
		r.log.Info("reconciliation jobs queued", zap.Int("count", queued))
	}
}

// ProcessOne claims at most one job and returns whether work was found.
func (r *Runner) ProcessOne(ctx context.Context) (bool, error) {
	job, err := r.jobs.Claim(ctx, r.workerID)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("claim job: %w", err)
	}

	if err := r.processor.Handle(ctx, job, r.workerID); err != nil {
		safeError := "provisioner operation failed"
		if job.Attempts >= job.MaxAttempts {
			if exhaustErr := r.processor.Exhaust(ctx, job, r.workerID, safeError); exhaustErr != nil {
				return true, fmt.Errorf("mark exhausted job %s: %w", job.ID, exhaustErr)
			}
			r.log.Warn(
				"job exhausted retries",
				zap.String("job_id", job.ID.String()),
				zap.String("kind", job.Kind),
			)
			return true, nil
		}
		runAt := time.Now().UTC().Add(retryBackoff(job.Attempts))
		if retryErr := r.jobs.Retry(ctx, job.ID, r.workerID, safeError, runAt); retryErr != nil {
			return true, fmt.Errorf("retry job %s: %w", job.ID, retryErr)
		}
		r.log.Warn(
			"job scheduled for retry",
			zap.String("job_id", job.ID.String()),
			zap.String("kind", job.Kind),
			zap.Int("attempt", job.Attempts),
		)
	}
	return true, nil
}

func (r *Runner) reap(ctx context.Context) {
	reaped, err := r.jobs.ReapStale(ctx, time.Now().UTC().Add(-r.leaseTimeout))
	if err != nil {
		r.log.Error("stale job reaper failed", zap.String("error_class", "queue_reaper_failed"))
		return
	}
	if reaped > 0 {
		r.log.Warn("requeued stale jobs", zap.Int64("count", reaped))
	}
}

func retryBackoff(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	backoff := time.Second << min(attempt-1, 8)
	if backoff > maxRetryBackoff {
		return maxRetryBackoff
	}
	return backoff
}

// Processor executes one provider operation and commits its control-plane
// transition plus audit event with the job completion.
type Processor struct {
	db                  *bun.DB
	sites               *repository.SiteRepo
	nodes               *repository.NodeRepo
	databases           *repository.ManagedDatabaseRepo
	jobs                *repository.JobRepo
	audit               *repository.AuditRepo
	provisioner         provisioner.SiteProvisioner
	databaseProvisioner provisioner.DatabaseProvisioner
	credentialCipher    *credential.Cipher
}

// NewProcessor constructs a Processor.
func NewProcessor(
	db *bun.DB,
	sites *repository.SiteRepo,
	nodes *repository.NodeRepo,
	jobs *repository.JobRepo,
	audit *repository.AuditRepo,
	p provisioner.SiteProvisioner,
	databases *repository.ManagedDatabaseRepo,
	databaseProvisioner provisioner.DatabaseProvisioner,
	credentialCipher *credential.Cipher,
) *Processor {
	return &Processor{
		db:                  db,
		sites:               sites,
		nodes:               nodes,
		databases:           databases,
		jobs:                jobs,
		audit:               audit,
		provisioner:         p,
		databaseProvisioner: databaseProvisioner,
		credentialCipher:    credentialCipher,
	}
}

type sitePayload struct {
	SiteID uuid.UUID `json:"site_id"`
}

// Handle executes an idempotent provider call, then atomically persists its
// domain state, audit row, and successful job status.
func (p *Processor) Handle(ctx context.Context, job *model.Job, workerID string) error {
	switch job.Kind {
	case model.JobProvisionDatabase, model.JobDeleteDatabase, model.JobCleanupDatabase:
		return p.handleDatabase(ctx, job, workerID)
	default:
		return p.handleSite(ctx, job, workerID)
	}
}

func (p *Processor) handleSite(ctx context.Context, job *model.Job, workerID string) error {
	var payload sitePayload
	if err := json.Unmarshal(job.Payload, &payload); err != nil || payload.SiteID == uuid.Nil {
		return errors.New("invalid job payload")
	}
	site, err := p.sites.GetForWorker(ctx, payload.SiteID)
	if err != nil {
		return fmt.Errorf("load job site: %w", err)
	}
	ref := provisioner.SiteRef{
		SiteID:    site.ID,
		AccountID: site.AccountID,
		NodeID:    site.NodeID,
	}

	var nextStatus, expectedPending, auditAction string
	switch job.Kind {
	case model.JobProvisionSite:
		err = p.provisioner.CreateSite(ctx, provisioner.SiteSpec{
			SiteID:       site.ID,
			AccountID:    site.AccountID,
			NodeID:       site.NodeID,
			Domain:       site.Domain,
			Image:        site.Image,
			InternalPort: uint16(site.InternalPort),
			MemoryBytes:  site.MemoryBytes,
			NanoCPUs:     site.NanoCPUs,
		})
		nextStatus, expectedPending, auditAction = model.SiteActive, model.SiteProvisioning, model.AuditSiteProvisioned
	case model.JobSuspendSite:
		err = p.provisioner.SuspendSite(ctx, ref)
		nextStatus, expectedPending, auditAction = model.SiteSuspended, model.SiteSuspending, model.AuditSiteSuspended
	case model.JobResumeSite:
		err = p.provisioner.ResumeSite(ctx, ref)
		nextStatus, expectedPending, auditAction = model.SiteActive, model.SiteResuming, model.AuditSiteResumed
	case model.JobDeleteSite, model.JobCleanupSite:
		err = p.provisioner.DeleteSite(ctx, ref)
		nextStatus, expectedPending, auditAction = model.SiteDeleted, model.SiteDeleting, model.AuditSiteDeleted
	case model.JobReconcileSite:
		err = p.reconcile(ctx, site, ref)
		nextStatus, auditAction = site.Status, model.AuditSiteReconciled
	default:
		return fmt.Errorf("unsupported job kind %q", job.Kind)
	}
	if err != nil {
		return err
	}

	return p.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		sites := p.sites.WithDB(tx)
		nodes := p.nodes.WithDB(tx)
		jobs := p.jobs.WithDB(tx)
		audit := p.audit.WithDB(tx)

		current, err := sites.GetForWorkerForUpdate(ctx, site.ID)
		if err != nil {
			return err
		}
		if job.Kind == model.JobDeleteSite || job.Kind == model.JobCleanupSite {
			nodeID, err := sites.MarkCapacityReleased(ctx, site.ID)
			if err != nil {
				return err
			}
			if nodeID != uuid.Nil {
				if err := nodes.ReleaseCapacity(ctx, nodeID); err != nil {
					return err
				}
			}
			if err := sites.MarkDeleted(ctx, site.ID); err != nil {
				return err
			}
		} else if job.Kind != model.JobReconcileSite && current.Status == expectedPending {
			if err := sites.SetWorkerStatus(ctx, site.ID, nextStatus, nil); err != nil {
				return err
			}
		}
		aid := current.AccountID
		if err := audit.Append(ctx, repository.Entry{
			AccountID: &aid,
			Action:    auditAction,
			Target:    stringPointer(site.ID.String()),
			Metadata: map[string]any{
				"domain":   current.Domain,
				"job_id":   job.ID.String(),
				"job_kind": job.Kind,
				"status":   nextStatus,
			},
		}); err != nil {
			return err
		}
		return jobs.Complete(ctx, job.ID, workerID)
	})
}

type databasePayload struct {
	DatabaseID uuid.UUID `json:"database_id"`
}

func (p *Processor) handleDatabase(
	ctx context.Context,
	job *model.Job,
	workerID string,
) error {
	if p.databases == nil || p.databaseProvisioner == nil || p.credentialCipher == nil {
		return errors.New("customer database provisioner is not configured")
	}
	var payload databasePayload
	if err := json.Unmarshal(job.Payload, &payload); err != nil || payload.DatabaseID == uuid.Nil {
		return errors.New("invalid database job payload")
	}
	row, err := p.databases.GetForWorker(ctx, payload.DatabaseID)
	if err != nil {
		return fmt.Errorf("load job database: %w", err)
	}
	ref := provisioner.DatabaseRef{
		DatabaseID:   row.ID,
		AccountID:    row.AccountID,
		Engine:       row.Engine,
		DatabaseName: row.PhysicalDatabaseName,
		Username:     row.PhysicalUsername,
	}

	switch job.Kind {
	case model.JobProvisionDatabase:
		credentials, err := p.databaseProvisioner.CreateDatabase(
			ctx,
			provisioner.DatabaseSpec(ref),
		)
		if err != nil {
			return err
		}
		plaintext, err := json.Marshal(credentials)
		credentials.Password = ""
		if err != nil {
			return err
		}
		defer clear(plaintext)
		envelope, err := p.credentialCipher.Encrypt(row.ID, plaintext)
		if err != nil {
			return err
		}
		return p.completeDatabaseProvision(ctx, job, workerID, row.ID, envelope)
	case model.JobDeleteDatabase, model.JobCleanupDatabase:
		if err := p.databaseProvisioner.DeleteDatabase(ctx, ref); err != nil {
			return err
		}
		return p.completeDatabaseDelete(ctx, job, workerID, row.ID)
	default:
		return fmt.Errorf("unsupported database job kind %q", job.Kind)
	}
}

func (p *Processor) completeDatabaseProvision(
	ctx context.Context,
	job *model.Job,
	workerID string,
	databaseID uuid.UUID,
	envelope []byte,
) error {
	return p.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		rows := p.databases.WithDB(tx)
		jobs := p.jobs.WithDB(tx)
		audit := p.audit.WithDB(tx)

		current, err := rows.GetForWorkerForUpdate(ctx, databaseID)
		if err != nil {
			return err
		}
		action := model.AuditDatabaseProvisioned
		resultStatus := current.Status
		if current.Status == model.DatabaseProvisioning {
			if err := rows.StoreCredential(ctx, databaseID, envelope); err != nil {
				return err
			}
			if err := rows.SetWorkerStatus(ctx, databaseID, model.DatabaseActive, nil); err != nil {
				return err
			}
			resultStatus = model.DatabaseActive
		} else {
			// A newer delete intent wins. The delete job will remove the
			// idempotently-created data-plane resource; no credential is stored.
			action = model.AuditDatabaseProvisionSuperseded
		}
		aid := current.AccountID
		if err := audit.Append(ctx, repository.Entry{
			AccountID: &aid,
			Action:    action,
			Target:    stringPointer(databaseID.String()),
			Metadata: map[string]any{
				"name":     current.Name,
				"engine":   current.Engine,
				"job_id":   job.ID.String(),
				"job_kind": job.Kind,
				"status":   resultStatus,
			},
		}); err != nil {
			return err
		}
		return jobs.Complete(ctx, job.ID, workerID)
	})
}

func (p *Processor) completeDatabaseDelete(
	ctx context.Context,
	job *model.Job,
	workerID string,
	databaseID uuid.UUID,
) error {
	return p.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		rows := p.databases.WithDB(tx)
		jobs := p.jobs.WithDB(tx)
		audit := p.audit.WithDB(tx)

		current, err := rows.GetForWorkerForUpdate(ctx, databaseID)
		if err != nil {
			return err
		}
		if err := rows.DeleteCredential(ctx, databaseID); err != nil {
			return err
		}
		action := model.AuditDatabaseCleanupCompleted
		resultStatus := current.Status
		if job.Kind == model.JobDeleteDatabase {
			if err := rows.MarkDeleted(ctx, databaseID); err != nil {
				return err
			}
			action = model.AuditDatabaseDeleted
			resultStatus = model.DatabaseDeleted
		}
		aid := current.AccountID
		if err := audit.Append(ctx, repository.Entry{
			AccountID: &aid,
			Action:    action,
			Target:    stringPointer(databaseID.String()),
			Metadata: map[string]any{
				"name":     current.Name,
				"engine":   current.Engine,
				"job_id":   job.ID.String(),
				"job_kind": job.Kind,
				"status":   resultStatus,
			},
		}); err != nil {
			return err
		}
		return jobs.Complete(ctx, job.ID, workerID)
	})
}

func (p *Processor) reconcile(ctx context.Context, site *model.Site, ref provisioner.SiteRef) error {
	observed, err := p.provisioner.SiteStatus(ctx, ref)
	if err != nil {
		return err
	}
	switch site.Status {
	case model.SiteActive:
		if observed == provisioner.SiteStateMissing {
			return p.provisioner.CreateSite(ctx, provisioner.SiteSpec{
				SiteID:       site.ID,
				AccountID:    site.AccountID,
				NodeID:       site.NodeID,
				Domain:       site.Domain,
				Image:        site.Image,
				InternalPort: uint16(site.InternalPort),
				MemoryBytes:  site.MemoryBytes,
				NanoCPUs:     site.NanoCPUs,
			})
		}
		if observed == provisioner.SiteStateSuspended {
			return p.provisioner.ResumeSite(ctx, ref)
		}
	case model.SiteSuspended:
		if observed == provisioner.SiteStateMissing {
			if err := p.provisioner.CreateSite(ctx, provisioner.SiteSpec{
				SiteID:       site.ID,
				AccountID:    site.AccountID,
				NodeID:       site.NodeID,
				Domain:       site.Domain,
				Image:        site.Image,
				InternalPort: uint16(site.InternalPort),
				MemoryBytes:  site.MemoryBytes,
				NanoCPUs:     site.NanoCPUs,
			}); err != nil {
				return err
			}
			return p.provisioner.SuspendSite(ctx, ref)
		}
		if observed == provisioner.SiteStateRunning {
			return p.provisioner.SuspendSite(ctx, ref)
		}
	case model.SiteDeleting, model.SiteDeleted:
		if observed != provisioner.SiteStateMissing {
			return p.provisioner.DeleteSite(ctx, ref)
		}
	}
	return nil
}

// Exhaust atomically fails a job, marks the resource failed, appends an audit,
// and enqueues compensating cleanup after failed provisioning.
func (p *Processor) Exhaust(ctx context.Context, job *model.Job, workerID, safeError string) error {
	switch job.Kind {
	case model.JobProvisionDatabase, model.JobDeleteDatabase, model.JobCleanupDatabase:
		return p.exhaustDatabase(ctx, job, workerID, safeError)
	default:
		return p.exhaustSite(ctx, job, workerID, safeError)
	}
}

func (p *Processor) exhaustSite(ctx context.Context, job *model.Job, workerID, safeError string) error {
	var payload sitePayload
	if err := json.Unmarshal(job.Payload, &payload); err != nil {
		return err
	}
	return p.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		sites := p.sites.WithDB(tx)
		jobs := p.jobs.WithDB(tx)
		audit := p.audit.WithDB(tx)

		site, err := sites.GetForWorkerForUpdate(ctx, payload.SiteID)
		if err != nil {
			return err
		}
		currentStatus := site.Status
		if currentStatus != model.SiteDeleting {
			if err := sites.SetWorkerStatus(ctx, site.ID, model.SiteFailed, &safeError); err != nil {
				return err
			}
		}
		if err := jobs.Fail(ctx, job.ID, workerID, safeError); err != nil {
			return err
		}
		if job.Kind == model.JobProvisionSite && currentStatus != model.SiteDeleting {
			aid := site.AccountID
			if _, err := jobs.Enqueue(ctx, &aid, model.JobCleanupSite, sitePayload{SiteID: site.ID}); err != nil {
				return err
			}
		}
		aid := site.AccountID
		if err := audit.Append(ctx, repository.Entry{
			AccountID: &aid,
			Action:    model.AuditSiteFailed,
			Target:    stringPointer(site.ID.String()),
			Metadata:  map[string]any{"job_id": job.ID.String(), "job_kind": job.Kind},
		}); err != nil {
			return err
		}
		return nil
	})
}

func (p *Processor) exhaustDatabase(
	ctx context.Context,
	job *model.Job,
	workerID, safeError string,
) error {
	if p.databases == nil {
		return errors.New("customer database repository is not configured")
	}
	var payload databasePayload
	if err := json.Unmarshal(job.Payload, &payload); err != nil || payload.DatabaseID == uuid.Nil {
		return errors.New("invalid database job payload")
	}
	return p.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		rows := p.databases.WithDB(tx)
		jobs := p.jobs.WithDB(tx)
		audit := p.audit.WithDB(tx)

		current, err := rows.GetForWorkerForUpdate(ctx, payload.DatabaseID)
		if err != nil {
			return err
		}
		if err := jobs.Fail(ctx, job.ID, workerID, safeError); err != nil {
			return err
		}
		if err := rows.DeleteCredential(ctx, current.ID); err != nil {
			return err
		}
		if current.Status != model.DatabaseDeleting && current.Status != model.DatabaseDeleted {
			if err := rows.SetWorkerStatus(ctx, current.ID, model.DatabaseFailed, &safeError); err != nil {
				return err
			}
		} else if job.Kind == model.JobDeleteDatabase {
			if err := rows.SetWorkerStatus(ctx, current.ID, model.DatabaseFailed, &safeError); err != nil {
				return err
			}
		}
		if job.Kind == model.JobProvisionDatabase && current.Status != model.DatabaseDeleting {
			aid := current.AccountID
			if _, err := jobs.Enqueue(
				ctx,
				&aid,
				model.JobCleanupDatabase,
				databasePayload{DatabaseID: current.ID},
			); err != nil {
				return err
			}
		}
		aid := current.AccountID
		return audit.Append(ctx, repository.Entry{
			AccountID: &aid,
			Action:    model.AuditDatabaseFailed,
			Target:    stringPointer(current.ID.String()),
			Metadata: map[string]any{
				"name":     current.Name,
				"engine":   current.Engine,
				"job_id":   job.ID.String(),
				"job_kind": job.Kind,
			},
		})
	})
}

func stringPointer(value string) *string { return &value }
