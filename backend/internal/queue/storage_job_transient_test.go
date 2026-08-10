package queue_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"sync"
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

// TestStorageJobTransientFailureRetries verifies the complete retry lifecycle
// when a storage provider returns a transient error.
func TestStorageJobTransientFailureRetries(t *testing.T) {
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

	// Transient-fail provider: fails first attempt, succeeds on second
	attemptCounter := 0
	_ = attemptCounter // use to silence warning
	transientProvider := &transientFailProvider{
		fakeProvider: provisioner.NewFakeStorageProvider(),
		failAttempts: 1,
	}
	log, _ := zap.NewDevelopment()
	handlers := queue.NewStorageJobHandlers(log, db, bucketRepo, jobsRepo, auditRepo, transientProvider)

	payload := model.ProvisionStorageBucketPayload{BucketID: bucketID}
	rawPayload, _ := json.Marshal(payload)
	job := &model.Job{
		ID:          uuid.New(),
		AccountID:   &account.ID,
		Kind:        model.JobProvisionStorageBucket,
		Status:      model.JobQueued,
		Attempts:    0,
		MaxAttempts: 3, // Allow multiple retries for testing
		Payload:     rawPayload,
		RunAt:       time.Now().UTC(),
	}
	_, err = db.NewInsert().Model(job).Exec(ctx)
	require.NoError(t, err)

	// ========== STEP 1: Claim becomes running ==========
	claimed, err := jobsRepo.Claim(ctx, "worker-1")
	require.NoError(t, err)
	require.NotNil(t, claimed)
	require.Equal(t, job.ID, claimed.ID)
	require.Equal(t, model.JobRunning, claimed.Status)
	require.Equal(t, int64(1), claimed.Attempts)
	require.NotNil(t, claimed.LockedAt)
	require.Equal(t, "worker-1", claimed.LockedBy)

	// Verify in DB
	var checkAttempt int64
	err = db.NewSelect().Model((*model.Job)(nil)).Where("id = ?", job.ID).Column("attempts").Scan(ctx, &checkAttempt)
	require.NoError(t, err)
	require.Equal(t, int64(1), checkAttempt)

	// ========== STEP 2: Provider returns transient error ==========
	err = handlers.Handle(ctx, claimed, "worker-1")
	require.Error(t, err, "expected transient provision failure")
	require.Contains(t, err.Error(), "provision failed")

	// Verify bucket is NOT terminal failed
	result, err := bucketRepo.GetByAccount(ctx, account.ID, bucketID)
	require.NoError(t, err)
	require.Equal(t, model.BucketCreating, result.Status, "bucket should remain creating after transient failure")
	require.Nil(t, result.LastError)

	// Verify attempts still = 1 (not incremented during handle)
	err = db.NewSelect().Model((*model.Job)(nil)).Where("id = ?", job.ID).Column("attempts").Scan(ctx, &checkAttempt)
	require.NoError(t, err)
	require.Equal(t, int64(1), checkAttempt, "attempts should still be 1 after handle returns error")

	// ========== STEP 3-9: Runner calls Retry, job transitions properly ==========
	// Simulate Runner's retry logic
	runAt := time.Now().UTC().Add(5 * time.Second)
	safeError := "provisioner operation failed"
	err = jobsRepo.Retry(ctx, job.ID, "worker-1", safeError, runAt)
	require.NoError(t, err)

	// Verify job status = queued
	var jobStatus string
	err = db.NewSelect().Model((*model.Job)(nil)).Where("id = ?", job.ID).Column("status").Scan(ctx, &jobStatus)
	require.NoError(t, err)
	require.Equal(t, model.JobQueued, jobStatus)

	// Verify locked_at cleared
	var lockedAt interface{}
	err = db.NewSelect().Model((*model.Job)(nil)).Where("id = ?", job.ID).Column("locked_at").Scan(ctx, &lockedAt)
	require.NoError(t, err)
	require.Nil(t, lockedAt)

	// Verify locked_by cleared
	var lockedBy interface{}
	err = db.NewSelect().Model((*model.Job)(nil)).Where("id = ?", job.ID).Column("locked_by").Scan(ctx, &lockedBy)
	require.NoError(t, err)
	require.Nil(t, lockedBy)

	// Verify run_at moved into future
	var retrievedRunAt time.Time
	err = db.NewSelect().Model((*model.Job)(nil)).Where("id = ?", job.ID).Column("run_at").Scan(ctx, &retrievedRunAt)
	require.NoError(t, err)
	require.True(t, retrievedRunAt.After(time.Now().UTC()), "run_at should be in future")

	// Verify last_error contains only safe queue error
	var lastError *string
	err = db.NewSelect().Model((*model.Job)(nil)).Where("id = ?", job.ID).Column("last_error").Scan(ctx, &lastError)
	require.NoError(t, err)
	require.NotNil(t, lastError)
	require.Equal(t, safeError, *lastError, "last_error should contain only safe queue error, not stack trace")

	// ========== STEP 10: Bucket does NOT become terminal failed ==========
	result, err = bucketRepo.GetByAccount(ctx, account.ID, bucketID)
	require.NoError(t, err)
	require.Equal(t, model.BucketCreating, result.Status)
	require.Nil(t, result.LastError)

	// ========== STEP 11: Attempts not incremented a second time ==========
	err = db.NewSelect().Model((*model.Job)(nil)).Where("id = ?", job.ID).Column("attempts").Scan(ctx, &checkAttempt)
	require.NoError(t, err)
	require.Equal(t, int64(1), checkAttempt)

	// ========== Verify can claim again ==========
	claimed2, err := jobsRepo.Claim(ctx, "worker-1")
	require.NoError(t, err)
	require.NotNil(t, claimed2)
	require.Equal(t, job.ID, claimed2.ID)

	err = handlers.Handle(ctx, claimed2, "worker-1")
	require.NoError(t, err)

	// Final verification: bucket should now be active (provider succeeded on second attempt)
	result, err = bucketRepo.GetByAccount(ctx, account.ID, bucketID)
	require.NoError(t, err)
	require.Equal(t, model.BucketActive, result.Status)
}

