package repository

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"

	"github.com/nazxf/opencloud/backend/internal/database"
	"github.com/nazxf/opencloud/backend/internal/model"
)

func openConsoleSessionTestDB(t *testing.T) *bun.DB {
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
	err = db.NewRaw(`SELECT count(*) FROM information_schema.tables WHERE table_name = 'database_console_sessions'`).Scan(context.Background(), &n)
	if err != nil || n == 0 {
		t.Skip("database_console_sessions missing; run migrations first")
	}
	return db
}

func seedDatabaseFixture(t *testing.T, db *bun.DB) uuid.UUID {
	t.Helper()
	accountID := uuid.New()
	account := &model.Account{
		ID:     accountID,
		Name:   "console session test",
		Status: model.AccountActive,
	}
	_, err := db.NewInsert().Model(account).Exec(context.Background())
	require.NoError(t, err)

	return accountID
}

func TestDatabaseConsoleSessionRepository_Create(t *testing.T) {
	db := openConsoleSessionTestDB(t)
	repo := NewDatabaseConsoleSessionRepository(db)

	accountID := seedDatabaseFixture(t, db)
	databaseID := uuid.New()

	session := &model.DatabaseConsoleSession{
		AccountID:  accountID,
		DatabaseID: databaseID,
		ActorID:    "test-actor-id",
		Engine:     model.DatabaseEnginePostgres,
		Status:     model.ConsoleSessionActive,
		ExpiresAt:  time.Now().Add(30 * time.Minute),
		CreatedAt:  time.Now(),
	}

	err := repo.Create(context.Background(), session)
	require.NoError(t, err)
	assert.NotEqual(t, uuid.Nil, session.ID)
}

func TestDatabaseConsoleSessionRepository_GetActiveByDatabase(t *testing.T) {
	db := openConsoleSessionTestDB(t)
	repo := NewDatabaseConsoleSessionRepository(db)

	accountID := seedDatabaseFixture(t, db)
	databaseID := uuid.New()

	now := time.Now()
	expiresAt := now.Add(30 * time.Minute)

	session := &model.DatabaseConsoleSession{
		AccountID:  accountID,
		DatabaseID: databaseID,
		ActorID:    "test-actor-id",
		Engine:     model.DatabaseEnginePostgres,
		Status:     model.ConsoleSessionActive,
		ExpiresAt:  expiresAt,
		CreatedAt:  now,
	}

	err := repo.Create(context.Background(), session)
	require.NoError(t, err)

	found, err := repo.GetActiveByDatabase(context.Background(), accountID, databaseID)
	require.NoError(t, err)
	assert.Equal(t, session.ID, found.ID)
	assert.Equal(t, model.ConsoleSessionActive, found.Status)
}

func TestDatabaseConsoleSessionRepository_Revoke(t *testing.T) {
	db := openConsoleSessionTestDB(t)
	repo := NewDatabaseConsoleSessionRepository(db)

	accountID := seedDatabaseFixture(t, db)
	databaseID := uuid.New()

	session := &model.DatabaseConsoleSession{
		AccountID:  accountID,
		DatabaseID: databaseID,
		ActorID:    "test-actor-id",
		Engine:     model.DatabaseEngineMariaDB,
		Status:     model.ConsoleSessionActive,
		ExpiresAt:  time.Now().Add(30 * time.Minute),
		CreatedAt:  time.Now(),
	}

	err := repo.Create(context.Background(), session)
	require.NoError(t, err)

	err = repo.Revoke(context.Background(), session.ID, "test-actor-id")
	require.NoError(t, err)

	found, err := repo.GetActiveByDatabase(context.Background(), accountID, databaseID)
	assert.Error(t, err) // Should return error when session is revoked
	assert.Nil(t, found)
}

func TestDatabaseConsoleSessionRepository_ExpireOldSessions(t *testing.T) {
	db := openConsoleSessionTestDB(t)
	repo := NewDatabaseConsoleSessionRepository(db)

	accountID := seedDatabaseFixture(t, db)
	databaseID := uuid.New()

	now := time.Now()

	// Create an expired session
	expiredSession := &model.DatabaseConsoleSession{
		AccountID:  accountID,
		DatabaseID: databaseID,
		ActorID:    "test-actor-id-1",
		Engine:     model.DatabaseEnginePostgres,
		Status:     model.ConsoleSessionActive,
		ExpiresAt:  now.Add(-1 * time.Hour), // Already expired
		CreatedAt:  now.Add(-2 * time.Hour),
	}
	err := repo.Create(context.Background(), expiredSession)
	require.NoError(t, err)

	// Create a valid session
	validSession := &model.DatabaseConsoleSession{
		AccountID:  accountID,
		DatabaseID: databaseID,
		ActorID:    "test-actor-id-2",
		Engine:     model.DatabaseEngineMariaDB,
		Status:     model.ConsoleSessionActive,
		ExpiresAt:  now.Add(1 * time.Hour), // Still valid
		CreatedAt:  now,
	}
	err = repo.Create(context.Background(), validSession)
	require.NoError(t, err)

	rowsExpired, err := repo.ExpireOldSessions(context.Background(), now)
	require.NoError(t, err)
	assert.Equal(t, int64(1), rowsExpired)

	// Verify expired session was marked as expired
	var expired model.DatabaseConsoleSession
	err = db.NewSelect().Model(&expired).
		Where("id = ?", expiredSession.ID).
		Scan(context.Background())
	require.NoError(t, err)
	assert.Equal(t, model.ConsoleSessionExpired, expired.Status)

	// Verify valid session is still active
	var valid model.DatabaseConsoleSession
	err = db.NewSelect().Model(&valid).
		Where("id = ?", validSession.ID).
		Scan(context.Background())
	require.NoError(t, err)
	assert.Equal(t, model.ConsoleSessionActive, valid.Status)
}

func TestDatabaseConsoleSessionRepository_DeleteByDatabase(t *testing.T) {
	db := openConsoleSessionTestDB(t)
	repo := NewDatabaseConsoleSessionRepository(db)

	accountID := seedDatabaseFixture(t, db)
	databaseID := uuid.New()

	// Create multiple sessions for same database
	for i := 0; i < 3; i++ {
		session := &model.DatabaseConsoleSession{
			AccountID:  accountID,
			DatabaseID: databaseID,
			ActorID:    "test-actor-id-" + string(rune('0'+i)),
			Engine:     model.DatabaseEnginePostgres,
			Status:     model.ConsoleSessionActive,
			ExpiresAt:  time.Now().Add(30 * time.Minute),
			CreatedAt:  time.Now(),
		}
		err := repo.Create(context.Background(), session)
		require.NoError(t, err)
	}

	err := repo.DeleteByDatabase(context.Background(), databaseID)
	require.NoError(t, err)

	// Verify all sessions deleted
	var count int
	err = db.NewRaw(`SELECT count(*) FROM database_console_sessions WHERE database_id = ?`, databaseID).Scan(context.Background(), &count)
	require.NoError(t, err)
	assert.Equal(t, 0, count)
}
