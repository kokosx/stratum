//go:build !windows

package datalock

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// Acquire obtains a non-blocking exclusive lock for dataDir.
func Acquire(dataDir string) (*Lock, error) {
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, fmt.Errorf("create data directory: %w", err)
	}
	f, err := os.OpenFile(filepath.Join(dataDir, ".stratum.lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open data lock: %w", err)
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("acquire data lock: %w", err)
	}
	return &Lock{file: f}, nil
}

// Close releases the lock.
func (l *Lock) Close() error {
	if l == nil || l.file == nil {
		return nil
	}
	if err := syscall.Flock(int(l.file.Fd()), syscall.LOCK_UN); err != nil {
		_ = l.file.Close()
		return fmt.Errorf("unlock data directory: %w", err)
	}
	if err := l.file.Close(); err != nil {
		return fmt.Errorf("close data lock: %w", err)
	}
	l.file = nil
	return nil
}
