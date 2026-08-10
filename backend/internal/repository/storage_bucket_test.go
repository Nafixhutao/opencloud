package repository_test

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"

	"github.com/nazxf/opencloud/backend/internal/database"
	"github.com/nazxf/opencloud/backend/internal/model"
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

func TestStorageBucketRepoTenantIsolation(t *testing.T) {
	t.Parallel()

	db := openTestDB(t)
	ctx := context.Background()

	acctRepo := repository.NewAccountRepo(db)
	projectRepo := repository.NewProjectRepo(db)
	bucketRepo := repository.NewStorageBucketRepo(db)

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

	project2 := &model.Project{
		ID:        uuid.New(),
		AccountID: account2.ID,
		Name:      "project-2",
		Status:    model.ProjectActive,
	}
	require.NoError(t, projectRepo.CreateProject(ctx, project2))
	defer func() { _, _ = db.NewDelete().Model(project2).Where("id = ?", project2.ID).Exec(ctx) }()

	bucket := &model.StorageBucket{
		ID:                 uuid.New(),
		AccountID:          account1.ID,
		ProjectID:          project1.ID,
		Name:               "isolated-bucket",
		PhysicalName:       "ocb-" + uuid.New().String(),
		Status:             model.BucketActive,
		ObjectCount:        0,
		StorageLimitBytes:  1073741824,
		MaxObjectSizeBytes: 104857600,
		CreatedAt:          time.Now().UTC(),
		UpdatedAt:          time.Now().UTC(),
	}
	require.NoError(t, bucketRepo.Create(ctx, bucket))

	found, err := bucketRepo.GetByAccount(ctx, account2.ID, bucket.ID)
	require.ErrorIs(t, err, sql.ErrNoRows)
	require.Nil(t, found)
}

func TestStorageBucketRepoIdempotencyConstraint(t *testing.T) {
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

	idempotencyKey := "same-key"

	bucket1 := &model.StorageBucket{
		ID:             uuid.New(),
		AccountID:      account.ID,
		ProjectID:      project.ID,
		IdempotencyKey: &idempotencyKey,
		Name:           "bucket-1",
		PhysicalName:   "ocb-" + uuid.New().String(),
		Status:         model.BucketActive,
		CreatedAt:      time.Now().UTC(),
		UpdatedAt:      time.Now().UTC(),
	}
	require.NoError(t, bucketRepo.Create(ctx, bucket1))

	bucket2 := &model.StorageBucket{
		ID:             uuid.New(),
		AccountID:      account.ID,
		ProjectID:      project.ID,
		IdempotencyKey: &idempotencyKey,
		Name:           "bucket-2",
		PhysicalName:   "ocb-" + uuid.New().String(),
		Status:         model.BucketActive,
		CreatedAt:      time.Now().UTC(),
		UpdatedAt:      time.Now().UTC(),
	}
	err = bucketRepo.Create(ctx, bucket2)
	require.Error(t, err)
	require.Contains(t, err.Error(), "UNIQUE constraint")
}

func strPtr(s string) *string {
	return &s
}

func TestStorageBucketRepoFindStaleAndFailingBuckets(t *testing.T) {
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

	staleCreating := &model.StorageBucket{
		ID:                 uuid.New(),
		AccountID:          account.ID,
		ProjectID:          project.ID,
		Name:               "stale-creating",
		PhysicalName:       "ocb-" + uuid.New().String(),
		Status:             model.BucketCreating,
		ObjectCount:        0,
		StorageLimitBytes:  1073741824,
		MaxObjectSizeBytes: 104857600,
		CreatedAt:          time.Now().UTC(),
		UpdatedAt:          time.Now().UTC(),
	}
	require.NoError(t, bucketRepo.Create(ctx, staleCreating))

	active := &model.StorageBucket{
		ID:                 uuid.New(),
		AccountID:          account.ID,
		ProjectID:          project.ID,
		Name:               "active",
		PhysicalName:       "ocb-" + uuid.New().String(),
		Status:             model.BucketActive,
		ObjectCount:        0,
		StorageLimitBytes:  1073741824,
		MaxObjectSizeBytes: 104857600,
		CreatedAt:          time.Now().UTC(),
		UpdatedAt:          time.Now().UTC(),
	}
	require.NoError(t, bucketRepo.Create(ctx, active))

	failing := &model.StorageBucket{
		ID:                 uuid.New(),
		AccountID:          account.ID,
		ProjectID:          project.ID,
		Name:               "failing",
		PhysicalName:       "ocb-" + uuid.New().String(),
		Status:             model.BucketFailed,
		LastError:          strPtr("last error"),
		ObjectCount:        0,
		StorageLimitBytes:  1073741824,
		MaxObjectSizeBytes: 104857600,
		CreatedAt:          time.Now().UTC(),
		UpdatedAt:          time.Now().UTC(),
	}
	require.NoError(t, bucketRepo.Create(ctx, failing))

	result, err := bucketRepo.FindStaleAndFailingBuckets(ctx, account.ID)
	require.NoError(t, err)
	require.Len(t, result, 2)

	foundMap := make(map[uuid.UUID]bool)
	for _, b := range result {
		foundMap[b.ID] = true
	}
	require.True(t, foundMap[staleCreating.ID])
	require.True(t, foundMap[failing.ID])
	require.False(t, foundMap[active.ID])
}
