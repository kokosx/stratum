package media

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"sync"
)

// serveMeta is the cached metadata needed to serve a stored variant without DB.
type serveMeta struct {
	StorageKey string
	MIME       string
	Size       int64
	ETag       string
}

// serveCache is a small immutable-entry cache for media serving metadata.
// It stores only metadata (storage key, mime, size, etag), never image bytes.
// Zero DB on warm after first load; OS page cache handles bytes.
type serveCache struct {
	mu      sync.RWMutex
	entries map[string]serveMeta
}

func newServeCache() *serveCache {
	return &serveCache{entries: make(map[string]serveMeta)}
}

func serveKey(id, kind string) string {
	if kind == "" {
		kind = "original"
	}
	// strip query string
	if idx := strings.Index(kind, "?"); idx != -1 {
		kind = kind[:idx]
	}
	return id + "\x00" + kind
}

func (c *serveCache) get(id, kind string) (serveMeta, bool) {
	k := serveKey(id, kind)
	c.mu.RLock()
	v, ok := c.entries[k]
	c.mu.RUnlock()
	return v, ok
}

func (c *serveCache) set(id, kind string, meta serveMeta) {
	k := serveKey(id, kind)
	c.mu.Lock()
	c.entries[k] = meta
	c.mu.Unlock()
}

func (c *serveCache) delete(id, kind string) {
	k := serveKey(id, kind)
	c.mu.Lock()
	delete(c.entries, k)
	c.mu.Unlock()
}

func (c *serveCache) deleteByMediaID(id string) {
	prefix := id + "\x00"
	c.mu.Lock()
	for k := range c.entries {
		if strings.HasPrefix(k, prefix) {
			delete(c.entries, k)
		}
	}
	c.mu.Unlock()
}

func (c *serveCache) invalidateAll() {
	c.mu.Lock()
	c.entries = make(map[string]serveMeta)
	c.mu.Unlock()
}

// etagFromHash builds a quoted ETag from a content hash (12 hex chars stored on variant).
func etagFromHash(hash string) string {
	if hash == "" {
		return ""
	}
	return `"` + hash + `"`
}

// etagForOriginal builds a stable ETag for originals (write-once). Use hash of storage key.
func etagForOriginal(storageKey string, size int64) string {
	h := sha256.Sum256([]byte(storageKey))
	return `"` + hex.EncodeToString(h[:8]) + `"`
}
