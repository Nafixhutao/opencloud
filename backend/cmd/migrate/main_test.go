package main

import (
	"context"
	"strings"
	"testing"

	"go.uber.org/zap"
)

func TestMissingPhase3MaintenanceAck(t *testing.T) {
	tests := []struct {
		name       string
		migration  string
		production bool
		ack        string
		missing    bool
	}{
		{
			name:       "production phase3 is fail closed",
			migration:  phase3DomainsMigration,
			production: true,
			missing:    true,
		},
		{
			name:       "wrong acknowledgement is rejected",
			migration:  phase3DomainsMigration,
			production: true,
			ack:        "yes",
			missing:    true,
		},
		{
			name:       "documented acknowledgement unlocks only phase3",
			migration:  phase3DomainsMigration,
			production: true,
			ack:        phase3MaintenanceAck,
		},
		{
			name:       "development remains automation friendly",
			migration:  phase3DomainsMigration,
			production: false,
		},
		{
			name:       "unrelated production migrations do not require phase3 acknowledgement",
			migration:  "20260701000000_other",
			production: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := missingPhase3MaintenanceAck(
				test.migration,
				test.production,
				test.ack,
			); got != test.missing {
				t.Fatalf("missingPhase3MaintenanceAck() = %v, want %v", got, test.missing)
			}
		})
	}
}

func TestProductionDownIsRejectedBeforeRollback(t *testing.T) {
	err := run(context.Background(), zap.NewNop(), nil, "down", true, "")
	if err == nil || !strings.Contains(err.Error(), "disabled in production") {
		t.Fatalf("run(production down) error = %v", err)
	}
}
