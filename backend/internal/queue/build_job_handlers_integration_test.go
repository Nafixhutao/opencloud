package queue_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
	"go.uber.org/zap"

	"github.com/nazxf/opencloud/backend/internal/model"
	"github.com/nazxf/opencloud/backend/internal/queue"
	"github.com/nazxf/opencloud/backend/internal/repository"
)

// TestEnqueueDestroySchedulesOnlyItsOwnJob pins the destroy-scheduling contract:
// delaying one preview's destroy job must never reschedule another queued
// destroy, and the delay must actually be applied to the returned job.
func TestEnqueueDestroySchedulesOnlyItsOwnJob(t *testing.T) {
	t.Parallel()

	db := openTestDB(t)
	ctx := context.Background()

	acctRepo := repository.NewAccountRepo(db)
	jobRepo := repository.NewJobRepo(db)

	account, err := acctRepo.CreateAccount(ctx, "destroy-scheduling-test")
	require.NoError(t, err)
	defer cleanupAccount(t, db, ctx, account)

	handlers := queue.NewBuildJobHandlers(
		zap.NewNop(), db,
		nil, nil, jobRepo, nil, // repos unused on the destroy path
		nil, nil, nil, "",
	)

	first, err := handlers.EnqueueDestroy(ctx, account.ID, uuid.NewString())
	require.NoError(t, err)

	firstRunAt := readJobRunAt(t, db, first.ID)

	// A second scheduled destroy must leave the first job's schedule alone.
	second, err := handlers.EnqueueDestroy(ctx, account.ID, uuid.NewString())
	require.NoError(t, err)

	require.Equal(t, firstRunAt, readJobRunAt(t, db, first.ID), "scheduling a second destroy must not move the first job's run_at")

	// Both jobs carry a real ~24h delay rather than running immediately.
	for _, jobID := range []uuid.UUID{first.ID, second.ID} {
		runAt := readJobRunAt(t, db, jobID)
		require.WithinDuration(t, time.Now().UTC().Add(24*time.Hour), runAt, 5*time.Minute, "destroy job should run ~24h from now")
	}

	// Payloads stay scoped to their own preview.
	var firstPayload map[string]string
	require.NoError(t, json.Unmarshal(first.Payload, &firstPayload))
	require.Len(t, firstPayload, 1)
	require.NotEmpty(t, firstPayload["preview_id"])
}

func readJobRunAt(t *testing.T, db *bun.DB, id uuid.UUID) time.Time {
	t.Helper()
	var job model.Job
	require.NoError(t, db.NewSelect().Model(&job).Where("id = ?", id).Scan(context.Background()))
	return job.RunAt
}
