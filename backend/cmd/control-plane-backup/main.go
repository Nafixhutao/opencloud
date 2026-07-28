// Command control-plane-backup creates, verifies, schedules, and restores
// encrypted PostgreSQL control-plane archives. It never prints connection URLs
// or encryption keys.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/nazxf/opencloud/backend/internal/backup"
)

const (
	defaultBackupInterval = 24 * time.Hour
	minimumBackupInterval = 5 * time.Minute
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()
	if err := run(ctx, os.Args[1:]); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "control-plane backup operation failed:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string) error {
	if len(args) != 1 {
		return errors.New("usage: control-plane-backup backup|schedule|verify|restore")
	}
	manager, err := managerFromEnvironment()
	if err != nil {
		return err
	}
	switch args[0] {
	case "backup":
		result, err := manager.Backup(ctx)
		return output(result, err)
	case "schedule":
		return schedule(ctx, manager)
	case "verify":
		result, err := manager.Verify(ctx, os.Getenv("BACKUP_FILE"))
		return output(result, err)
	case "restore":
		if os.Getenv("ALLOW_DESTRUCTIVE_RESTORE") != "restore-to-confirmed-empty-target" {
			return errors.New(
				"ALLOW_DESTRUCTIVE_RESTORE must equal restore-to-confirmed-empty-target",
			)
		}
		result, err := manager.Restore(
			ctx,
			os.Getenv("BACKUP_FILE"),
			os.Getenv("RESTORE_CONFIRM_DATABASE"),
		)
		return output(result, err)
	default:
		return errors.New("usage: control-plane-backup backup|schedule|verify|restore")
	}
}

func managerFromEnvironment() (*backup.Manager, error) {
	key, err := backup.DecodeKey(strings.TrimSpace(os.Getenv("BACKUP_ENCRYPTION_KEY")))
	if err != nil {
		return nil, err
	}
	retention, err := integerEnvironment("BACKUP_RETENTION_DAYS", 14, 1, 3650)
	if err != nil {
		return nil, err
	}
	return &backup.Manager{
		DatabaseURL:        os.Getenv("DATABASE_URL"),
		RestoreDatabaseURL: os.Getenv("RESTORE_DATABASE_URL"),
		Directory:          os.Getenv("BACKUP_DIR"),
		TempDirectory:      os.Getenv("RESTORE_TEMP_DIR"),
		Key:                key,
		RetentionDays:      retention,
	}, nil
}

func schedule(ctx context.Context, manager *backup.Manager) error {
	intervalSeconds, err := integerEnvironment(
		"BACKUP_INTERVAL_SECONDS",
		int(defaultBackupInterval/time.Second),
		int(minimumBackupInterval/time.Second),
		int((30*24*time.Hour)/time.Second),
	)
	if err != nil {
		return err
	}
	interval := time.Duration(intervalSeconds) * time.Second
	for {
		result, err := manager.Backup(ctx)
		if err != nil {
			return err
		}
		if err := output(result, nil); err != nil {
			return err
		}
		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return nil
		case <-timer.C:
		}
	}
}

func integerEnvironment(name string, fallback, minimum, maximum int) (int, error) {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < minimum || value > maximum {
		return 0, fmt.Errorf("%s must be an integer between %d and %d", name, minimum, maximum)
	}
	return value, nil
}

func output(result *backup.Result, err error) error {
	if err != nil {
		return err
	}
	if result == nil {
		return errors.New("backup operation returned no result")
	}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetEscapeHTML(false)
	return encoder.Encode(result)
}
