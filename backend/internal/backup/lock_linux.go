//go:build linux

package backup

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

func acquireLock(directory string) (*os.File, error) {
	path := filepath.Join(directory, ".opencloud-backup.lock")
	file, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open backup lock: %w", err)
	}
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = file.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, errors.New("another backup process holds the directory lock")
		}
		return nil, fmt.Errorf("acquire backup lock: %w", err)
	}
	if err := file.Truncate(0); err == nil {
		_, _ = fmt.Fprintf(file, "pid=%d\n", os.Getpid())
	}
	return file, nil
}

func releaseLock(lock *os.File) {
	if lock == nil {
		return
	}
	_ = syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
	_ = lock.Close()
}
