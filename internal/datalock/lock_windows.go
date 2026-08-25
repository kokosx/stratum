//go:build windows

package datalock

import "fmt"

// Acquire is intentionally unsupported until the Windows lock implementation is
// added; this preserves cross-compilation without pretending restore is safe.
func Acquire(string) (*Lock, error) {
	return nil, fmt.Errorf("data directory locking is not supported on Windows")
}

func (l *Lock) Close() error { return nil }
