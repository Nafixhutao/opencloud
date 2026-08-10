package queue_test

import (
	"context"
	"encoding/json"
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

func TestStorageJobReconcileStaleCreatingBucket(t *testing.T) {
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
	rawPayload, _ := json.Marshal(payload)
	job := &model.Job{
		ID:        uuid.New(),
		AccountID: &account.ID,
		Kind:      model.JobReconcileStorageBucket,
		Status:    model.JobQueued,
		Payload:   rawPayload,
		RunAt:     time.Now().UTC(),
	}
	_, err = jobsRepo.EnqueueWithMaxAttempts(ctx, &account.ID, model.JobReconcileStorageBucket, payload, 5)
	require.NoError(t, err)

	err = handlers.Handle(ctx, job, "worker-1")
	require.NoError(t, err)

	result, err := bucketRepo.GetByAccount(ctx, account.ID, bucketID)
	require.NoError(t, err)
	require.Equal(t, model.BucketFailed, result.Status)
}

func TestStorageJobDeleteBlockedRestoreToActive(t *testing.T) {
	t.Parallel()

	db := openTestDB(t)
	ctx := context.Background()

	acctRepo := repository.NewAccountRepo(db)
	projectRepo := repository.NewProjectRepo(db)
	jobsRepo := repository.NewJobRepo(db)
	bucketRepo := repository.NewStorageBucketRepo(db)
	auditRepo := repository.NewAuditRepo(db)

	account, err := acctRepo.CreateAccount(ctx, "test-account-2")
	require.NoError(t, err)
	defer func() { _, _ = db.NewDelete().Model(account).Where("id = ?", account.ID).Exec(ctx) }()

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
	fakeProvider.SetBucketsForTest(map[string]*provisioner.BucketState{
		bucket.PhysicalName: {
			PhysicalName: bucket.PhysicalName,
			Visibility:   "private",
			HasObjects:   true,
			ObjectCount:  1,
		},
	})
	handlers := queue.NewStorageJobHandlers(log, db, bucketRepo, jobsRepo, auditRepo, fakeProvider)

	payload := model.DeleteStorageBucketPayload{BucketID: bucketID}
	rawPayload, _ := json.Marshal(payload)
	job := &model.Job{
		ID:        uuid.New(),
		AccountID: &account.ID,
		Kind:      model.JobDeleteStorageBucket,
		Status:    model.JobRunning,
		Payload:   rawPayload,
		RunAt:     time.Now().UTC(),
	}
	_, err = jobsRepo.EnqueueWithMaxAttempts(ctx, &account.ID, model.JobDeleteStorageBucket, payload, 5)
	require.NoError(t, err)

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
