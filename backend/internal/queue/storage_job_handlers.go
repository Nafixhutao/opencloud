package queue

import (
	"context"
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

// StorageJobHandlers provides PostgreSQL queue workers for storage bucket jobs.
type StorageJobHandlers struct {
	log        *zap.Logger
	db         *bun.DB
	bucketRepo *repository.StorageBucketRepo
	jobRepo    *repository.JobRepo
	audit      *repository.AuditRepo
	provider   provisioner.ObjectStorageProvider
}

// NewStorageJobHandlers constructs storage queue workers.
func NewStorageJobHandlers(
	log *zap.Logger,
	db *bun.DB,
	bucketRepo *repository.StorageBucketRepo,
	jobRepo *repository.JobRepo,
	audit *repository.AuditRepo,
	provider provisioner.ObjectStorageProvider,
) *StorageJobHandlers {
	return &StorageJobHandlers{
		log: log, db: db, bucketRepo: bucketRepo, jobRepo: jobRepo, audit: audit, provider: provider,
	}
}

// Handle executes one storage job.
func (h *StorageJobHandlers) Handle(ctx context.Context, job *model.Job, workerID string) error {
	switch job.Kind {
	case model.JobProvisionStorageBucket:
		return h.handleProvision(ctx, job, workerID)
	case model.JobDeleteStorageBucket:
		return h.handleDelete(ctx, job, workerID)
	case model.JobReconcileStorageBucket:
		return h.handleReconcile(ctx, job, workerID)
	default:
		return fmt.Errorf("unknown storage job kind: %s", job.Kind)
	}
}

func parseStorageBucketPayload(job *model.Job) (uuid.UUID, error) {
	var payload model.ProvisionStorageBucketPayload
	if err := json.Unmarshal(job.Payload, &payload); err != nil {
		return uuid.Nil, fmt.Errorf("unmarshal provision payload: %w", err)
	}
	return payload.BucketID, nil
}

func parseDeleteStorageBucketPayload(job *model.Job) (uuid.UUID, error) {
	var payload model.DeleteStorageBucketPayload
	if err := json.Unmarshal(job.Payload, &payload); err != nil {
		return uuid.Nil, fmt.Errorf("unmarshal delete payload: %w", err)
	}
	return payload.BucketID, nil
}

func validateStorageJobOwnership(job *model.Job, bucket *model.StorageBucket) error {
	if job.AccountID == nil || *job.AccountID != bucket.AccountID {
		return errors.New("storage job ownership mismatch")
	}
	return nil
}

// handleProvision creates the underlying object storage bucket.
func (h *StorageJobHandlers) handleProvision(ctx context.Context, job *model.Job, workerID string) error {
	bucketID, err := parseStorageBucketPayload(job)
	if err != nil {
		return err
	}

	h.log.Info("provisioning storage bucket", zap.Stringer("bucket_id", bucketID))

	// Load bucket FOR UPDATE - we'll validate ownership after loading
	bucket, err := h.bucketRepo.GetByAccountForUpdate(ctx, *job.AccountID, bucketID)
	if err != nil {
		return fmt.Errorf("load bucket: %w", err)
	}

	if err := validateStorageJobOwnership(job, bucket); err != nil {
		return errors.New("job ownership mismatch")
	}

	accountID := *job.AccountID

	if bucket.Status == model.BucketDeleted || bucket.Status == model.BucketDeleting {
		h.log.Warn("bucket status changed during provisioning",
			zap.Stringer("bucket_id", bucketID), zap.String("status", bucket.Status))
		return nil // Exit silently; deletion took precedence
	}

	// Invoke provider outside SQL transaction
	err = h.provider.CreateBucket(ctx, provisioner.BucketSpec{
		BucketID:     bucket.ID,
		AccountID:    bucket.AccountID,
		PhysicalName: bucket.PhysicalName,
		Visibility:   bucket.Visibility,
	})

	if err != nil {
		if errors.Is(err, provisioner.ErrBucketExists) {
			h.log.Warn("bucket already exists at provider", zap.Stringer("bucket_id", bucketID))
			// Update state to active since resource exists
			return h.completeCreation(ctx, job.ID, bucketID, workerID, accountID)
		}

		var bucketErr provisioner.BucketNotEmptyError
		if errors.As(err, &bucketErr) {
			h.log.Warn("provider reported bucket not empty during create",
				zap.Stringer("bucket_id", bucketID))
			// Return to active with error so customer can remediate
			return h.restoreActiveWithError(ctx, job.ID, bucketID, workerID, accountID, err.Error())
		}

		h.log.Warn("provision failed", zap.Stringer("bucket_id", bucketID), zap.Error(err))
		// Job lifecycle handled by Processor.Handle(): will Retry or Fail based on attempt count
		return fmt.Errorf("provision failed: %w", err)
	}

	// Success path: mark active
	return h.completeCreation(ctx, job.ID, bucketID, workerID, accountID)
}

func (h *StorageJobHandlers) completeCreation(ctx context.Context, jobID uuid.UUID, bucketID uuid.UUID, workerID string, accountID uuid.UUID) error {
	now := time.Now().UTC()

	// Atomic transaction: update bucket state + audit event + job completion together
	err := h.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		bucketRepo := h.bucketRepo.WithDB(tx)
		auditRepo := h.audit.WithDB(tx)
		jobRepo := h.jobRepo.WithDB(tx)

		result, err := bucketRepo.UpdateStatusCompleted(
			ctx, bucketID, model.BucketActive, now, nil,
		)
		if err != nil {
			return fmt.Errorf("update to active: %w", err)
		}
		rows, _ := result.RowsAffected()
		if rows == 0 {
			return errors.New("no rows updated; bucket may have been deleted")
		}

		// Audit success
		aid := accountID
		if err := auditRepo.Append(ctx, repository.Entry{
			AccountID: &aid, Action: model.AuditStorageBucketProvisioned, Target: strPtr(bucketID.String()),
		}); err != nil {
			return fmt.Errorf("append provisioned audit: %w", err)
		}

		// Mark job succeeded
		if err := jobRepo.Complete(ctx, jobID, workerID); err != nil {
			return fmt.Errorf("complete job: %w", err)
		}

		return nil
	})

	return err
}

