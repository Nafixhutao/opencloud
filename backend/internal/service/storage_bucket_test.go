package service_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/nazxf/opencloud/backend/internal/apperr"
	"github.com/nazxf/opencloud/backend/internal/model"
	"github.com/nazxf/opencloud/backend/internal/repository"
	service "github.com/nazxf/opencloud/backend/internal/service"
)

func TestStorageBucketServiceCreateTenantIsolation(t *testing.T) {
	t.Parallel()

	db := openTestDB(t)
	ctx := context.Background()

	acctRepo := repository.NewAccountRepo(db)
	projectRepo := repository.NewProjectRepo(db)
	bucketRepo := repository.NewStorageBucketRepo(db)
	jobRepo := repository.NewJobRepo(db)
	auditRepo := repository.NewAuditRepo(db)

	svc := service.NewStorageBucketService(db, bucketRepo, projectRepo, jobRepo, auditRepo)

	account1, err := acctRepo.CreateAccount(ctx, "account-1")
	require.NoError(t, err)
	defer func() { _, _ = db.NewDelete().Model(account1).Where("id = ?", account1.ID).Exec(ctx) }()

	account2, err := acctRepo.CreateAccount(ctx, "account-2")
	require.NoError(t, err)
	defer func() { _, _ = db.NewDelete().Model(account2).Where("id = ?", account2.ID).Exec(ctx) }()

	project1 := &model.Project{
		ID:        uuid.New(),
		AccountID: account1.ID,
		Name:      "project-1",
		Status:    model.ProjectActive,
	}
	require.NoError(t, projectRepo.CreateProject(ctx, project1))
	defer func() { _, _ = db.NewDelete().Model(project1).Where("id = ?", project1.ID).Exec(ctx) }()

	userID := "test-user"
	_, err = svc.CreateBucket(ctx, userID, account2.ID, project1.ID, "unique-key", service.CreateBucketRequest{
		Name: "test-bucket",
	})
	var ae *apperr.Error
	require.ErrorAs(t, err, &ae)
	require.Equal(t, "FORBIDDEN", ae.Code)
	require.Contains(t, ae.Message, "permission denied")
}

func TestStorageBucketServiceIdempotencySameKey(t *testing.T) {
	t.Parallel()

	db := openTestDB(t)
	ctx := context.Background()

	acctRepo := repository.NewAccountRepo(db)
	projectRepo := repository.NewProjectRepo(db)
	bucketRepo := repository.NewStorageBucketRepo(db)
	jobRepo := repository.NewJobRepo(db)
	auditRepo := repository.NewAuditRepo(db)

	svc := service.NewStorageBucketService(db, bucketRepo, projectRepo, jobRepo, auditRepo)

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

	userID := "test-user"
	idempotencyKey := "same-idempotency-key"

	bucket1, err := svc.CreateBucket(ctx, userID, account.ID, project.ID, idempotencyKey, service.CreateBucketRequest{
		Name: "first-bucket",
	})
	require.NoError(t, err)
	require.NotNil(t, bucket1)

	bucket2, err := svc.CreateBucket(ctx, userID, account.ID, project.ID, idempotencyKey, service.CreateBucketRequest{
		Name: "second-bucket-same-key",
	})
	require.NoError(t, err)
	require.NotNil(t, bucket2)

	require.Equal(t, bucket1.ID, bucket2.ID)
}
