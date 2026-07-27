package backup

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestResolveArchiveRejectsTraversalSymlinkAndUnknownNames(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.Chmod(dir, 0o700))
	name := "opencloud-20260727T010203Z-0123456789abcdef.dump.ocb"
	require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte("archive"), 0o600))

	path, err := resolveArchive(dir, name)
	require.NoError(t, err)
	require.Equal(t, filepath.Join(dir, name), path)

	_, err = resolveArchive(dir, "../"+name)
	require.Error(t, err)
	_, err = resolveArchive(dir, "customer.dump")
	require.Error(t, err)

	if runtime.GOOS != "windows" {
		require.NoError(t, os.Symlink(filepath.Join(dir, name), filepath.Join(dir, strings.Replace(name, "0123", "ffff", 1))))
		_, err = resolveArchive(dir, strings.Replace(name, "0123", "ffff", 1))
		require.Error(t, err)
	}
}

func TestPruneTouchesOnlyExpiredGeneratedRegularFiles(t *testing.T) {
	dir := t.TempDir()
	oldTime := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	newTime := time.Date(2026, 7, 26, 0, 0, 0, 0, time.UTC)
	cutoff := time.Date(2026, 7, 14, 0, 0, 0, 0, time.UTC)
	oldBase := "opencloud-20260701T000000Z-0123456789abcdef.dump.ocb"
	newBase := "opencloud-20260726T000000Z-fedcba9876543210.dump.ocb"
	orphanBase := "opencloud-20260701T000000Z-1111111111111111.dump.ocb"
	unknown := "keep-me.txt"
	for _, name := range []string{
		oldBase,
		oldBase + ".sha256",
		newBase,
		newBase + ".sha256",
		orphanBase,
		unknown,
	} {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(name), 0o600))
	}
	require.NoError(t, os.Chtimes(filepath.Join(dir, oldBase), oldTime, oldTime))
	require.NoError(t, os.Chtimes(filepath.Join(dir, oldBase+".sha256"), oldTime, oldTime))
	require.NoError(t, os.Chtimes(filepath.Join(dir, newBase), newTime, newTime))
	require.NoError(t, os.Chtimes(filepath.Join(dir, newBase+".sha256"), newTime, newTime))
	require.NoError(t, os.Chtimes(filepath.Join(dir, orphanBase), oldTime, oldTime))

	require.NoError(t, Prune(dir, cutoff))
	_, err := os.Stat(filepath.Join(dir, oldBase))
	require.ErrorIs(t, err, os.ErrNotExist)
	_, err = os.Stat(filepath.Join(dir, oldBase+".sha256"))
	require.ErrorIs(t, err, os.ErrNotExist)
	for _, name := range []string{newBase, newBase + ".sha256", orphanBase, unknown} {
		_, err = os.Stat(filepath.Join(dir, name))
		require.NoError(t, err)
	}
}

func TestPruneLeavesArchiveWhenChecksumIsSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation requires elevated Windows permissions")
	}
	dir := t.TempDir()
	oldTime := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	cutoff := time.Date(2026, 7, 14, 0, 0, 0, 0, time.UTC)
	base := "opencloud-20260701T000000Z-2222222222222222.dump.ocb"
	archive := filepath.Join(dir, base)
	require.NoError(t, os.WriteFile(archive, []byte("archive"), 0o600))
	require.NoError(t, os.Chtimes(archive, oldTime, oldTime))
	external := filepath.Join(dir, "external-checksum")
	require.NoError(t, os.WriteFile(external, []byte("checksum"), 0o600))
	require.NoError(t, os.Symlink(external, archive+".sha256"))

	require.NoError(t, Prune(dir, cutoff))
	_, err := os.Stat(archive)
	require.NoError(t, err)
	_, err = os.Lstat(archive + ".sha256")
	require.NoError(t, err)
}

func TestPostgresEnvironmentKeepsSecretsOutOfCommandArguments(t *testing.T) {
	database, env, err := postgresEnvironment(
		"postgres://backup-user:super-secret@db.internal:5544/opencloud?sslmode=require",
	)
	require.NoError(t, err)
	require.Equal(t, "opencloud", database)
	joined := strings.Join(env, "\n")
	require.Contains(t, joined, "PGHOST=db.internal")
	require.Contains(t, joined, "PGPORT=5544")
	require.Contains(t, joined, "PGUSER=backup-user")
	require.Contains(t, joined, "PGPASSWORD=super-secret")
	require.Contains(t, joined, "PGSSLMODE=require")
}

func TestSecureDirectoryRejectsRootRelativeAndSymlink(t *testing.T) {
	_, err := secureDirectory(".")
	require.Error(t, err)
	_, err = secureDirectory(string(filepath.Separator))
	require.Error(t, err)

	if runtime.GOOS != "windows" {
		target := t.TempDir()
		link := filepath.Join(t.TempDir(), "backup-link")
		require.NoError(t, os.Symlink(target, link))
		_, err = secureDirectory(link)
		require.Error(t, err)
	}
}

func TestWriteAndVerifyChecksumDetectsTamper(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "opencloud-20260727T010203Z-0123456789abcdef.dump.ocb")
	require.NoError(t, os.WriteFile(path, []byte("encrypted"), 0o600))
	digest, err := writeChecksum(path)
	require.NoError(t, err)
	require.Len(t, digest, 64)
	verified, err := verifyChecksum(path)
	require.NoError(t, err)
	require.Equal(t, digest, verified)

	require.NoError(t, os.WriteFile(path, []byte("tampered"), 0o600))
	_, err = verifyChecksum(path)
	require.Error(t, err)
}

func TestVerifyChecksumRejectsSymlinkSidecar(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation requires elevated Windows permissions")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "opencloud-20260727T010203Z-0123456789abcdef.dump.ocb")
	require.NoError(t, os.WriteFile(path, []byte("encrypted"), 0o600))
	other := filepath.Join(dir, "external-checksum")
	require.NoError(t, os.WriteFile(other, []byte(strings.Repeat("0", 64)+"  "+filepath.Base(path)+"\n"), 0o600))
	require.NoError(t, os.Symlink(other, path+".sha256"))

	_, err := verifyChecksum(path)
	require.Error(t, err)
	require.Contains(t, err.Error(), "regular file")
}

func TestBackupDirectoryLockRejectsConcurrentHolderAndRecovers(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.Chmod(dir, 0o700))

	first, err := acquireLock(dir)
	require.NoError(t, err)
	_, err = acquireLock(dir)
	require.Error(t, err)
	releaseLock(first)

	second, err := acquireLock(dir)
	require.NoError(t, err)
	releaseLock(second)
}
