package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"

	"github.com/nazxf/opencloud/backend/internal/model"
)

// JobRepo is the PostgreSQL-backed queue repository.
type JobRepo struct {
	db bun.IDB
}

// NewJobRepo constructs a JobRepo.
func NewJobRepo(db bun.IDB) *JobRepo {
	return &JobRepo{db: db}
}

// WithDB returns a copy using db.
func (r *JobRepo) WithDB(db bun.IDB) *JobRepo {
	return &JobRepo{db: db}
}

// Enqueue inserts work in the caller's resource transaction.
func (r *JobRepo) Enqueue(ctx context.Context, accountID *uuid.UUID, kind string, payload any) (*model.Job, error) {
	return r.EnqueueWithMaxAttempts(ctx, accountID, kind, payload, 5)
}

// EnqueueWithMaxAttempts inserts work with an explicit retry budget.
func (r *JobRepo) EnqueueWithMaxAttempts(
	ctx context.Context,
	accountID *uuid.UUID,
	kind string,
	payload any,
	maxAttempts int,
) (*model.Job, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	job := &model.Job{
		ID:          uuid.New(),
		AccountID:   accountID,
		Kind:        kind,
		Status:      model.JobQueued,
		MaxAttempts: maxAttempts,
		RunAt:       now,
		Payload:     raw,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if _, err := r.db.NewInsert().Model(job).Exec(ctx); err != nil {
		return nil, err
	}
	return job, nil
}

// EnqueueUniqueDomain inserts domain work unless the same kind/domain is
// already queued or running.
func (r *JobRepo) EnqueueUniqueDomain(
	ctx context.Context,
	accountID uuid.UUID,
	kind string,
	domainID uuid.UUID,
	maxAttempts int,
) (bool, error) {
	payload, err := json.Marshal(map[string]uuid.UUID{"domain_id": domainID})
	if err != nil {
		return false, err
	}
	result, err := r.db.NewRaw(`
		INSERT INTO jobs (
			id, account_id, kind, status, attempts, max_attempts,
			run_at, payload, created_at, updated_at
		)
		VALUES (?, ?, ?, ?, 0, ?, now(), ?::jsonb, now(), now())
		ON CONFLICT DO NOTHING`,
		uuid.New(),
		accountID,
		kind,
		model.JobQueued,
		maxAttempts,
		string(payload),
	).Exec(ctx)
	if err != nil {
		return false, err
	}
	count, err := result.RowsAffected()
	return count == 1, err
}

// EnqueueUniqueSite inserts periodic work unless an equivalent site/kind job is
// already queued or running.
func (r *JobRepo) EnqueueUniqueSite(
	ctx context.Context,
	accountID uuid.UUID,
	kind string,
	siteID uuid.UUID,
) (bool, error) {
	payload, err := json.Marshal(map[string]uuid.UUID{"site_id": siteID})
	if err != nil {
		return false, err
	}
	result, err := r.db.NewRaw(`
		INSERT INTO jobs (
			id, account_id, kind, status, attempts, max_attempts,
			run_at, payload, created_at, updated_at
		)
		VALUES (?, ?, ?, ?, 0, 5, now(), ?::jsonb, now(), now())
		ON CONFLICT DO NOTHING`,
		uuid.New(),
		accountID,
		kind,
		model.JobQueued,
		string(payload),
	).Exec(ctx)
	if err != nil {
		return false, err
	}
	count, err := result.RowsAffected()
	return count == 1, err
}

// Claim atomically leases one due job. Concurrent workers skip rows already
// locked by another worker.
func (r *JobRepo) Claim(ctx context.Context, workerID string) (*model.Job, error) {
	job := new(model.Job)
	err := r.db.NewRaw(`
		WITH candidate AS (
			SELECT id
			FROM jobs
			WHERE status = ?
			  AND run_at <= now()
			ORDER BY run_at ASC, created_at ASC
			FOR UPDATE SKIP LOCKED
			LIMIT 1
		)
		UPDATE jobs AS j
		SET status = ?,
		    attempts = j.attempts + 1,
		    locked_at = now(),
		    locked_by = ?,
		    updated_at = now()
		FROM candidate
		WHERE j.id = candidate.id
		RETURNING j.*`,
		model.JobQueued,
		model.JobRunning,
		workerID,
	).Scan(ctx, job)
	if err != nil {
		return nil, err
	}
	return job, nil
}

// Complete marks a running job successful.
func (r *JobRepo) Complete(ctx context.Context, id uuid.UUID, workerID string) error {
	result, err := r.db.NewUpdate().
		Model((*model.Job)(nil)).
		Set("status = ?", model.JobSucceeded).
		Set("locked_at = NULL").
		Set("locked_by = NULL").
		Set("last_error = NULL").
		Set("updated_at = now()").
		Where("id = ?", id).
		Where("status = ?", model.JobRunning).
		Where("locked_by = ?", workerID).
		Exec(ctx)
	if err != nil {
		return err
	}
	return requireOneRow(result)
}

// Retry returns a failed attempt to the queue after backoff.
func (r *JobRepo) Retry(ctx context.Context, id uuid.UUID, workerID, safeError string, runAt time.Time) error {
	result, err := r.db.NewUpdate().
		Model((*model.Job)(nil)).
		Set("status = ?", model.JobQueued).
		Set("run_at = ?", runAt).
		Set("locked_at = NULL").
		Set("locked_by = NULL").
		Set("last_error = ?", safeError).
		Set("updated_at = now()").
		Where("id = ?", id).
		Where("status = ?", model.JobRunning).
		Where("locked_by = ?", workerID).
		Exec(ctx)
	if err != nil {
		return err
	}
	return requireOneRow(result)
}

// Fail marks an exhausted job permanently failed.
func (r *JobRepo) Fail(ctx context.Context, id uuid.UUID, workerID, safeError string) error {
	result, err := r.db.NewUpdate().
		Model((*model.Job)(nil)).
		Set("status = ?", model.JobFailed).
		Set("locked_at = NULL").
		Set("locked_by = NULL").
		Set("last_error = ?", safeError).
		Set("updated_at = now()").
		Where("id = ?", id).
		Where("status = ?", model.JobRunning).
		Where("locked_by = ?", workerID).
		Exec(ctx)
	if err != nil {
		return err
	}
	return requireOneRow(result)
}

// ReapStale requeues leases abandoned by crashed workers.
func (r *JobRepo) ReapStale(ctx context.Context, before time.Time) (int64, error) {
	result, err := r.db.NewUpdate().
		Model((*model.Job)(nil)).
		Set("status = ?", model.JobQueued).
		Set("run_at = now()").
		Set("locked_at = NULL").
		Set("locked_by = NULL").
		Set("last_error = ?", "worker lease expired").
		Set("updated_at = now()").
		Where("status = ?", model.JobRunning).
		Where("locked_at < ?", before).
		Exec(ctx)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func requireOneRow(result sql.Result) error {
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return sql.ErrNoRows
	}
	return nil
}
