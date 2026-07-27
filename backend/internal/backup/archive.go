package backup

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const (
	defaultRetentionDays = 14
	maxToolErrorBytes    = 4096
)

var archiveNamePattern = regexp.MustCompile(
	`^opencloud-\d{8}T\d{6}Z-[0-9a-f]{16}\.dump\.ocb$`,
)

// Manager creates, verifies, restores, and prunes encrypted pg_dump archives.
type Manager struct {
	DatabaseURL        string
	RestoreDatabaseURL string
	Directory          string
	TempDirectory      string
	Key                []byte
	RetentionDays      int
	Now                func() time.Time
}

// Result is safe to emit as structured command output.
type Result struct {
	File      string    `json:"file"`
	SHA256    string    `json:"sha256,omitempty"`
	CreatedAt time.Time `json:"created_at,omitempty"`
	Database  string    `json:"database,omitempty"`
}

// Backup runs pg_dump in custom format, encrypts the stream, publishes it
// atomically with a checksum, and prunes only matching expired archives.
func (m *Manager) Backup(ctx context.Context) (*Result, error) {
	if len(m.Key) != 32 {
		return nil, errors.New("backup encryption key must be exactly 32 bytes")
	}
	dir, err := secureDirectory(m.Directory)
	if err != nil {
		return nil, err
	}
	lock, err := acquireLock(dir)
	if err != nil {
		return nil, err
	}
	defer releaseLock(lock)

	now := m.now()
	name, err := newArchiveName(now)
	if err != nil {
		return nil, err
	}
	temp, err := os.CreateTemp(dir, ".opencloud-backup-*.tmp")
	if err != nil {
		return nil, fmt.Errorf("create backup temp file: %w", err)
	}
	tempPath := temp.Name()
	published := false
	defer func() {
		_ = temp.Close()
		if !published {
			_ = os.Remove(tempPath)
		}
	}()
	if err := temp.Chmod(0o600); err != nil {
		return nil, fmt.Errorf("secure backup temp file: %w", err)
	}

	database, childEnv, err := postgresEnvironment(m.DatabaseURL)
	if err != nil {
		return nil, err
	}
	cmd := exec.CommandContext(
		ctx,
		"pg_dump",
		"--format=custom",
		"--no-owner",
		"--no-privileges",
		"--dbname="+database,
	)
	cmd.Env = childEnv
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("open pg_dump output: %w", err)
	}
	var stderr limitedBuffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start pg_dump: %w", err)
	}
	encryptErr := Encrypt(temp, stdout, m.Key)
	if encryptErr != nil {
		_ = stdout.Close()
	}
	waitErr := cmd.Wait()
	if encryptErr != nil {
		return nil, encryptErr
	}
	if waitErr != nil {
		return nil, toolError("pg_dump failed", waitErr, stderr.String())
	}
	if err := temp.Sync(); err != nil {
		return nil, fmt.Errorf("sync encrypted backup: %w", err)
	}
	if err := temp.Close(); err != nil {
		return nil, fmt.Errorf("close encrypted backup: %w", err)
	}

	finalPath := filepath.Join(dir, name)
	if err := os.Rename(tempPath, finalPath); err != nil {
		return nil, fmt.Errorf("publish encrypted backup: %w", err)
	}
	published = true
	digest, err := writeChecksum(finalPath)
	if err != nil {
		_ = os.Remove(finalPath)
		published = false
		return nil, err
	}
	if err := syncDirectory(dir); err != nil {
		return nil, err
	}
	if err := Prune(dir, now.AddDate(0, 0, -m.retentionDays())); err != nil {
		return nil, err
	}
	return &Result{File: name, SHA256: digest, CreatedAt: now, Database: database}, nil
}

// Verify authenticates an archive, checks its SHA-256 sidecar, and asks
// pg_restore to parse the complete custom-format catalog.
func (m *Manager) Verify(ctx context.Context, name string) (*Result, error) {
	path, err := resolveArchive(m.Directory, name)
	if err != nil {
		return nil, err
	}
	digest, err := verifyChecksum(path)
	if err != nil {
		return nil, err
	}
	plainPath, cleanup, err := m.decryptToTemp(path)
	if err != nil {
		return nil, err
	}
	defer cleanup()

	cmd := exec.CommandContext(ctx, "pg_restore", "--list", plainPath)
	cmd.Env = sanitizedEnvironment()
	var stderr limitedBuffer
	cmd.Stdout = io.Discard
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, toolError("pg_restore catalog verification failed", err, stderr.String())
	}
	return &Result{File: filepath.Base(path), SHA256: digest}, nil
}

