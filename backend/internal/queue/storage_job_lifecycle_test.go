package queue_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/nazxf/opencloud/backend/internal/model"
	"github.com/nazxf/opencloud/backend/internal/provisioner"
	"github.com/nazxf/opencloud/backend/internal/queue"
	"github.com/nazxf/opencloud/backend/internal/repository"
)

// TestStorageJobClaimIncrementsAttemptsExactlyOnce verifies that Claim atomically
// increments attempts count exactly once when leasing a queued job.
func TestStorageJobClaimIncrementsAttemptsExactlyOnce(t *testing.T) {
	t.Parallel()

	db := openTestDB(t)
	ctx := context.Background()

	jobsRepo := repository.NewJobRepo(db)

	account, err := repository.NewAccountRepo(db).CreateAccount(ctx, "test-account")
	require.NoError(t, err)
	defer func() { _, _ = db.NewDelete().Model(account).Where("id = ?", account.ID).Exec(ctx) }()

	payload := model.ProvisionStorageBucketPayload{BucketID: uuid.New()}
	job, err := jobsRepo.EnqueueWithMaxAttempts(ctx, &account.ID, model.JobProvisionStorageBucket, payload, 5)
	require.NoError(t, err)
	require.NotNil(t, job)

	// Verify initial state
	claims := make([]*model.Job, 0)
	err = db.NewSelect().Model((*model.Job)(nil)).Where("id = ?", job.ID).Scan(ctx, &claims)
	require.NoError(t, err)
	require.Len(t, claims, 1)
	require.Equal(t, int64(0), claims[0].Attempts)

	// Claim the job
	claimed, err := jobsRepo.Claim(ctx, "worker-1")
	require.NoError(t, err)
	require.NotNil(t, claimed)
	require.Equal(t, job.ID, claimed.ID)
	require.Equal(t, model.JobRunning, claimed.Status)
	require.Equal(t, int64(1), claimed.Attempts)

	// Claim again with different worker - should get no rows since job is already claimed
	claimed2, err := jobsRepo.Claim(ctx, "worker-2")
	require.ErrorIs(t, err, sql.ErrNoRows)
	require.Nil(t, claimed2)

	// Verify job still has attempts=1 in DB
	err = db.NewSelect().Model((*model.Job)(nil)).Where("id = ?", job.ID).Where("status = ?", model.JobRunning).Column("attempts").Scan(ctx, &claims)
	require.NoError(t, err)
	require.Len(t, claims, 1)
	require.Equal(t, int64(1), claims[0].Attempts)
}

