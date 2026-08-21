package media

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Storage is the small, deliberate abstraction over where blobs live. The first
// implementation is the local filesystem, but an S3-compatible backend can be
// dropped in later without touching the rest of the media domain.
type Storage interface {
	Put(ctx context.Context, key string, data []byte) error
	Read(ctx context.Context, key string) ([]byte, error)
	Delete(ctx context.Context, key string) error
	Exists(ctx context.Context, key string) bool
}

// LocalStorage keeps blobs under a root directory split into originals/ and
// generated/. Keys are server-generated random tokens, never user input, so the
// path can never escape the root.
type LocalStorage struct {
	root string
}

func NewLocalStorage(root string) (*LocalStorage, error) {
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, fmt.Errorf("create media storage root: %w", err)
	}
	return &LocalStorage{root: root}, nil
}

// safePath resolves a storage key to an absolute path inside the root. It rejects
// anything that could traverse out of the root.
func (s *LocalStorage) safePath(key string) (string, error) {
	if key == "" || strings.Contains(key, "..") || filepath.IsAbs(key) {
		return "", fmt.Errorf("invalid storage key %q", key)
	}
	full := filepath.Join(s.root, filepath.Clean(key))
	if !strings.HasPrefix(full, filepath.Clean(s.root)+string(os.PathSeparator)) && full != filepath.Clean(s.root) {
		return "", fmt.Errorf("storage key escapes root: %q", key)
	}
	return full, nil
}

func (s *LocalStorage) Put(ctx context.Context, key string, data []byte) error {
	path, err := s.safePath(key)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create media directory: %w", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("write media file: %w", err)
	}
	return nil
}

func (s *LocalStorage) Read(ctx context.Context, key string) ([]byte, error) {
	path, err := s.safePath(key)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read media file: %w", err)
	}
	return data, nil
}

func (s *LocalStorage) Delete(ctx context.Context, key string) error {
	path, err := s.safePath(key)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("delete media file: %w", err)
	}
	return nil
}

func (s *LocalStorage) Exists(ctx context.Context, key string) bool {
	path, err := s.safePath(key)
	if err != nil {
		return false
	}
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