// Restore decrypts and restores an archive into an explicitly confirmed target
// database. The caller must enforce the destructive confirmation gate.
func (m *Manager) Restore(ctx context.Context, name, confirmedDatabase string) (*Result, error) {
	path, err := resolveArchive(m.Directory, name)
	if err != nil {
		return nil, err
	}
	if _, err := verifyChecksum(path); err != nil {
		return nil, err
	}
	database, childEnv, err := postgresEnvironment(m.RestoreDatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("invalid restore database configuration: %w", err)
	}
	if confirmedDatabase == "" || confirmedDatabase != database {
		return nil, errors.New("RESTORE_CONFIRM_DATABASE must exactly match the target database name")
	}
	plainPath, cleanup, err := m.decryptToTemp(path)
	if err != nil {
		return nil, err
	}
	defer cleanup()

	cmd := exec.CommandContext(
		ctx,
		"pg_restore",
		"--clean",
		"--if-exists",
		"--no-owner",
		"--no-privileges",
		"--exit-on-error",
		"--dbname="+database,
		plainPath,
	)
	cmd.Env = childEnv
	var stderr limitedBuffer
	cmd.Stdout = io.Discard
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, toolError("pg_restore failed", err, stderr.String())
	}
	return &Result{File: filepath.Base(path), Database: database}, nil
}

// Prune removes only regular archive/checksum pairs older than cutoff. Unknown
// files, directories, and symlinks are never touched.
func Prune(directory string, cutoff time.Time) error {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return fmt.Errorf("read backup directory for retention: %w", err)
	}
	for _, entry := range entries {
		name := entry.Name()
		if !archiveNamePattern.MatchString(name) {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return fmt.Errorf("inspect retention candidate: %w", err)
		}
		if !info.Mode().IsRegular() || !info.ModTime().Before(cutoff) {
			continue
		}
		sidecar := filepath.Join(directory, name+".sha256")
		sidecarInfo, err := os.Lstat(sidecar)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return fmt.Errorf("inspect expired backup checksum: %w", err)
		}
		if !sidecarInfo.Mode().IsRegular() {
			continue
		}
		if err := os.Remove(filepath.Join(directory, name)); err != nil {
			return fmt.Errorf("remove expired backup artifact: %w", err)
		}
		if err := os.Remove(sidecar); err != nil {
			return fmt.Errorf("remove expired backup checksum: %w", err)
		}
	}
	return nil
}

func (m *Manager) decryptToTemp(path string) (string, func(), error) {
	tempDir := m.TempDirectory
	if tempDir == "" {
		tempDir = os.TempDir()
	}
	temp, err := os.CreateTemp(tempDir, "opencloud-restore-*.dump")
	if err != nil {
		return "", func() {}, fmt.Errorf("create restore temp file: %w", err)
	}
	cleanup := func() {
		_ = temp.Close()
		_ = os.Remove(temp.Name())
	}
	if err := temp.Chmod(0o600); err != nil {
		cleanup()
		return "", func() {}, fmt.Errorf("secure restore temp file: %w", err)
	}
	encrypted, err := os.Open(path)
	if err != nil {
		cleanup()
		return "", func() {}, fmt.Errorf("open encrypted backup: %w", err)
	}
	defer func() { _ = encrypted.Close() }()
	if err := Decrypt(temp, encrypted, m.Key); err != nil {
		cleanup()
		return "", func() {}, err
	}
	if err := temp.Sync(); err != nil {
		cleanup()
		return "", func() {}, fmt.Errorf("sync decrypted restore file: %w", err)
	}
	if err := temp.Close(); err != nil {
		cleanup()
		return "", func() {}, fmt.Errorf("close decrypted restore file: %w", err)
	}
	return temp.Name(), cleanup, nil
}