// TestStorageJobExhaustedRetriesTerminalFailure verifies that max retry exhaustion
// results in terminal failure state for both job and resource.
func TestStorageJobExhaustedRetriesTerminalFailure(t *testing.T) {
	t.Parallel()

	db := openTestDB(t)
	ctx := context.Background()

	acctRepo := repository.NewAccountRepo(db)
	projectRepo := repository.NewProjectRepo(db)
	jobsRepo := repository.NewJobRepo(db)
	bucketRepo := repository.NewStorageBucketRepo(db)
	auditRepo := repository.NewAuditRepo(db)

	account, err := acctRepo.CreateAccount(ctx, "test-account")
	require.NoError(t, err)
	defer func() { _, _ = db.NewDelete().Model(account).Where("id = ?", account.ID).Exec(ctx) }()

	project := &model.Project{
		ID:        uuid.New(),
		AccountID: account.ID,
		Name:      "test-project",
		Status:    model.ProjectActive,
	}
	require.NoError(t, projectRepo.CreateProject(ctx, project))
	defer func() { _, _ = db.NewDelete().Model(project).Where("id = ?", project.ID).Exec(ctx) }()

	bucketID := uuid.New()
	bucket := &model.StorageBucket{
		ID:                 bucketID,
		AccountID:          account.ID,
		ProjectID:          project.ID,
		Name:               "test-bucket",
		PhysicalName:       "ocb-" + bucketID.String(),
		Status:             model.BucketCreating,
		ObjectCount:        0,
		StorageLimitBytes:  1073741824,
		MaxObjectSizeBytes: 104857600,
		AllowedMimeTypes:   []byte("[]"),
	}
	require.NoError(t, bucketRepo.Create(ctx, bucket))

	// Always-fail provider for exhaustion test
	alwaysFailProvider := &alwaysFailProvider{}
	log, _ := zap.NewDevelopment()
	handlers := queue.NewStorageJobHandlers(log, db, bucketRepo, jobsRepo, auditRepo, alwaysFailProvider)

	payload := model.ProvisionStorageBucketPayload{BucketID: bucketID}
	_, err = jobsRepo.EnqueueWithMaxAttempts(ctx, &account.ID, model.JobProvisionStorageBucket, payload, 2)
	require.NoError(t, err)

	// First claim+handle: fails
	claimed, err := jobsRepo.Claim(ctx, "worker-1")
	require.NoError(t, err)
	err = handlers.Handle(ctx, claimed, "worker-1")
	require.Error(t, err)

	// Trigger Retry manually (simulating Runner behavior on transient error)
	runAt := time.Now().UTC().Add(1 * time.Second)
	err = jobsRepo.Retry(ctx, claimed.ID, "worker-1", "provision failure", runAt)
	require.NoError(t, err)

	// Second claim+handle: fails again
	claimed, err = jobsRepo.Claim(ctx, "worker-1")
	require.NoError(t, err)
	err = handlers.Handle(ctx, claimed, "worker-1")
	require.Error(t, err)

	// After max attempts exhausted, bucket should be FAILED
	result, err := bucketRepo.GetByAccount(ctx, account.ID, bucketID)
	require.NoError(t, err)
	require.Equal(t, model.BucketFailed, result.Status)

	// Verify job status is also FAILED
	var jobStatus string
	err = db.NewSelect().Model((*model.Job)(nil)).Where("id = ?", claimed.ID).Column("status").Scan(ctx, &jobStatus)
	require.NoError(t, err)
	require.Equal(t, model.JobFailed, jobStatus)
}

// TestBucketNotEmptyNilCount verifies that BucketNotEmptyError{Count:nil} still
// blocks deletion and returns safe last_error message.
func TestBucketNotEmptyNilCount(t *testing.T) {
	t.Parallel()

	db := openTestDB(t)
	ctx := context.Background()

	acctRepo := repository.NewAccountRepo(db)
	projectRepo := repository.NewProjectRepo(db)
	jobsRepo := repository.NewJobRepo(db)
	bucketRepo := repository.NewStorageBucketRepo(db)
	auditRepo := repository.NewAuditRepo(db)

	account, err := acctRepo.CreateAccount(ctx, "test-account")
	require.NoError(t, err)
	defer func() { _, _ = db.NewDelete().Model(account).Where("id = ?", account.ID).Exec(ctx) }()

	project := &model.Project{
		ID:        uuid.New(),
		AccountID: account.ID,
		Name:      "test-project",
		Status:    model.ProjectActive,
	}
	require.NoError(t, projectRepo.CreateProject(ctx, project))
	defer func() { _, _ = db.NewDelete().Model(project).Where("id = ?", project.ID).Exec(ctx) }()

	bucketID := uuid.New()
	bucket := &model.StorageBucket{
		ID:                 bucketID,
		AccountID:          account.ID,
		ProjectID:          project.ID,
		Name:               "test-bucket",
		PhysicalName:       "ocb-" + bucketID.String(),
		Status:             model.BucketDeleting,
		ObjectCount:        0,
		StorageLimitBytes:  1073741824,
		MaxObjectSizeBytes: 104857600,
	}
	require.NoError(t, bucketRepo.Create(ctx, bucket))

	log, _ := zap.NewDevelopment()
	fakeProvider := provisioner.NewFakeStorageProvider()
	fakeProvider.SetBucketsForTest(map[string]*provisioner.BucketState{
		bucket.PhysicalName: {
			PhysicalName: bucket.PhysicalName,
			Visibility:   "private",
			HasObjects:   true,
			ObjectCount:  0, // Set to 0 but HasObjects=true forces nil Count
		},
	})
	handlers := queue.NewStorageJobHandlers(log, db, bucketRepo, jobsRepo, auditRepo, fakeProvider)

	payload := model.DeleteStorageBucketPayload{BucketID: bucketID}
	_, err = jobsRepo.EnqueueWithMaxAttempts(ctx, &account.ID, model.JobDeleteStorageBucket, payload, 5)
	require.NoError(t, err)

	// Get the job we enqueued
	allJobs := make([]*model.Job, 0)
	err = db.NewSelect().Model((*model.Job)(nil)).Where("account_id = ? AND kind = ?", account.ID, model.JobDeleteStorageBucket).Order("id ASC").Limit(1).Scan(ctx, &allJobs)
	require.NoError(t, err)
	require.Len(t, allJobs, 1)
	job := allJobs[0]

	err = handlers.Handle(ctx, job, "worker-1")
	require.NoError(t, err)

	result, err := bucketRepo.GetByAccount(ctx, account.ID, bucketID)
	require.NoError(t, err)
	require.Equal(t, model.BucketActive, result.Status)
	require.NotNil(t, result.LastError)
	require.Contains(t, *result.LastError, "BUCKET_NOT_EMPTY")

	var count int
	count, err = db.NewSelect().Model((*model.AuditLog)(nil)).
		Where("account_id = ? AND action = ?", account.ID, model.AuditStorageBucketDeleteBlocked).
		Count(ctx)
	require.NoError(t, err)
	require.Greater(t, count, 0)
}

