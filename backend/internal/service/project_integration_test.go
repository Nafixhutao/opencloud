package service_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/nazxf/opencloud/backend/internal/model"
	"github.com/nazxf/opencloud/backend/internal/repository"
)

func TestProjectRepoDoesNotExposeAnotherAccountsProject(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	var count int
	require.NoError(t, db.NewRaw(`SELECT count(*) FROM information_schema.tables WHERE table_schema = 'public' AND table_name = 'projects'`).Scan(ctx, &count))
	if count != 1 {
		t.Skip("Phase 4A tables missing; run migrations first")
	}

	now := time.Now().UTC()
	owner := &model.Account{ID: uuid.New(), Name: "Project owner " + uuid.NewString(), Status: model.AccountActive, CreatedAt: now, UpdatedAt: now}
	other := &model.Account{ID: uuid.New(), Name: "Project other " + uuid.NewString(), Status: model.AccountActive, CreatedAt: now, UpdatedAt: now}
	_, err := db.NewInsert().Model(owner).Exec(ctx)
	require.NoError(t, err)
	_, err = db.NewInsert().Model(other).Exec(ctx)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = db.NewDelete().Model((*model.Account)(nil)).Where("id IN (?, ?)", owner.ID, other.ID).Exec(ctx)
	})

	projects := repository.NewProjectRepo(db)
	project := &model.Project{ID: uuid.New(), AccountID: owner.ID, Name: "isolated-project", Status: model.ProjectActive}
	require.NoError(t, projects.CreateProject(ctx, project))

	_, err = projects.GetProjectByAccount(ctx, other.ID, project.ID)
	require.ErrorIs(t, err, sql.ErrNoRows)
}