func secureDirectory(value string) (string, error) {
	if value == "" {
		return "", errors.New("BACKUP_DIR is required")
	}
	if !filepath.IsAbs(value) {
		return "", errors.New("BACKUP_DIR must be an absolute path")
	}
	clean := filepath.Clean(value)
	if clean == string(filepath.Separator) {
		return "", errors.New("BACKUP_DIR cannot be the filesystem root")
	}
	if err := os.MkdirAll(clean, 0o700); err != nil {
		return "", fmt.Errorf("create backup directory: %w", err)
	}
	info, err := os.Lstat(clean)
	if err != nil {
		return "", fmt.Errorf("inspect backup directory: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return "", errors.New("BACKUP_DIR must be a real directory, not a symlink")
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
		return "", errors.New("BACKUP_DIR must not grant group or world permissions")
	}
	return clean, nil
}

func resolveArchive(directory, name string) (string, error) {
	dir, err := secureDirectory(directory)
	if err != nil {
		return "", err
	}
	if filepath.Base(name) != name || !archiveNamePattern.MatchString(name) {
		return "", errors.New("BACKUP_FILE must be a generated archive filename without a path")
	}
	path := filepath.Join(dir, name)
	info, err := os.Lstat(path)
	if err != nil {
		return "", fmt.Errorf("inspect backup archive: %w", err)
	}
	if !info.Mode().IsRegular() {
		return "", errors.New("backup archive must be a regular file")
	}
	return path, nil
}

func newArchiveName(now time.Time) (string, error) {
	var suffix [8]byte
	if _, err := io.ReadFull(rand.Reader, suffix[:]); err != nil {
		return "", fmt.Errorf("generate backup filename: %w", err)
	}
	return fmt.Sprintf(
		"opencloud-%s-%s.dump.ocb",
		now.UTC().Format("20060102T150405Z"),
		hex.EncodeToString(suffix[:]),
	), nil
}

func writeChecksum(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("open backup for checksum: %w", err)
	}
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		_ = file.Close()
		return "", fmt.Errorf("hash encrypted backup: %w", err)
	}
	if err := file.Close(); err != nil {
		return "", fmt.Errorf("close encrypted backup after hash: %w", err)
	}
	digest := hex.EncodeToString(hash.Sum(nil))
	sidecar := path + ".sha256"
	temp, err := os.CreateTemp(filepath.Dir(path), ".opencloud-checksum-*.tmp")
	if err != nil {
		return "", fmt.Errorf("create checksum temp file: %w", err)
	}
	tempName := temp.Name()
	defer func() {
		_ = temp.Close()
		_ = os.Remove(tempName)
	}()
	if err := temp.Chmod(0o600); err != nil {
		return "", fmt.Errorf("secure checksum temp file: %w", err)
	}
	if _, err := fmt.Fprintf(temp, "%s  %s\n", digest, filepath.Base(path)); err != nil {
		return "", fmt.Errorf("write backup checksum: %w", err)
	}
	if err := temp.Sync(); err != nil {
		return "", fmt.Errorf("sync backup checksum: %w", err)
	}
	if err := temp.Close(); err != nil {
		return "", fmt.Errorf("close backup checksum: %w", err)
	}
	if err := os.Rename(tempName, sidecar); err != nil {
		return "", fmt.Errorf("publish backup checksum: %w", err)
	}
	return digest, nil
}

func verifyChecksum(path string) (string, error) {
	sidecarInfo, err := os.Lstat(path + ".sha256")
	if err != nil {
		return "", fmt.Errorf("inspect backup checksum: %w", err)
	}
	if !sidecarInfo.Mode().IsRegular() {
		return "", errors.New("backup checksum must be a regular file")
	}
	sidecar, err := os.Open(path + ".sha256")
	if err != nil {
		return "", fmt.Errorf("open backup checksum: %w", err)
	}
	defer func() { _ = sidecar.Close() }()
	scanner := bufio.NewScanner(io.LimitReader(sidecar, 256))
	if !scanner.Scan() {
		return "", errors.New("backup checksum is empty")
	}
	fields := strings.Fields(scanner.Text())
	if len(fields) != 2 || fields[1] != filepath.Base(path) || len(fields[0]) != sha256.Size*2 {
		return "", errors.New("backup checksum format is invalid")
	}
	if scanner.Scan() || scanner.Err() != nil {
		return "", errors.New("backup checksum must contain exactly one record")
	}
	file, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("open backup for verification: %w", err)
	}
	defer func() { _ = file.Close() }()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", fmt.Errorf("verify backup checksum: %w", err)
	}
	actual := hex.EncodeToString(hash.Sum(nil))
	if actual != fields[0] {
		return "", errors.New("backup checksum mismatch")
	}
	return actual, nil
}

