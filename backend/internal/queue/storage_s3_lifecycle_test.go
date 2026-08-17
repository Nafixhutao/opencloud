package queue_test

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/nazxf/opencloud/backend/internal/model"
	"github.com/nazxf/opencloud/backend/internal/provisioner"
	"github.com/nazxf/opencloud/backend/internal/queue"
	"github.com/nazxf/opencloud/backend/internal/repository"
)

func TestS3StorageQueueLifecycle(t *testing.T) {
	endpoint := os.Getenv("STORAGE_S3_ENDPOINT")
	if endpoint == "" {
		t.Skip("STORAGE_S3_ENDPOINT not set; skipping S3 queue lifecycle test")
	}

	db := openTestDB(t)
	ctx := context.Background()

	acctRepo := repository.NewAccountRepo(db)
	projectRepo := repository.NewProjectRepo(db)
	jobsRepo := repository.NewJobRepo(db)
	bucketRepo := repository.NewStorageBucketRepo(db)
	auditRepo := repository.NewAuditRepo(db)

	account, err := acctRepo.CreateAccount(ctx, "s3-lifecycle-test")
	require.NoError(t, err)
	defer cleanupAccount(ctx, t, db, account)

	project := &model.Project{
		ID:        uuid.New(),
		AccountID: account.ID,
		Name:      "s3-lifecycle-project",
		Status:    model.ProjectActive,
	}
	require.NoError(t, projectRepo.CreateProject(ctx, project))
	defer func() { _, _ = db.NewDelete().Model(project).Where("id = ?", project.ID).Exec(ctx) }()

	s3cfg := provisioner.S3StorageConfig{
		Endpoint:        endpoint,
		Region:          os.Getenv("STORAGE_S3_REGION"),
		AccessKeyID:     os.Getenv("STORAGE_S3_ACCESS_KEY_ID"),
		SecretAccessKey: os.Getenv("STORAGE_S3_SECRET_ACCESS_KEY"),
		UsePathStyle:    true,
	}
	if s3cfg.Region == "" {
		s3cfg.Region = "us-east-1"
	}

	s3Provider, err := provisioner.NewS3StorageProvider(ctx, s3cfg)
	require.NoError(t, err)

	log, _ := zap.NewDevelopment()
	handlers := queue.NewStorageJobHandlers(log, db, bucketRepo, jobsRepo, auditRepo, s3Provider)

	t.Run("provision-success", func(t *testing.T) {
		bucketID := uuid.New()
		physicalName := "s3-lifecycle-" + bucketID.String()

		bucket := &model.StorageBucket{
			ID:                 bucketID,
			AccountID:          account.ID,
			ProjectID:          project.ID,
			Name:               "provision-test",
			PhysicalName:       physicalName,
			Status:             model.BucketCreating,
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
		require.NoError(t, err)

		result, err := bucketRepo.GetByAccount(ctx, account.ID, bucketID)
		require.NoError(t, err)
		require.Equal(t, model.BucketActive, result.Status)

		exists, err := s3Provider.BucketExists(ctx, provisioner.BucketRef{
			PhysicalName: physicalName,
		})
		require.NoError(t, err)
		require.True(t, exists)

		_ = s3Provider.DeleteBucket(ctx, provisioner.BucketRef{PhysicalName: physicalName})
	})

	t.Run("delete-success", func(t *testing.T) {
		bucketID := uuid.New()
		physicalName := "s3-lifecycle-del-" + bucketID.String()

		err := s3Provider.CreateBucket(ctx, provisioner.BucketSpec{
			PhysicalName: physicalName,
		})
		require.NoError(t, err)

		bucket := &model.StorageBucket{
			ID:                 bucketID,
			AccountID:          account.ID,
			ProjectID:          project.ID,
			Name:               "delete-test",
			PhysicalName:       physicalName,
			Status:             model.BucketDeleting,
			ObjectCount:        0,
			StorageLimitBytes:  1073741824,
			MaxObjectSizeBytes: 104857600,
			AllowedMimeTypes:   []byte("[]"),
		}
		require.NoError(t, bucketRepo.Create(ctx, bucket))

		payload := model.DeleteStorageBucketPayload{BucketID: bucketID}
		rawPayload, _ := json.Marshal(payload)
		job := claimedJob(account.ID, model.JobDeleteStorageBucket, rawPayload, 3)
		_, err = db.NewInsert().Model(job).Exec(ctx)
		require.NoError(t, err)

		err = handlers.Handle(ctx, job, "worker-1")
		require.NoError(t, err)

		exists, err := s3Provider.BucketExists(ctx, provisioner.BucketRef{
			PhysicalName: physicalName,
		})
		require.NoError(t, err)
		require.False(t, exists)
	})

	t.Run("provision-failure-unknown-endpoint", func(t *testing.T) {
		badCfg := provisioner.S3StorageConfig{
			Endpoint:        "http://127.0.0.1:1",
			Region:          "us-east-1",
			AccessKeyID:     "key",
			SecretAccessKey: "secret",
			UsePathStyle:    true,
		}
		badProvider, err := provisioner.NewS3StorageProvider(ctx, badCfg)
		require.NoError(t, err)

		badHandlers := queue.NewStorageJobHandlers(log, db, bucketRepo, jobsRepo, auditRepo, badProvider)

		bucketID := uuid.New()
		bucket := &model.StorageBucket{
			ID:                 bucketID,
			AccountID:          account.ID,
			ProjectID:          project.ID,
			Name:               "fail-test",
			PhysicalName:       "s3-fail-" + bucketID.String(),
			Status:             model.BucketCreating,
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

		err = badHandlers.Handle(ctx, job, "worker-1")
		require.Error(t, err)
		require.Contains(t, err.Error(), "provision failed")

		result, err := bucketRepo.GetByAccount(ctx, account.ID, bucketID)
		require.NoError(t, err)
		require.Equal(t, model.BucketCreating, result.Status)
	})
}
