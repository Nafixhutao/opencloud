package service

import (
	"context"
	"database/sql"
	"fmt"
	"regexp"
	"strings"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/Nafixhutao/opencloud/backend/internal/apperr"
	"github.com/Nafixhutao/opencloud/backend/internal/credential"
	"github.com/Nafixhutao/opencloud/backend/internal/model"
	"github.com/Nafixhutao/opencloud/backend/internal/repository"
)

var (
	envKeyPattern = regexp.MustCompile(`^[A-Z][A-Z0-9_]{0,127}$`)
	// Reserved prefixes that should not be exposed to users via NEXT_PUBLIC pattern
	reservedPrefixes = []string{"DATABASE_", "REDIS_", "OPENCLOUD_", "INTERNAL_"}
)

// EnvironmentVariableService manages tenant-scoped environment variables and secrets.
type EnvironmentVariableService struct {
	log        *zap.Logger
	envRepo    *repository.EnvironmentVariableRepository
	projectRepo *repository.ProjectRepository
	cipher     *credential.Cipher
}

// NewEnvironmentVariableService creates a service for environment variables.
func NewEnvironmentVariableService(
	log *zap.Logger,
	envRepo *repository.EnvironmentVariableRepository,
	projectRepo *repository.ProjectRepository,
	cipher *credential.Cipher,
) *EnvironmentVariableService {
	return &EnvironmentVariableService{
		log:        log,
		envRepo:    envRepo,
		projectRepo: projectRepo,
		cipher:     cipher,
	}
}

// CreateVariable creates a new environment variable or secret.
func (s *EnvironmentVariableService) CreateVariable(
	ctx context.Context,
	accountID, userID, projectID, serviceID uuid.UUID,
	key, value, environment string,
	isSecret bool,
) (*model.EnvironmentVariable, error) {
	// Validate inputs
	if err := s.validateKey(key); err != nil {
		return nil, err
	}
	if err := s.validateEnvironment(environment); err != nil {
		return nil, err
	}
	if value == "" {
		return nil, apperr.Validation("value cannot be empty")
	}

	// Verify service ownership
	service, err := s.projectRepo.GetServiceByID(ctx, accountID, serviceID)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, apperr.NotFound("service")
		}
		return nil, fmt.Errorf("verify service ownership: %w", err)
	}
	if service.ProjectID != projectID {
		return nil, apperr.NotFound("service")
	}

	variable := &model.EnvironmentVariable{
		AccountID:   accountID,
		ProjectID:   projectID,
		ServiceID:   serviceID,
		Key:         strings.ToUpper(strings.TrimSpace(key)),
		Environment: environment,
		IsSecret:    isSecret,
		CreatedBy:   userID,
	}

	if isSecret {
		// Encrypt secret value
		encrypted, err := s.cipher.Encrypt(serviceID, []byte(value))
		if err != nil {
			s.log.Error("encrypt secret failed", zap.Error(err))
			return nil, apperr.Internal("failed to encrypt secret")
		}
		variable.EncryptedValue = encrypted
	} else {
		variable.Value = &value
	}

	if err := s.envRepo.Create(ctx, variable, userID); err != nil {
		return nil, fmt.Errorf("create environment variable: %w", err)
	}

	s.log.Info("environment variable created",
		zap.String("account_id", accountID.String()),
		zap.String("service_id", serviceID.String()),
		zap.String("key", variable.Key),
		zap.Bool("is_secret", isSecret),
		zap.String("environment", environment))

	return variable, nil
}

// UpdateVariable updates an existing environment variable or secret.
func (s *EnvironmentVariableService) UpdateVariable(
	ctx context.Context,
	accountID, userID, id uuid.UUID,
	value string,
) (*model.EnvironmentVariable, error) {
	if value == "" {
		return nil, apperr.Validation("value cannot be empty")
	}

	// Get existing variable
	variable, err := s.envRepo.GetByID(ctx, accountID, id)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, apperr.NotFound("environment variable")
		}
		return nil, fmt.Errorf("get environment variable: %w", err)
	}

	if variable.IsSecret {
		// Encrypt new secret value
		encrypted, err := s.cipher.Encrypt(variable.ServiceID, []byte(value))
		if err != nil {
			s.log.Error("encrypt secret failed", zap.Error(err))
			return nil, apperr.Internal("failed to encrypt secret")
		}
		variable.EncryptedValue = encrypted
		variable.Value = nil
	} else {
		variable.Value = &value
		variable.EncryptedValue = nil
	}

	if err := s.envRepo.Update(ctx, variable, userID); err != nil {
		return nil, fmt.Errorf("update environment variable: %w", err)
	}

	s.log.Info("environment variable updated",
		zap.String("account_id", accountID.String()),
		zap.String("variable_id", id.String()),
		zap.String("key", variable.Key),
		zap.Bool("is_secret", variable.IsSecret))

	return variable, nil
}

