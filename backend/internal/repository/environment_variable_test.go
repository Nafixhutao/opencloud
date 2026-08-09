package repository

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Nafixhutao/opencloud/backend/internal/model"
	"github.com/Nafixhutao/opencloud/backend/internal/testutil"
)

func TestEnvironmentVariableRepository_Create(t *testing.T) {
	db := testutil.TestDB(t)
	repo := NewEnvironmentVariableRepository(db)

	accountID := uuid.New()
	projectID := uuid.New()
	serviceID := uuid.New()
	actorID := uuid.New()

	testutil.SeedAccount(t, db, accountID)
	testutil.SeedProject(t, db, accountID, projectID)
	testutil.SeedService(t, db, accountID, projectID, serviceID)

	variable := &model.EnvironmentVariable{
		AccountID:   accountID,
		ProjectID:   projectID,
		ServiceID:   serviceID,
		Key:         "TEST_VAR",
		Value:       stringPtr("test-value"),
		IsSecret:    false,
		Environment: model.EnvProduction,
		CreatedBy:   actorID,
	}

	err := repo.Create(context.Background(), variable, actorID)
	require.NoError(t, err)
	assert.NotEqual(t, uuid.Nil, variable.ID)
}

func TestEnvironmentVariableRepository_CreateSecret(t *testing.T) {
	db := testutil.TestDB(t)
	repo := NewEnvironmentVariableRepository(db)

	accountID := uuid.New()
	projectID := uuid.New()
	serviceID := uuid.New()
	actorID := uuid.New()

	testutil.SeedAccount(t, db, accountID)
	testutil.SeedProject(t, db, accountID, projectID)
	testutil.SeedService(t, db, accountID, projectID, serviceID)

	variable := &model.EnvironmentVariable{
		AccountID:      accountID,
		ProjectID:      projectID,
		ServiceID:      serviceID,
		Key:            "SECRET_KEY",
		EncryptedValue: []byte("encrypted-secret"),
		IsSecret:       true,
		Environment:    model.EnvProduction,
		CreatedBy:      actorID,
	}

	err := repo.Create(context.Background(), variable, actorID)
	require.NoError(t, err)
	assert.NotEqual(t, uuid.Nil, variable.ID)

	// Verify audit was created
	audits, err := repo.ListAudit(context.Background(), accountID, projectID, serviceID, 10)
	require.NoError(t, err)
	assert.Len(t, audits, 1)
	assert.Equal(t, model.EnvAuditCreated, audits[0].Action)
	assert.Equal(t, "SECRET_KEY", audits[0].Key)
}

func TestEnvironmentVariableRepository_ListByService(t *testing.T) {
	db := testutil.TestDB(t)
	repo := NewEnvironmentVariableRepository(db)

	accountID := uuid.New()
	projectID := uuid.New()
	serviceID := uuid.New()
	actorID := uuid.New()

	testutil.SeedAccount(t, db, accountID)
	testutil.SeedProject(t, db, accountID, projectID)
	testutil.SeedService(t, db, accountID, projectID, serviceID)

	// Create variables for different environments
	for _, env := range []string{model.EnvProduction, model.EnvPreview} {
		variable := &model.EnvironmentVariable{
			AccountID:   accountID,
			ProjectID:   projectID,
			ServiceID:   serviceID,
			Key:         "TEST_VAR",
			Value:       stringPtr("test-value"),
			IsSecret:    false,
			Environment: env,
			CreatedBy:   actorID,
		}
		require.NoError(t, repo.Create(context.Background(), variable, actorID))
	}

	// List production variables
	variables, err := repo.ListByService(context.Background(), accountID, projectID, serviceID, model.EnvProduction)
	require.NoError(t, err)
	assert.Len(t, variables, 1)
	assert.Equal(t, model.EnvProduction, variables[0].Environment)
}

func TestEnvironmentVariableRepository_Update(t *testing.T) {
	db := testutil.TestDB(t)
	repo := NewEnvironmentVariableRepository(db)

	accountID := uuid.New()
	projectID := uuid.New()
	serviceID := uuid.New()
	actorID := uuid.New()

	testutil.SeedAccount(t, db, accountID)
	testutil.SeedProject(t, db, accountID, projectID)
	testutil.SeedService(t, db, accountID, projectID, serviceID)

	variable := &model.EnvironmentVariable{
		AccountID:   accountID,
		ProjectID:   projectID,
		ServiceID:   serviceID,
		Key:         "TEST_VAR",
		Value:       stringPtr("old-value"),
		IsSecret:    false,
		Environment: model.EnvProduction,
		CreatedBy:   actorID,
	}
	require.NoError(t, repo.Create(context.Background(), variable, actorID))

	// Update value
	variable.Value = stringPtr("new-value")
	err := repo.Update(context.Background(), variable, actorID)
	require.NoError(t, err)

	// Verify audit trail has both created and updated
	audits, err := repo.ListAudit(context.Background(), accountID, projectID, serviceID, 10)
	require.NoError(t, err)
	assert.Len(t, audits, 2)
}

func TestEnvironmentVariableRepository_Delete(t *testing.T) {
	db := testutil.TestDB(t)
	repo := NewEnvironmentVariableRepository(db)

	accountID := uuid.New()
	projectID := uuid.New()
	serviceID := uuid.New()
	actorID := uuid.New()

	testutil.SeedAccount(t, db, accountID)
	testutil.SeedProject(t, db, accountID, projectID)
	testutil.SeedService(t, db, accountID, projectID, serviceID)

	variable := &model.EnvironmentVariable{
		AccountID:   accountID,
		ProjectID:   projectID,
		ServiceID:   serviceID,
		Key:         "TEST_VAR",
		Value:       stringPtr("test-value"),
		IsSecret:    false,
		Environment: model.EnvProduction,
		CreatedBy:   actorID,
	}
	require.NoError(t, repo.Create(context.Background(), variable, actorID))

	err := repo.Delete(context.Background(), accountID, variable.ID, actorID)
	require.NoError(t, err)

	// Verify audit trail has created and deleted
	audits, err := repo.ListAudit(context.Background(), accountID, projectID, serviceID, 10)
	require.NoError(t, err)
	assert.Len(t, audits, 2)
	assert.Equal(t, model.EnvAuditDeleted, audits[0].Action) // Most recent first
}

func stringPtr(s string) *string {
	return &s
}
