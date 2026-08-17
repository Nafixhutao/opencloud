package queue_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
	"go.uber.org/zap"

	"github.com/nazxf/opencloud/backend/internal/database"
	"github.com/nazxf/opencloud/backend/internal/model"
	"github.com/nazxf/opencloud/backend/internal/provisioner"
	"github.com/nazxf/opencloud/backend/internal/queue"
	"github.com/nazxf/opencloud/backend/internal/repository"
)

func openTestDB(t *testing.T) *bun.DB {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set; skipping integration tests")
	}
	db, err := database.Connect(context.Background(), dsn)
	if err != nil {
		t.Skipf("database unavailable: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// cleanupAccount removes a test account together with every queue-owned row
// it created. Jobs, buckets, and objects do not cascade from accounts, and
// leftover queued/running jobs poison later Claim-based test runs, so each
// integration test must clean all of its rows.
func cleanupAccount(t *testing.T, db *bun.DB, ctx context.Context, account *model.Account) {
	t.Helper()
	_, _ = db.NewDelete().Model((*model.StorageObject)(nil)).Where("account_id = ?", account.ID).Exec(ctx)
	_, _ = db.NewDelete().Model((*model.StorageBucket)(nil)).Where("account_id = ?", account.ID).Exec(ctx)
	_, _ = db.NewDelete().Model((*model.Job)(nil)).Where("account_id = ?", account.ID).Exec(ctx)
	_, _ = db.NewDelete().Model(account).Where("id = ?", account.ID).Exec(ctx)
}

// claimSpecificJob transitions exactly one job into the state the runner
// hands to Handle. Unlike JobRepo.Claim it cannot steal a queued job from
// another test's account when a shared integration database carries residue.
func claimSpecificJob(t *testing.T, db *bun.DB, ctx context.Context, jobID uuid.UUID) *model.Job {
	t.Helper()
	_, err := db.NewUpdate().Model((*model.Job)(nil)).
		Set("status = ?", model.JobRunning).
		Set("attempts = attempts + 1").
		Set("locked_by = ?", "worker-1").
		Set("locked_at = now()").
		Where("id = ?", jobID).
		Exec(ctx)
	require.NoError(t, err)
	job := new(model.Job)
	require.NoError(t, db.NewSelect().Model(job).Where("id = ?", jobID).Scan(ctx))
	return job
}

// claimedJob builds a job in the exact state the production runner hands to
// StorageJobHandlers.Handle: claimed (running, locked, attempts=1). Handle's
// completion path rejects anything else, so tests must not pass raw queued
// or fabricated-never-inserted jobs.
func claimedJob(accountID uuid.UUID, kind string, payload []byte, maxAttempts int) *model.Job {
	worker := "worker-1"
	now := time.Now().UTC()
	return &model.Job{
		ID:          uuid.New(),
		AccountID:   &accountID,
		Kind:        kind,
		Status:      model.JobRunning,
		Attempts:    1,
		MaxAttempts: maxAttempts,
		LockedBy:    &worker,
		LockedAt:    &now,
		Payload:     payload,
		RunAt:       now,
	}
}

func TestStorageJobReconcileStaleCreatingBucket(t *testing.T) {

	db := openTestDB(t)
	ctx := context.Background()

	acctRepo := repository.NewAccountRepo(db)
	projectRepo := repository.NewProjectRepo(db)
	jobsRepo := repository.NewJobRepo(db)
	bucketRepo := repository.NewStorageBucketRepo(db)
	auditRepo := repository.NewAuditRepo(db)

	account, err := acctRepo.CreateAccount(ctx, "test-account")
	require.NoError(t, err)
	defer cleanupAccount(t, db, ctx, account)

	project := &model.Project{
		ID:        uuid.New(),
		AccountID: account.ID,
		Name:      "test-project",
		Status:    model.ProjectActive,
	}
	require.NoError(t, projectRepo.CreateProject(ctx, project))
	defer func() { _, _ = db.NewDelete().Model(project).Where("id = ?", project.ID).Exec(ctx) }()

	bucketID := uuid.New()
	staleBucket := &model.StorageBucket{
		ID:                 bucketID,
		AccountID:          account.ID,
		ProjectID:          project.ID,
		Name:               "stale-creating-bucket",
		PhysicalName:       "ocb-" + bucketID.String(),
		Status:             model.BucketCreating,
		ObjectCount:        0,
		StorageLimitBytes:  1073741824,
		MaxObjectSizeBytes: 104857600,
		CreatedAt:          time.Now().UTC().Add(-2 * time.Hour),
	}
	require.NoError(t, bucketRepo.Create(ctx, staleBucket))

	log, _ := zap.NewDevelopment()
	fakeProvider := provisioner.NewFakeStorageProvider()
	handlers := queue.NewStorageJobHandlers(log, db, bucketRepo, jobsRepo, auditRepo, fakeProvider)

	payload := model.ReconcileStorageBucketPayload{BucketID: bucketID}
	enqueued, err := jobsRepo.EnqueueWithMaxAttempts(ctx, &account.ID, model.JobReconcileStorageBucket, payload, 5)
	require.NoError(t, err)

	// The runner only ever hands Handle a claimed job.
	claimed := claimSpecificJob(t, db, ctx, enqueued.ID)
	require.Equal(t, model.JobReconcileStorageBucket, claimed.Kind)

	err = handlers.Handle(ctx, claimed, "worker-1")
	require.NoError(t, err)

	result, err := bucketRepo.GetByAccount(ctx, account.ID, bucketID)
	require.NoError(t, err)
	require.Equal(t, model.BucketFailed, result.Status)
}

func TestStorageJobDeleteBlockedRestoreToActive(t *testing.T) {

	db := openTestDB(t)
	ctx := context.Background()

	acctRepo := repository.NewAccountRepo(db)
	projectRepo := repository.NewProjectRepo(db)
	jobsRepo := repository.NewJobRepo(db)
	bucketRepo := repository.NewStorageBucketRepo(db)
	auditRepo := repository.NewAuditRepo(db)

	account, err := acctRepo.CreateAccount(ctx, "test-account-2")
	require.NoError(t, err)
	defer cleanupAccount(t, db, ctx, account)

	project := &model.Project{
		ID:        uuid.New(),
		AccountID: account.ID,
		Name:      "test-project-2",
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
	fakeProvider.SetBucketsForTest(map[string]*provisioner.FakeBucketState{
		bucket.PhysicalName: {
			PhysicalName: bucket.PhysicalName,
			Visibility:   "private",
			Objects: map[string]*provisioner.FakeObjectState{
				"obj1": {Key: "obj1", Data: []byte("x")},
			},
		},
	})
	handlers := queue.NewStorageJobHandlers(log, db, bucketRepo, jobsRepo, auditRepo, fakeProvider)

	payload := model.DeleteStorageBucketPayload{BucketID: bucketID}
	enqueued, err := jobsRepo.EnqueueWithMaxAttempts(ctx, &account.ID, model.JobDeleteStorageBucket, payload, 5)
	require.NoError(t, err)

	claimed := claimSpecificJob(t, db, ctx, enqueued.ID)
	require.Equal(t, model.JobDeleteStorageBucket, claimed.Kind)

	err = handlers.Handle(ctx, claimed, "worker-1")
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
