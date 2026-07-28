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
	db          *bun.DB
	sites       *repository.SiteRepo
	nodes       *repository.NodeRepo
	jobs        *repository.JobRepo
	audit       *repository.AuditRepo
	provisioner provisioner.SiteProvisioner
}

// NewProcessor constructs a Processor.
func NewProcessor(
	db *bun.DB,
	sites *repository.SiteRepo,
	nodes *repository.NodeRepo,
	jobs *repository.JobRepo,
	audit *repository.AuditRepo,
	p provisioner.SiteProvisioner,
) *Processor {
	return &Processor{
		db:          db,
		sites:       sites,
		nodes:       nodes,
		jobs:        jobs,
		audit:       audit,
		provisioner: p,
	}
}

type sitePayload struct {
	SiteID uuid.UUID `json:"site_id"`
}

// Handle executes an idempotent provider call, then atomically persists its
// domain state, audit row, and successful job status.
func (p *Processor) Handle(ctx context.Context, job *model.Job, workerID string) error {
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

func stringPointer(value string) *string { return &value }