// DeleteVariable removes an environment variable.
func (s *EnvironmentVariableService) DeleteVariable(
	ctx context.Context,
	accountID, userID, id uuid.UUID,
) error {
	if err := s.envRepo.Delete(ctx, accountID, id, userID); err != nil {
		if err == sql.ErrNoRows {
			return apperr.NotFound("environment variable")
		}
		return fmt.Errorf("delete environment variable: %w", err)
	}

	s.log.Info("environment variable deleted",
		zap.String("account_id", accountID.String()),
		zap.String("variable_id", id.String()))

	return nil
}

// ListVariables retrieves all environment variables for a service and environment.
func (s *EnvironmentVariableService) ListVariables(
	ctx context.Context,
	accountID, projectID, serviceID uuid.UUID,
	environment string,
) ([]model.EnvironmentVariable, error) {
	// Verify service ownership
	service, err := s.projectRepo.GetServiceByID(ctx, accountID, serviceID)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, apperr.NotFound("service")
		}
		return nil, fmt.Errorf("verify service ownership: %w", err)
	}
	if service.ProjectID != projectID {
		return nil, apperr.NotFound("service")
	}

	if err := s.validateEnvironment(environment); err != nil {
		return nil, err
	}

	variables, err := s.envRepo.ListByService(ctx, accountID, projectID, serviceID, environment)
	if err != nil {
		return nil, fmt.Errorf("list environment variables: %w", err)
	}

	// Redact encrypted values from list response
	for i := range variables {
		variables[i].EncryptedValue = nil
	}

	return variables, nil
}

// RevealSecret decrypts and returns a secret value with audit trail.
func (s *EnvironmentVariableService) RevealSecret(
	ctx context.Context,
	accountID, userID, id uuid.UUID,
) (string, error) {
	variable, err := s.envRepo.GetByID(ctx, accountID, id)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", apperr.NotFound("environment variable")
		}
		return "", fmt.Errorf("get environment variable: %w", err)
	}

	if !variable.IsSecret {
		return "", apperr.Validation("variable is not a secret")
	}

	// Decrypt secret
	plaintext, err := s.cipher.Decrypt(variable.ServiceID, variable.EncryptedValue)
	if err != nil {
		s.log.Error("decrypt secret failed", zap.Error(err))
		return "", apperr.Internal("failed to decrypt secret")
	}

	// Audit the reveal
	if err := s.envRepo.AuditReveal(ctx, variable, userID); err != nil {
		s.log.Error("audit reveal failed", zap.Error(err))
		// Continue despite audit failure - the value was already decrypted
	}

	s.log.Info("secret revealed",
		zap.String("account_id", accountID.String()),
		zap.String("variable_id", id.String()),
		zap.String("key", variable.Key),
		zap.String("user_id", userID.String()))

	return string(plaintext), nil
}

// ListAudit retrieves audit trail for a service.
func (s *EnvironmentVariableService) ListAudit(
	ctx context.Context,
	accountID, projectID, serviceID uuid.UUID,
	limit int,
) ([]model.EnvironmentVariableAudit, error) {
	// Verify service ownership
	service, err := s.projectRepo.GetServiceByID(ctx, accountID, serviceID)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, apperr.NotFound("service")
		}
		return nil, fmt.Errorf("verify service ownership: %w", err)
	}
	if service.ProjectID != projectID {
		return nil, apperr.NotFound("service")
	}

	audits, err := s.envRepo.ListAudit(ctx, accountID, projectID, serviceID, limit)
	if err != nil {
		return nil, fmt.Errorf("list audit trail: %w", err)
	}

	return audits, nil
}

func (s *EnvironmentVariableService) validateKey(key string) error {
	key = strings.TrimSpace(key)
	if key == "" {
		return apperr.Validation("key cannot be empty")
	}
	if !envKeyPattern.MatchString(key) {
		return apperr.Validation("key must start with uppercase letter and contain only uppercase letters, numbers, and underscores")
	}
	// Check for reserved prefixes
	upperKey := strings.ToUpper(key)
	for _, prefix := range reservedPrefixes {
		if strings.HasPrefix(upperKey, prefix) {
			return apperr.Validation(fmt.Sprintf("key cannot start with reserved prefix %s", prefix))
		}
	}
	return nil
}

func (s *EnvironmentVariableService) validateEnvironment(environment string) error {
	environment = strings.ToLower(strings.TrimSpace(environment))
	if environment != model.EnvProduction && environment != model.EnvPreview && environment != model.EnvDevelopment {
		return apperr.Validation("environment must be production, preview, or development")
	}
	return nil
}