// TestIdempotentPhysicalNameStability verifies that same project-scoped idempotency key
// returns existing bucket with unchanged physical_name on replay.
func TestIdempotentPhysicalNameStability(t *testing.T) {
	t.Parallel()

	db := openTestDB(t)
	ctx := context.Background()

	acctRepo := repository.NewAccountRepo(db)
	projectRepo := repository.NewProjectRepo(db)
	bucketRepo := repository.NewStorageBucketRepo(db)

	account, err := acctRepo.CreateAccount(ctx, "test-account")
	require.NoError(t, err)
	defer func() { _, _ = db.NewDelete().Model(account).Where("id = ?", account.ID).Exec(ctx) }()

	project := &model.Project{
		ID:        uuid.New(),
		AccountID: account.ID,
		Name:      "test-project",
		Status:    model.ProjectActive,
	}
	require.NoError(t, projectRepo.CreateProject(ctx, project))
	defer func() { _, _ = db.NewDelete().Model(project).Where("id = ?", project.ID).Exec(ctx) }()

	idempotencyKey := "same-idempotency-key"

	// First request creates bucket
	firstBucket := &model.StorageBucket{
		ID:             uuid.New(),
		AccountID:      account.ID,
		ProjectID:      project.ID,
		IdempotencyKey: &idempotencyKey,
		Name:           "first-request",
		PhysicalName:   "ocb-first-request-physics-name",
		Status:         model.BucketActive,
		ObjectCount:    0,
	}
	err = bucketRepo.Create(ctx, firstBucket)
	require.NoError(t, err)

	// Second request with same key should return existing bucket
	secondBucket := &model.StorageBucket{
		ID:             uuid.New(), // Different ID for new record attempt
		AccountID:      account.ID,
		ProjectID:      project.ID,
		IdempotencyKey: &idempotencyKey,
		Name:           "second-request",
		PhysicalName:   "ocb-second-request-physics-name",
		Status:         model.BucketActive,
		ObjectCount:    0,
	}
	err = bucketRepo.Create(ctx, secondBucket)
	require.Error(t, err)
	require.Contains(t, err.Error(), "UNIQUE constraint")

	// Verify physical_name remained stable
	retrieved, err := bucketRepo.GetByIDempotencyKey(ctx, account.ID, project.ID, idempotencyKey)
	require.NoError(t, err)
	require.NotNil(t, retrieved)
	require.Equal(t, "ocb-first-request-physics-name", retrieved.PhysicalName)
}

// Helper provider that always fails
type alwaysFailProvider struct{}

func (a *alwaysFailProvider) CreateBucket(ctx context.Context, spec provisioner.BucketSpec) error {
	return errors.New("permanent provision failure")
}

func (a *alwaysFailProvider) DeleteBucket(ctx context.Context, ref provisioner.BucketRef) error {
	return provisioner.ErrBucketNotFound
}

func (a *alwaysFailProvider) BucketExists(ctx context.Context, ref provisioner.BucketRef) (bool, error) {
	return false, nil
}
