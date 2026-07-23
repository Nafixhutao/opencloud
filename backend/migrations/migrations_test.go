package migrations

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

var immutablePhase1Checksums = map[string]string{
	"20260723010000_create_account_memberships.up.sql":   "171ceca8893c004d38845be085405263539f603f9187be6d14353fcbeecad390",
	"20260723010000_create_account_memberships.down.sql": "c838dbb2591a5d0961b2b94bf14138fa207eadec62ab9ddf2c69e14650f0e280",
	"20260723020000_create_audit_logs.up.sql":            "4c60a91192944cac46a278df24ecd46473fe79d6ea74ae10adfa9150422346ff",
	"20260723020000_create_audit_logs.down.sql":          "cf7a72fea0c27cf0c3d81100abe79d5bc19d8be3b44402c88e3a54088b40ffc0",
}

func TestPhase1MigrationHistoryIsImmutable(t *testing.T) {
	for name, want := range immutablePhase1Checksums {
		content, err := sqlMigrations.ReadFile(name)
		require.NoError(t, err)
		sum := sha256.Sum256(content)
		require.Equal(
			t,
			want,
			hex.EncodeToString(sum[:]),
			"never edit an applied Phase 1 migration; add a new timestamped migration",
		)
	}
}

func TestCommittedMigrationChecksums(t *testing.T) {
	manifest, err := sqlMigrations.ReadFile("checksums.sha256")
	require.NoError(t, err)

	expected := make(map[string]string)
	scanner := bufio.NewScanner(bytes.NewReader(manifest))
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		require.Len(t, fields, 2)
		expected[fields[1]] = fields[0]
	}
	require.NoError(t, scanner.Err())

	entries, err := fs.ReadDir(sqlMigrations, ".")
	require.NoError(t, err)
	found := 0
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		found++
		content, err := sqlMigrations.ReadFile(entry.Name())
		require.NoError(t, err)
		sum := sha256.Sum256(content)
		require.Equal(
			t,
			expected[entry.Name()],
			hex.EncodeToString(sum[:]),
			"shipped migrations are immutable; add a new timestamped migration",
		)
	}
	require.Equal(t, len(expected), found, "every SQL migration must be checksummed")
}