// restoreActiveWithError restores a bucket from creating->active when BUCKET_NOT_EMPTY,
// completing the job atomically within a transaction to ensure consistent state.
func (h *StorageJobHandlers) restoreActiveWithError(ctx context.Context, jobID uuid.UUID, bucketID uuid.UUID, workerID string, accountID uuid.UUID, message string) error {
	now := time.Now().UTC()

	// Atomic transaction: update bucket state + audit event + job completion together
	err := h.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		bucketRepo := h.bucketRepo.WithDB(tx)
		auditRepo := h.audit.WithDB(tx)
		jobRepo := h.jobRepo.WithDB(tx)

		// Transition bucket to active with error marker
		result, err := bucketRepo.UpdateStatusActiveWithMessage(
			ctx, bucketID, message, now,
		)
		if err != nil {
			return fmt.Errorf("restore bucket to active: %w", err)
		}
		rows, _ := result.RowsAffected()
		if rows == 0 {
			return errors.New("no rows updated; bucket state uncertain")
		}

		// Audit failure
		aid := accountID
		if err := auditRepo.Append(ctx, repository.Entry{
			AccountID: &aid, Action: model.AuditStorageBucketProvisionFailed, Target: strPtr(bucketID.String()), Metadata: map[string]any{"error": message},
		}); err != nil {
			return fmt.Errorf("append provision_failed audit: %w", err)
		}

		// Do not mark job failed; we returned bucket to active for remediation
		// Provider-reported BUCKET_NOT_EMPTY means customer must manually remove objects
		// Then retry delete later
		if err := jobRepo.Complete(ctx, jobID, workerID); err != nil {
			return fmt.Errorf("complete job: %w", err)
		}

		return nil
	})

	return err
}

