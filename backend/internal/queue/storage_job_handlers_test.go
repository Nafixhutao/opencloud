package queue_test

import (
	"context"
	"encoding/json"
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

func TestStorageJobProvisionReconciliation(t *testing.T) {
	t.Parallel()

	db := openTestDB(t)
	ctx := context.Background()

	acctRepo := repository.NewAccountRepo(db)
	projectRepo := repository.NewProjectRepo(db)
	jobsRepo := repository.NewJobRepo(db)
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

	log, _ := zap.NewDevelopment()
	fakeProvider := provisioner.NewFakeStorageProvider()
	handlers := queue.NewStorageJobHandlers(log, db, bucketRepo, jobsRepo, nil, fakeProvider)

	t.Run("queued-provision-job-waits", func(t *testing.T) {
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
		_, err = db.NewInsert().Model(job).Exec(ctx)
		require.NoError(t, err)

		err = handlers.Handle(ctx, job, "worker-1")
		require.NoError(t, err)

		result, err := bucketRepo.GetByAccount(ctx, account.ID, bucketID)
		require.NoError(t, err)
		require.Equal(t, model.BucketCreating, result.Status)
	})

	t.Run("running-provision-job-waits", func(t *testing.T) {
		bucketID := uuid.New()
		bucket := &model.StorageBucket{
			ID:                 bucketID,
			AccountID:          account.ID,
			ProjectID:          project.ID,
			Name:               "test-bucket-2",
			PhysicalName:       "ocb-" + bucketID.String(),
			Status:             model.BucketCreating,
			ObjectCount:        0,
			StorageLimitBytes:  1073741824,
			MaxObjectSizeBytes: 104857600,
			AllowedMimeTypes:   []byte("[]"),
		}
		require.NoError(t, bucketRepo.Create(ctx, bucket))

		payload := model.ReconcileStorageBucketPayload{BucketID: bucketID}
		rawPayload, _ := json.Marshal(payload)
		job := &model.Job{
			ID:        uuid.New(),
			AccountID: &account.ID,
			Kind:      model.JobReconcileStorageBucket,
			Status:    model.JobRunning,
			Payload:   rawPayload,
			RunAt:     time.Now().UTC(),
		}
		_, err = db.NewInsert().Model(job).Exec(ctx)
		require.NoError(t, err)

		err = handlers.Handle(ctx, job, "worker-1")
		require.NoError(t, err)

		result, err := bucketRepo.GetByAccount(ctx, account.ID, bucketID)
		require.NoError(t, err)
		require.Equal(t, model.BucketCreating, result.Status)
	})

	t.Run("failed-job-no-active-job-transitions-to-failed", func(t *testing.T) {
		bucketID := uuid.New()
		bucket := &model.StorageBucket{
			ID:                 bucketID,
			AccountID:          account.ID,
			ProjectID:          project.ID,
			Name:               "stale-bucket",
			PhysicalName:       "ocb-" + bucketID.String(),
			Status:             model.BucketCreating,
			ObjectCount:        0,
			StorageLimitBytes:  1073741824,
			MaxObjectSizeBytes: 104857600,
			AllowedMimeTypes:   []byte("[]"),
		}
		require.NoError(t, bucketRepo.Create(ctx, bucket))

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
		_, err = db.NewInsert().Model(job).Exec(ctx)
		require.NoError(t, err)

		err = handlers.Handle(ctx, job, "worker-1")
		require.NoError(t, err)

		result, err := bucketRepo.GetByAccount(ctx, account.ID, bucketID)
		require.NoError(t, err)
		require.Equal(t, model.BucketFailed, result.Status)
	})

	t.Run("requeued-provision-job-waits", func(t *testing.T) {
		bucketID := uuid.New()
		bucket := &model.StorageBucket{
			ID:                 bucketID,
			AccountID:          account.ID,
			ProjectID:          project.ID,
			Name:               "requeued-bucket",
			PhysicalName:       "ocb-" + bucketID.String(),
			Status:             model.BucketCreating,
			ObjectCount:        0,
			StorageLimitBytes:  1073741824,
			MaxObjectSizeBytes: 104857600,
			AllowedMimeTypes:   []byte("[]"),
		}
		require.NoError(t, bucketRepo.Create(ctx, bucket))

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
		_, err = db.NewInsert().Model(job).Exec(ctx)
		require.NoError(t, err)

		err = handlers.Handle(ctx, job, "worker-1")
		require.NoError(t, err)

		result, err := bucketRepo.GetByAccount(ctx, account.ID, bucketID)
		require.NoError(t, err)
		require.Equal(t, model.BucketCreating, result.Status)
	})

	t.Run("provider-exists-converge-active", func(t *testing.T) {
		bucketID := uuid.New()
		physicalName := "ocb-" + bucketID.String()
		fakeProvider.SetBucketsForTest(map[string]*provisioner.BucketState{
			physicalName: {PhysicalName: physicalName, Visibility: "private", HasObjects: false},
		})
		bucket := &model.StorageBucket{
			ID:                 bucketID,
			AccountID:          account.ID,
			ProjectID:          project.ID,
			Name:               "existing-bucket",
			PhysicalName:       physicalName,
			Status:             model.BucketCreating,
			ObjectCount:        0,
			StorageLimitBytes:  1073741824,
			MaxObjectSizeBytes: 104857600,
			AllowedMimeTypes:   []byte("[]"),
		}
		require.NoError(t, bucketRepo.Create(ctx, bucket))

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
		_, err = db.NewInsert().Model(job).Exec(ctx)
		require.NoError(t, err)

		err = handlers.Handle(ctx, job, "worker-1")
		require.NoError(t, err)

		result, err := bucketRepo.GetByAccount(ctx, account.ID, bucketID)
		require.NoError(t, err)
		require.Equal(t, model.BucketActive, result.Status)
	})
}