func postgresEnvironment(rawURL string) (string, []string, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "", nil, errors.New("invalid PostgreSQL URL")
	}
	if parsed.Scheme != "postgres" && parsed.Scheme != "postgresql" {
		return "", nil, errors.New("database URL must use postgres:// or postgresql://")
	}
	if parsed.Hostname() == "" || parsed.User == nil {
		return "", nil, errors.New("database URL must include host and user")
	}
	database := strings.TrimPrefix(parsed.EscapedPath(), "/")
	database, err = url.PathUnescape(database)
	if err != nil || database == "" || strings.Contains(database, "/") {
		return "", nil, errors.New("database URL must include one database name")
	}
	port := parsed.Port()
	if port == "" {
		port = "5432"
	}
	if _, err := strconv.ParseUint(port, 10, 16); err != nil {
		return "", nil, errors.New("database URL has an invalid port")
	}
	password, _ := parsed.User.Password()
	values := map[string]string{
		"PGHOST":     parsed.Hostname(),
		"PGPORT":     port,
		"PGUSER":     parsed.User.Username(),
		"PGPASSWORD": password,
		"PGDATABASE": database,
	}
	queryMappings := map[string]string{
		"sslmode":         "PGSSLMODE",
		"sslrootcert":     "PGSSLROOTCERT",
		"sslcert":         "PGSSLCERT",
		"sslkey":          "PGSSLKEY",
		"connect_timeout": "PGCONNECT_TIMEOUT",
	}
	for queryKey, envKey := range queryMappings {
		if value := parsed.Query().Get(queryKey); value != "" {
			values[envKey] = value
		}
	}
	env := sanitizedEnvironment()
	for name, value := range values {
		env = append(env, name+"="+value)
	}
	return database, env, nil
}

func sanitizedEnvironment() []string {
	blocked := map[string]struct{}{
		"BACKUP_ENCRYPTION_KEY": {},
		"DATABASE_URL":          {},
		"RESTORE_DATABASE_URL":  {},
		"PGPASSWORD":            {},
	}
	env := make([]string, 0, len(os.Environ()))
	for _, item := range os.Environ() {
		name, _, ok := strings.Cut(item, "=")
		if _, skip := blocked[name]; ok && skip {
			continue
		}
		env = append(env, item)
	}
	return env
}

func syncDirectory(directory string) error {
	dir, err := os.Open(directory)
	if err != nil {
		return fmt.Errorf("open backup directory for sync: %w", err)
	}
	defer func() { _ = dir.Close() }()
	if err := dir.Sync(); err != nil {
		return fmt.Errorf("sync backup directory: %w", err)
	}
	return nil
}

func (m *Manager) now() time.Time {
	if m.Now != nil {
		return m.Now().UTC()
	}
	return time.Now().UTC()
}

func (m *Manager) retentionDays() int {
	if m.RetentionDays <= 0 {
		return defaultRetentionDays
	}
	return m.RetentionDays
}

func toolError(prefix string, cause error, stderr string) error {
	stderr = strings.TrimSpace(stderr)
	if stderr == "" {
		return fmt.Errorf("%s: %w", prefix, cause)
	}
	return fmt.Errorf("%s: %s: %w", prefix, stderr, cause)
}

type limitedBuffer struct {
	data []byte
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	originalLength := len(p)
	remaining := maxToolErrorBytes - len(b.data)
	if remaining > 0 {
		if len(p) > remaining {
			p = p[:remaining]
		}
		b.data = append(b.data, p...)
	}
	return originalLength, nil
}

func (b *limitedBuffer) String() string { return string(b.data) }
