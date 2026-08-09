package dto

import (
	"time"

	"github.com/google/uuid"
)

// CreateEnvironmentVariableRequest is the payload for creating an environment variable.
type CreateEnvironmentVariableRequest struct {
	Key         string `json:"key" binding:"required"`
	Value       string `json:"value" binding:"required"`
	IsSecret    bool   `json:"is_secret"`
	Environment string `json:"environment" binding:"required"`
}

// UpdateEnvironmentVariableRequest is the payload for updating an environment variable.
type UpdateEnvironmentVariableRequest struct {
	Value string `json:"value" binding:"required"`
}

// EnvironmentVariableResponse is the safe public representation of an environment variable.
type EnvironmentVariableResponse struct {
	ID          uuid.UUID `json:"id"`
	Key         string    `json:"key"`
	Value       *string   `json:"value,omitempty"`
	IsSecret    bool      `json:"is_secret"`
	Environment string    `json:"environment"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// RevealSecretResponse contains the decrypted secret value.
type RevealSecretResponse struct {
	Value string `json:"value"`
}
