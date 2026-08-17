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

	db := openTestDB(t)
	ctx := context.Background()

	acctRepo := repository.NewAccountRepo(db)
	projectRepo := repository.NewProjectRepo(db)
	jobsRepo := repository.NewJobRepo(db)
	bucketRepo := repository.NewStorageBucketRepo(db)

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
		// The subtest's premise: an in-flight provision job for this bucket
		// must keep the creating bucket waiting during reconciliation.
		provisionPayload, _ := json.Marshal(model.ProvisionStorageBucketPayload{BucketID: bucketID})
		provisionJob := &model.Job{
			ID:          uuid.New(),
			AccountID:   &account.ID,
			Kind:        model.JobProvisionStorageBucket,
			MaxAttempts: 5,
			Status:      model.JobQueued,
			Payload:     provisionPayload,
			RunAt:       time.Now().UTC(),
		}
		_, err = db.NewInsert().Model(provisionJob).Exec(ctx)
		require.NoError(t, err)

		payload := model.ReconcileStorageBucketPayload{BucketID: bucketID}
		rawPayload, _ := json.Marshal(payload)
		job := claimedJob(account.ID, model.JobReconcileStorageBucket, rawPayload, 5)
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
		// The subtest's premise: an in-flight provision job for this bucket
		// must keep the creating bucket waiting during reconciliation.
		provisionPayload, _ := json.Marshal(model.ProvisionStorageBucketPayload{BucketID: bucketID})
		provisionJob := &model.Job{
			ID:          uuid.New(),
			AccountID:   &account.ID,
			Kind:        model.JobProvisionStorageBucket,
			MaxAttempts: 5,
			Status:      model.JobRunning,
			Payload:     provisionPayload,
			RunAt:       time.Now().UTC(),
		}
		_, err = db.NewInsert().Model(provisionJob).Exec(ctx)
		require.NoError(t, err)

		payload := model.ReconcileStorageBucketPayload{BucketID: bucketID}
		rawPayload, _ := json.Marshal(payload)
		job := claimedJob(account.ID, model.JobReconcileStorageBucket, rawPayload, 5)
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
		job := claimedJob(account.ID, model.JobReconcileStorageBucket, rawPayload, 5)
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
		// The subtest's premise: an in-flight provision job for this bucket
		// must keep the creating bucket waiting during reconciliation.
		provisionPayload, _ := json.Marshal(model.ProvisionStorageBucketPayload{BucketID: bucketID})
		provisionJob := &model.Job{
			ID:          uuid.New(),
			AccountID:   &account.ID,
			Kind:        model.JobProvisionStorageBucket,
			MaxAttempts: 5,
			Status:      model.JobQueued,
			Payload:     provisionPayload,
			RunAt:       time.Now().UTC().Add(time.Minute),
		}
		_, err = db.NewInsert().Model(provisionJob).Exec(ctx)
		require.NoError(t, err)

		payload := model.ReconcileStorageBucketPayload{BucketID: bucketID}
		rawPayload, _ := json.Marshal(payload)
		job := claimedJob(account.ID, model.JobReconcileStorageBucket, rawPayload, 5)
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
		fakeProvider.SetBucketsForTest(map[string]*provisioner.FakeBucketState{
			physicalName: {PhysicalName: physicalName, Visibility: "private", Objects: map[string]*provisioner.FakeObjectState{}},
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
		job := claimedJob(account.ID, model.JobReconcileStorageBucket, rawPayload, 5)
		_, err = db.NewInsert().Model(job).Exec(ctx)
		require.NoError(t, err)

		err = handlers.Handle(ctx, job, "worker-1")
		require.NoError(t, err)

		result, err := bucketRepo.GetByAccount(ctx, account.ID, bucketID)
		require.NoError(t, err)
		require.Equal(t, model.BucketActive, result.Status)
	})

	t.Run("status-guard-prevents-race", func(t *testing.T) {
		bucketID := uuid.New()
		physicalName := "ocb-" + bucketID.String()

		bucket := &model.StorageBucket{
			ID:                 bucketID,
			AccountID:          account.ID,
			ProjectID:          project.ID,
			Name:               "race-bucket",
			PhysicalName:       physicalName,
			Status:             model.BucketDeleting,
			ObjectCount:        0,
			StorageLimitBytes:  1073741824,
			MaxObjectSizeBytes: 104857600,
			AllowedMimeTypes:   []byte("[]"),
		}
		require.NoError(t, bucketRepo.Create(ctx, bucket))

		payload := model.ProvisionStorageBucketPayload{BucketID: bucketID}
		rawPayload, _ := json.Marshal(payload)
		job := claimedJob(account.ID, model.JobProvisionStorageBucket, rawPayload, 3)
		_, err = db.NewInsert().Model(job).Exec(ctx)
		require.NoError(t, err)

		err = handlers.Handle(ctx, job, "worker-1")
		// Deletion took precedence: the provision path exits silently and
		// leaves the deleting state untouched.
		require.NoError(t, err)

		result, err := bucketRepo.GetByAccount(ctx, account.ID, bucketID)
		require.NoError(t, err)
		require.Equal(t, model.BucketDeleting, result.Status)
	})
}
