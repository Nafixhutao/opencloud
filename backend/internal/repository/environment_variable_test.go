package repository

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"

	"github.com/nazxf/opencloud/backend/internal/database"
	"github.com/nazxf/opencloud/backend/internal/model"
)

func openEnvVarTestDB(t *testing.T) *bun.DB {
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

	var n int
	err = db.NewRaw(`SELECT count(*) FROM information_schema.tables WHERE table_name = 'environment_variables'`).Scan(context.Background(), &n)
	if err != nil || n == 0 {
		t.Skip("environment_variables missing; run migrations first")
	}
	return db
}

func seedEnvVarFixture(t *testing.T, db *bun.DB) (accountID, projectID, serviceID uuid.UUID) {
	t.Helper()
	accountID = uuid.New()
	projectID = uuid.New()
	serviceID = uuid.New()

	account := &model.Account{
		ID:     accountID,
		Name:   "env var test",
		Status: model.AccountActive,
	}
	_, err := db.NewInsert().Model(account).Exec(context.Background())
	require.NoError(t, err)

	project := &model.Project{
		ID:        projectID,
		AccountID: accountID,
		Name:      "env var test project",
		Status:    model.ProjectActive,
	}
	_, err = db.NewInsert().Model(project).Exec(context.Background())
	require.NoError(t, err)

	service := &model.Service{
		ID:          serviceID,
		AccountID:   accountID,
		ProjectID:   projectID,
		Name:        "env-var-test-service",
		ServiceType: model.ServiceTypeWeb,
		SourceRoot:  ".",
		Status:      model.ServiceActive,
	}
	_, err = db.NewInsert().Model(service).Exec(context.Background())
	require.NoError(t, err)

	return accountID, projectID, serviceID
}

func TestEnvironmentVariableRepository_Create(t *testing.T) {
	db := openEnvVarTestDB(t)
	repo := NewEnvironmentVariableRepository(db)

	accountID, projectID, serviceID := seedEnvVarFixture(t, db)
	actorID := "test-user-id-1"

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
	db := openEnvVarTestDB(t)
	repo := NewEnvironmentVariableRepository(db)

	accountID, projectID, serviceID := seedEnvVarFixture(t, db)
	actorID := "test-user-id-1"

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
	db := openEnvVarTestDB(t)
	repo := NewEnvironmentVariableRepository(db)

	accountID, projectID, serviceID := seedEnvVarFixture(t, db)
	actorID := "test-user-id-1"

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
	db := openEnvVarTestDB(t)
	repo := NewEnvironmentVariableRepository(db)

	accountID, projectID, serviceID := seedEnvVarFixture(t, db)
	actorID := "test-user-id-1"

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
	db := openEnvVarTestDB(t)
	repo := NewEnvironmentVariableRepository(db)

	accountID, projectID, serviceID := seedEnvVarFixture(t, db)
	actorID := "test-user-id-1"

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