// Helper: TransientFailProvider fails for N attempts then succeeds.
type transientFailProvider struct {
	fakeProvider *provisioner.FakeStorageProvider
	failAttempts int
	currentCall  int
	mu           sync.Mutex
}

func (p *transientFailProvider) CreateBucket(ctx context.Context, spec provisioner.BucketSpec) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.currentCall++

	if p.currentCall <= p.failAttempts {
		return errors.New("transient provision failure")
	}
	return p.fakeProvider.CreateBucket(ctx, spec)
}

func (p *transientFailProvider) DeleteBucket(ctx context.Context, ref provisioner.BucketRef) error {
	return p.fakeProvider.DeleteBucket(ctx, ref)
}

func (p *transientFailProvider) BucketExists(ctx context.Context, ref provisioner.BucketRef) (bool, error) {
	return p.fakeProvider.BucketExists(ctx, ref)
}

func (p *transientFailProvider) PutObject(ctx context.Context, spec provisioner.PutObjectSpec) (*provisioner.ObjectInfo, error) {
	return p.fakeProvider.PutObject(ctx, spec)
}

func (p *transientFailProvider) GetObject(ctx context.Context, ref provisioner.ObjectRef) (io.ReadCloser, *provisioner.ObjectInfo, error) {
	return p.fakeProvider.GetObject(ctx, ref)
}

func (p *transientFailProvider) ListObjects(ctx context.Context, ref provisioner.ObjectRef, opts provisioner.ListObjectsOptions) ([]provisioner.ObjectInfo, string, error) {
	return p.fakeProvider.ListObjects(ctx, ref, opts)
}

func (p *transientFailProvider) DeleteObject(ctx context.Context, ref provisioner.ObjectRef) error {
	return p.fakeProvider.DeleteObject(ctx, ref)
}

func (p *transientFailProvider) HeadObject(ctx context.Context, ref provisioner.ObjectRef) (*provisioner.ObjectInfo, error) {
	return p.fakeProvider.HeadObject(ctx, ref)
}

func (p *transientFailProvider) PresignedGetURL(ctx context.Context, ref provisioner.ObjectRef, expiry time.Duration) (string, error) {
	return p.fakeProvider.PresignedGetURL(ctx, ref, expiry)
}

func (p *transientFailProvider) PresignedPutURL(ctx context.Context, ref provisioner.ObjectRef, expiry time.Duration) (string, error) {
	return p.fakeProvider.PresignedPutURL(ctx, ref, expiry)
}