// handleDelete removes the underlying object storage bucket.
func (h *StorageJobHandlers) handleDelete(ctx context.Context, job *model.Job, workerID string) error {
	bucketID, err := parseDeleteStorageBucketPayload(job)
	if err != nil {
		return err
	}

	h.log.Info("deleting storage bucket", zap.Stringer("bucket_id", bucketID))

	// Load bucket and validate ownership in one step
	bucket, err := h.bucketRepo.GetByAccount(ctx, *job.AccountID, bucketID)
	if err != nil {
		return fmt.Errorf("load bucket: %w", err)
	}

	if err := validateStorageJobOwnership(job, bucket); err != nil {
		return errors.New("job ownership mismatch")
	}

	accountID := *job.AccountID

	if bucket.Status != model.BucketDeleting {
		h.log.Warn("delete job started but bucket not in deleting state",
			zap.Stringer("bucket_id", bucketID), zap.String("status", bucket.Status))
		return nil
	}

	// Invoke provider outside SQL transaction
	err = h.provider.DeleteBucket(ctx, provisioner.BucketRef{
		BucketID:     bucket.ID,
		AccountID:    bucket.AccountID,
		PhysicalName: bucket.PhysicalName,
	})

	if err != nil {
		if errors.Is(err, provisioner.ErrBucketNotFound) {
			h.log.Warn("bucket not found at provider; finalize deletion", zap.Stringer("bucket_id", bucketID))
			return h.finalizeDeletion(ctx, job.ID, bucketID, workerID, accountID)
		}

		var bucketErr provisioner.BucketNotEmptyError
		if errors.As(err, &bucketErr) {
			h.log.Warn("provider reported bucket not empty during delete", zap.Stringer("bucket_id", bucketID))
			// Return to active; customer must manually remove objects
			return h.restoreDeleteBlocked(ctx, job.ID, bucketID, workerID, accountID, "BUCKET_NOT_EMPTY")
		}

		h.log.Warn("delete failed", zap.Stringer("bucket_id", bucketID), zap.Error(err))
		// Job lifecycle handled by Processor.Handle(): will Retry or Fail based on attempt count
		return fmt.Errorf("delete failed: %w", err)
	}

	// Success path: finalize deletion
	return h.finalizeDeletion(ctx, job.ID, bucketID, workerID, accountID)
}

// restoreDeleteBlocked restores a bucket from deleting->active when BUCKET_NOT_EMPTY,
// completing the job atomically within a transaction to ensure consistent state.
// bucketID is the target bucket UUID; jobID is the queue job UUID being processed.
func (h *StorageJobHandlers) restoreDeleteBlocked(ctx context.Context, jobID uuid.UUID, bucketID uuid.UUID, workerID string, accountID uuid.UUID, reason string) error {
	now := time.Now().UTC()

	// Atomic transaction: update bucket state + audit event + job completion together
	err := h.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		bucketRepo := h.bucketRepo.WithDB(tx)
		auditRepo := h.audit.WithDB(tx)
		jobRepo := h.jobRepo.WithDB(tx)

		// Transition bucket back to active with error marker
		result, err := bucketRepo.RestoreDeletingToActiveWithError(ctx, bucketID, reason, now)
		if err != nil {
			return fmt.Errorf("restore deleting->active: %w", err)
		}
		rows, _ := result.RowsAffected()
		if rows == 0 {
			return errors.New("no rows updated; bucket state uncertain")
		}

		// Audit delete_blocked event
		aid := accountID
		if err := auditRepo.Append(ctx, repository.Entry{
			AccountID: &aid, Action: model.AuditStorageBucketDeleteBlocked, Target: strPtr(bucketID.String()), Metadata: map[string]any{"reason": reason},
		}); err != nil {
			return fmt.Errorf("append delete_blocked audit: %w", err)
		}

		// Complete the job atomically
		if err := jobRepo.Complete(ctx, jobID, workerID); err != nil {
			return fmt.Errorf("complete job: %w", err)
		}

		return nil
	})

	return err
}

func (h *StorageJobHandlers) finalizeDeletion(ctx context.Context, jobID uuid.UUID, bucketID uuid.UUID, workerID string, accountID uuid.UUID) error {
	// Atomic transaction: update bucket state + audit event + job completion together
	err := h.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		bucketRepo := h.bucketRepo.WithDB(tx)
		auditRepo := h.audit.WithDB(tx)
		jobRepo := h.jobRepo.WithDB(tx)

		result, err := bucketRepo.MarkDeleted(ctx, bucketID)
		if err != nil {
			return fmt.Errorf("mark deleted: %w", err)
		}
		rows, _ := result.RowsAffected()
		if rows == 0 {
			return errors.New("no rows updated; bucket state uncertain")
		}

		// Audit success
		aid := accountID
		if err := auditRepo.Append(ctx, repository.Entry{
			AccountID: &aid, Action: model.AuditStorageBucketDeleted, Target: strPtr(bucketID.String()),
		}); err != nil {
			return fmt.Errorf("append deleted audit: %w", err)
		}

		// Mark job succeeded
		if err := jobRepo.Complete(ctx, jobID, workerID); err != nil {
			return fmt.Errorf("complete job: %w", err)
		}

		return nil
	})

	return err
}

