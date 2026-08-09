package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/uptrace/bun"

	"github.com/Nafixhutao/opencloud/backend/internal/model"
)

// EnvironmentVariableRepository manages tenant-scoped environment variables and secrets.
type EnvironmentVariableRepository struct {
	db *bun.DB
}

// NewEnvironmentVariableRepository creates a repository for environment variables.
func NewEnvironmentVariableRepository(db *bun.DB) *EnvironmentVariableRepository {
	return &EnvironmentVariableRepository{db: db}
}

// Create inserts a new environment variable with audit trail.
func (r *EnvironmentVariableRepository) Create(ctx context.Context, variable *model.EnvironmentVariable, actorID uuid.UUID) error {
	return r.db.RunInTx(ctx, &sql.TxOptions{}, func(ctx context.Context, tx bun.Tx) error {
		if _, err := tx.NewInsert().Model(variable).Exec(ctx); err != nil {
			return fmt.Errorf("insert environment variable: %w", err)
		}

		audit := &model.EnvironmentVariableAudit{
			AccountID:   variable.AccountID,
			ProjectID:   variable.ProjectID,
			ServiceID:   variable.ServiceID,
			VariableID:  &variable.ID,
			Action:      model.EnvAuditCreated,
			Key:         variable.Key,
			IsSecret:    variable.IsSecret,
			Environment: variable.Environment,
			ActorID:     actorID,
			Metadata:    json.RawMessage(`{}`),
		}
		if _, err := tx.NewInsert().Model(audit).Exec(ctx); err != nil {
			return fmt.Errorf("append environment variable audit: %w", err)
		}

		return nil
	})
}

// Update modifies an existing environment variable with audit trail.
func (r *EnvironmentVariableRepository) Update(ctx context.Context, variable *model.EnvironmentVariable, actorID uuid.UUID) error {
	return r.db.RunInTx(ctx, &sql.TxOptions{}, func(ctx context.Context, tx bun.Tx) error {
		result, err := tx.NewUpdate().
			Model(variable).
			Column("value", "encrypted_value", "is_secret", "updated_at").
			Where("id = ? AND account_id = ?", variable.ID, variable.AccountID).
			Exec(ctx)
		if err != nil {
			return fmt.Errorf("update environment variable: %w", err)
		}
		if rows, _ := result.RowsAffected(); rows == 0 {
			return sql.ErrNoRows
		}

		action := model.EnvAuditUpdated
		if variable.IsSecret {
			action = model.EnvAuditRotated
		}

		audit := &model.EnvironmentVariableAudit{
			AccountID:   variable.AccountID,
			ProjectID:   variable.ProjectID,
			ServiceID:   variable.ServiceID,
			VariableID:  &variable.ID,
			Action:      action,
			Key:         variable.Key,
			IsSecret:    variable.IsSecret,
			Environment: variable.Environment,
			ActorID:     actorID,
			Metadata:    json.RawMessage(`{}`),
		}
		if _, err := tx.NewInsert().Model(audit).Exec(ctx); err != nil {
			return fmt.Errorf("append environment variable audit: %w", err)
		}

		return nil
	})
}

// Delete removes an environment variable with audit trail.
func (r *EnvironmentVariableRepository) Delete(ctx context.Context, accountID, id uuid.UUID, actorID uuid.UUID) error {
	return r.db.RunInTx(ctx, &sql.TxOptions{}, func(ctx context.Context, tx bun.Tx) error {
		var variable model.EnvironmentVariable
		if err := tx.NewSelect().
			Model(&variable).
			Where("id = ? AND account_id = ?", id, accountID).
			Scan(ctx); err != nil {
			return fmt.Errorf("select environment variable for delete: %w", err)
		}

		result, err := tx.NewDelete().
			Model((*model.EnvironmentVariable)(nil)).
			Where("id = ? AND account_id = ?", id, accountID).
			Exec(ctx)
		if err != nil {
			return fmt.Errorf("delete environment variable: %w", err)
		}
		if rows, _ := result.RowsAffected(); rows == 0 {
			return sql.ErrNoRows
		}

		audit := &model.EnvironmentVariableAudit{
			AccountID:   variable.AccountID,
			ProjectID:   variable.ProjectID,
			ServiceID:   variable.ServiceID,
			VariableID:  &variable.ID,
			Action:      model.EnvAuditDeleted,
			Key:         variable.Key,
			IsSecret:    variable.IsSecret,
			Environment: variable.Environment,
			ActorID:     actorID,
			Metadata:    json.RawMessage(`{}`),
		}
		if _, err := tx.NewInsert().Model(audit).Exec(ctx); err != nil {
			return fmt.Errorf("append environment variable audit: %w", err)
		}

		return nil
	})
}

// GetByID retrieves one environment variable by ID within account scope.
func (r *EnvironmentVariableRepository) GetByID(ctx context.Context, accountID, id uuid.UUID) (*model.EnvironmentVariable, error) {
	var variable model.EnvironmentVariable
	err := r.db.NewSelect().
		Model(&variable).
		Where("id = ? AND account_id = ?", id, accountID).
		Scan(ctx)
	if err != nil {
		return nil, fmt.Errorf("select environment variable: %w", err)
	}
	return &variable, nil
}

// ListByService retrieves all environment variables for a service and environment.
func (r *EnvironmentVariableRepository) ListByService(ctx context.Context, accountID, projectID, serviceID uuid.UUID, environment string) ([]model.EnvironmentVariable, error) {
	var variables []model.EnvironmentVariable
	err := r.db.NewSelect().
		Model(&variables).
		Where("account_id = ? AND project_id = ? AND service_id = ? AND environment = ?",
			accountID, projectID, serviceID, environment).
		Order("key ASC").
		Scan(ctx)
	if err != nil {
		return nil, fmt.Errorf("list environment variables: %w", err)
	}
	return variables, nil
}

// AuditReveal records secret value access with audit trail.
func (r *EnvironmentVariableRepository) AuditReveal(ctx context.Context, variable *model.EnvironmentVariable, actorID uuid.UUID) error {
	audit := &model.EnvironmentVariableAudit{
		AccountID:   variable.AccountID,
		ProjectID:   variable.ProjectID,
		ServiceID:   variable.ServiceID,
		VariableID:  &variable.ID,
		Action:      model.EnvAuditRevealed,
		Key:         variable.Key,
		IsSecret:    variable.IsSecret,
		Environment: variable.Environment,
		ActorID:     actorID,
		Metadata:    json.RawMessage(`{}`),
	}
	if _, err := r.db.NewInsert().Model(audit).Exec(ctx); err != nil {
		return fmt.Errorf("append reveal audit: %w", err)
	}
	return nil
}

// ListAudit retrieves audit trail for a service.
func (r *EnvironmentVariableRepository) ListAudit(ctx context.Context, accountID, projectID, serviceID uuid.UUID, limit int) ([]model.EnvironmentVariableAudit, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	var audits []model.EnvironmentVariableAudit
	err := r.db.NewSelect().
		Model(&audits).
		Where("account_id = ? AND project_id = ? AND service_id = ?",
			accountID, projectID, serviceID).
		Order("created_at DESC").
		Limit(limit).
		Scan(ctx)
	if err != nil {
		return nil, fmt.Errorf("list environment variable audit: %w", err)
	}
	return audits, nil
}
