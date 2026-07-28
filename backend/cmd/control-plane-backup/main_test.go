package main

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIntegerEnvironmentValidatesBounds(t *testing.T) {
	t.Setenv("TEST_INTERVAL", "")
	value, err := integerEnvironment("TEST_INTERVAL", 10, 5, 20)
	require.NoError(t, err)
	require.Equal(t, 10, value)

	t.Setenv("TEST_INTERVAL", "12")
	value, err = integerEnvironment("TEST_INTERVAL", 10, 5, 20)
	require.NoError(t, err)
	require.Equal(t, 12, value)

	for _, invalid := range []string{"4", "21", "not-a-number"} {
		t.Setenv("TEST_INTERVAL", invalid)
		_, err = integerEnvironment("TEST_INTERVAL", 10, 5, 20)
		require.Error(t, err)
	}
}

func TestRunRefusesRestoreWithoutExactDestructiveGate(t *testing.T) {
	t.Setenv("BACKUP_ENCRYPTION_KEY", "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY=")
	t.Setenv("BACKUP_DIR", t.TempDir())
	t.Setenv("BACKUP_FILE", "opencloud-20260727T010203Z-0123456789abcdef.dump.ocb")
	t.Setenv("ALLOW_DESTRUCTIVE_RESTORE", "yes")

	err := run(context.Background(), []string{"restore"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "ALLOW_DESTRUCTIVE_RESTORE")
}

func TestManagerEnvironmentDoesNotAcceptMalformedKey(t *testing.T) {
	t.Setenv("BACKUP_ENCRYPTION_KEY", "not-a-key")
	t.Setenv("BACKUP_DIR", t.TempDir())
	_, err := managerFromEnvironment()
	require.Error(t, err)
}

func TestMainUsageDoesNotReadSecrets(t *testing.T) {
	original := os.Args
	t.Cleanup(func() { os.Args = original })
	err := run(context.Background(), nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "usage")
}