// handleReconcile scans for buckets that need convergence and updates their status.
func (h *StorageJobHandlers) handleReconcile(ctx context.Context, job *model.Job, workerID string) error {
	h.log.Info("reconciling storage buckets", zap.Stringer("account_id", *job.AccountID))

	// Find stale or failing buckets without an active job
	staleBuckets, err := h.bucketRepo.FindStaleAndFailingBuckets(ctx, *job.AccountID)
	if err != nil {
		return fmt.Errorf("find stale buckets: %w", err)
	}

	for _, b := range staleBuckets {
		h.log.Info("reconciling stale/failing bucket", zap.Stringer("bucket_id", b.ID), zap.String("status", b.Status))

		// Check for active provision job by querying jobs table directly
		hasActiveJob, err := h.hasActiveProvisionJob(ctx, *job.AccountID, b.ID)
		if err != nil {
			h.log.Warn("check active job failed", zap.Stringer("bucket_id", b.ID), zap.Error(err))
			hasActiveJob = false
		}

		if b.Status == model.BucketCreating && !hasActiveJob {
			// Transition creating -> failed if no running provision job
			if err := h.transitionCreateToFailed(ctx, b); err != nil {
				h.log.Warn("transition create->failed failed", zap.Stringer("bucket_id", b.ID), zap.Error(err))
			}
		} else if (b.Status == model.BucketFailed || b.Status == model.BucketDeleting) && hasActiveJob {
			// Restore from failed/deleting while job is actively processing
			if err := h.restoreFromTerminal(ctx, b); err != nil {
				h.log.Warn("restore from terminal failed", zap.Stringer("bucket_id", b.ID), zap.Error(err))
			}
		} else {
			// Update last_reconciled_at as heartbeat
			if err := h.updateReconciledAt(ctx, b.ID); err != nil {
				h.log.Warn("update reconciled_at failed", zap.Stringer("bucket_id", b.ID), zap.Error(err))
			}
		}
	}

	return nil
}

// hasActiveProvisionJob queries for a running OR queued provision job for the given bucket.
// Queued jobs may be retried after transient failure or reclaimed by ReapStale().
func (h *StorageJobHandlers) hasActiveProvisionJob(ctx context.Context, accountID uuid.UUID, bucketID uuid.UUID) (bool, error) {
	var count int
	err := h.db.NewRaw(`
		SELECT count(*) FROM jobs
		WHERE account_id = ?
		  AND kind = ?
		  AND status IN (?, ?)
		  AND payload->>'bucket_id' = ?
	`, accountID, model.JobProvisionStorageBucket, model.JobQueued, model.JobRunning, bucketID.String()).Scan(ctx, &count)
	return count > 0, err
}

func (h *StorageJobHandlers) transitionCreateToFailed(ctx context.Context, bucket model.StorageBucket) error {
	now := time.Now().UTC()
	return h.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		bucketRepo := h.bucketRepo.WithDB(tx)
		auditRepo := h.audit.WithDB(tx)

		result, err := bucketRepo.SetStatus(ctx, bucket.ID, model.BucketFailed, now)
		if err != nil {
			return err
		}
		rows, _ := result.RowsAffected()
		if rows == 0 {
			return errors.New("no rows updated")
		}

		aid := bucket.AccountID
		return auditRepo.Append(ctx, repository.Entry{
			AccountID: &aid, Action: model.AuditStorageBucketProvisionFailed, Target: strPtr(bucket.ID.String()),
		})
	})
}

func (h *StorageJobHandlers) restoreFromTerminal(ctx context.Context, bucket model.StorageBucket) error {
	now := time.Now().UTC()
	return h.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		bucketRepo := h.bucketRepo.WithDB(tx)

		var status string
		if bucket.Status == model.BucketFailed {
			status = model.BucketActive
		} else if bucket.Status == model.BucketDeleting {
			status = model.BucketDeleting // Keep deleting but reset error
		}
		result, err := bucketRepo.UpdateStatusNoError(ctx, bucket.ID, status, now)
		if err != nil {
			return err
		}
		rows, _ := result.RowsAffected()
		if rows == 0 {
			return errors.New("no rows updated")
		}
		return nil
	})
}

func (h *StorageJobHandlers) updateReconciledAt(ctx context.Context, bucketID uuid.UUID) error {
	result, err := h.bucketRepo.UpdateReconciledAt(ctx, bucketID)
	if err != nil {
		return err
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return errors.New("no rows updated")
	}
	return nil
}

func strPtr(s string) *string { return &s }
