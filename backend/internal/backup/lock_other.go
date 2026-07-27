//go:build !linux

package backup

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

func acquireLock(directory string) (*os.File, error) {
	path := filepath.Join(directory, ".opencloud-backup.lock")
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if errors.Is(err, os.ErrExist) {
		return nil, errors.New("another backup process holds the directory lock")
	}
	if err != nil {
		return nil, fmt.Errorf("acquire backup lock: %w", err)
	}
	return file, nil
}

func releaseLock(lock *os.File) {
	if lock == nil {
		return
	}
	name := lock.Name()
	_ = lock.Close()
	_ = os.Remove(name)
}
