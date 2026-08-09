package service_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	deploymentruntime "github.com/nazxf/opencloud/backend/internal/deployment"
	"github.com/nazxf/opencloud/backend/internal/model"
	"github.com/nazxf/opencloud/backend/internal/registry"
	"github.com/nazxf/opencloud/backend/internal/repository"
	"github.com/nazxf/opencloud/backend/internal/service"
)

const (
	deploymentDigestOne = "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	deploymentDigestTwo = "sha256:fedcba9876543210fedcba9876543210fedcba9876543210fedcba9876543210"
)

func TestDeploymentPublishPromoteAndRollbackPreserveImmutableRevision(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	var lifecycleFunction bool
	require.NoError(t, db.NewRaw(`SELECT to_regprocedure('enforce_opencloud_deployment_lifecycle()') IS NOT NULL`).Scan(ctx, &lifecycleFunction))
	if !lifecycleFunction {
		t.Skip("Phase 4D lifecycle migration missing; run migrations first")
	}

	now := time.Now().UTC()
	account := &model.Account{ID: uuid.New(), Name: "Deployment account " + uuid.NewString(), Status: model.AccountActive, CreatedAt: now, UpdatedAt: now}
	_, err := db.NewInsert().Model(account).Exec(ctx)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = db.NewDelete().Model((*model.Account)(nil)).Where("id = ?", account.ID).Exec(ctx)
	})

	projects := repository.NewProjectRepo(db)
	project := &model.Project{ID: uuid.New(), AccountID: account.ID, Name: "deployment-project-" + uuid.NewString(), Status: model.ProjectActive}
	require.NoError(t, projects.CreateProject(ctx, project))
	workload := &model.Service{ID: uuid.New(), AccountID: account.ID, ProjectID: project.ID, Name: "api", ServiceType: model.ServiceTypeWeb, SourceRoot: ".", Status: model.ServiceActive}
	require.NoError(t, projects.CreateService(ctx, workload))

	privateRegistry := registry.NewFakeProvider()
	runtime := &deploymentruntime.FakeRuntime{}
	deployments, err := service.NewDeploymentService(db, projects, privateRegistry, runtime)
	require.NoError(t, err)
	repositoryName, err := registry.NewRepository("registry.internal", account.ID, project.ID, workload.ID)
	require.NoError(t, err)

	first := publishDeployment(t, deployments, account.ID, project.ID, workload.ID, repositoryName, deploymentDigestOne)
	firstActive, err := deployments.Deploy(ctx, account.ID, project.ID, workload.ID, first.ID)
	require.NoError(t, err)
	require.Equal(t, model.DeploymentReady, firstActive.Status)
	require.True(t, firstActive.IsActive)

	second := publishDeployment(t, deployments, account.ID, project.ID, workload.ID, repositoryName, deploymentDigestTwo)
	secondActive, err := deployments.Deploy(ctx, account.ID, project.ID, workload.ID, second.ID)
	require.NoError(t, err)
	require.True(t, secondActive.IsActive)
	require.Equal(t, model.DeploymentReady, secondActive.Status)

	rolledBack, err := deployments.Rollback(ctx, account.ID, project.ID, workload.ID, first.ID)
	require.NoError(t, err)
	require.Equal(t, first.ID, rolledBack.ID)
	require.True(t, rolledBack.IsActive)
	require.Equal(t, deploymentDigestOne, rolledBack.ImageDigest)

	reloadedFirst, err := projects.GetDeploymentByAccount(ctx, account.ID, project.ID, workload.ID, first.ID)
	require.NoError(t, err)
	require.Equal(t, deploymentDigestOne, reloadedFirst.ImageDigest)
	require.True(t, reloadedFirst.IsActive)
	reloadedSecond, err := projects.GetDeploymentByAccount(ctx, account.ID, project.ID, workload.ID, second.ID)
	require.NoError(t, err)
	require.False(t, reloadedSecond.IsActive)
	require.Equal(t, model.DeploymentReady, reloadedSecond.Status)

	// The DB guard prevents a ready immutable deployment from being sent back
	// into an earlier lifecycle stage, even if an erroneous caller bypasses Go.
	_, err = db.NewRaw(`UPDATE deployments SET status = 'queued' WHERE id = ?`, first.ID).Exec(ctx)
	require.Error(t, err)

	actions := runtime.Actions()
	var got []string
	for _, action := range actions {
		got = append(got, action.Name)
	}
	want := []string{
		deploymentruntime.ActionStart, deploymentruntime.ActionCheckHealth, deploymentruntime.ActionSwitchTraffic,
		deploymentruntime.ActionStart, deploymentruntime.ActionCheckHealth, deploymentruntime.ActionSwitchTraffic, deploymentruntime.ActionRetire,
		deploymentruntime.ActionStart, deploymentruntime.ActionCheckHealth, deploymentruntime.ActionSwitchTraffic, deploymentruntime.ActionRetire,
	}
	require.Equal(t, want, got, fmt.Sprintf("unexpected runtime sequence: %#v", actions))
}

func publishDeployment(
	t *testing.T,
	deployments *service.DeploymentService,
	accountID, projectID, serviceID uuid.UUID,
	repositoryName registry.Repository,
	digest string,
) *model.Deployment {
	t.Helper()
	row, err := deployments.Publish(context.Background(), accountID, projectID, serviceID, service.PublishDeploymentRequest{
		Repository: repositoryName, SourceDigest: digest, SourceBytes: 123, BuildProvider: "railpack",
	})
	require.NoError(t, err)
	return row
}
