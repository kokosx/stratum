// Package pagecache provides an in-memory full-page cache for the public
// frontend. A cache entry stores the rendered HTML, a precompressed gzip copy,
// and the ETag so a cache HIT never touches the database, the JSON document, the
// block renderer, or the theme templates.
//
// The public content is immutable between publishes, so a simple whole-cache
// invalidation is sufficient. Misses for the same key are coalesced so 100
// concurrent requests for an uncached page produce a single render.
package pagecache

import (
	"crypto/sha256"
	"encoding/hex"
	"sync"
)

// Entry is a fully rendered, ready-to-serve page.
type Entry struct {
	HTML        []byte
	Gzip        []byte
	ETag        string
	Robots      string
	ContentType string
}

// Cache is a concurrency-safe, in-memory full-page cache. It is safe for
// concurrent use by many request goroutines.
type Cache struct {
	mu       sync.RWMutex
	entries  map[string]Entry
	inflight map[string]*call
}

type call struct {
	wg  sync.WaitGroup
	val Entry
	ok  bool
	err error
}

// New returns an empty cache.
func New() *Cache {
	return &Cache{
		entries:  make(map[string]Entry),
		inflight: make(map[string]*call),
	}
}

// Key derives a stable cache key. When the canonical URL depends on the request
// origin (empty configured Site URL) the origin is folded into the key so a
// page can never be served with the HTML generated for a different host.
func Key(origin, path string) string {
	if origin == "" {
		return path
	}
	return origin + "\x00" + path
}

// Get returns a cached entry and whether it was present.
func (c *Cache) Get(key string) (Entry, bool) {
	c.mu.RLock()
	e, ok := c.entries[key]
	c.mu.RUnlock()
	return e, ok
}

// Set stores a rendered entry.
func (c *Cache) Set(key string, e Entry) {
	c.mu.Lock()
	c.entries[key] = e
	c.mu.Unlock()
}

// InvalidateAll drops every cached page. It is called after any change that can
// affect the rendered HTML (publish, settings, theme, navigation, media, ...).
func (c *Cache) InvalidateAll() {
	c.mu.Lock()
	c.entries = make(map[string]Entry)
	c.mu.Unlock()
}

// ComputeETag derives a stable ETag from final HTML bytes.
func ComputeETag(html []byte) string {
	sum := sha256.Sum256(html)
	return `"` + hex.EncodeToString(sum[:16]) + `"`
}

// Do coalesces concurrent misses for the same key. fn renders the page; its
// result (and error) is shared by every waiter. The entry is not stored on
// error so a later request can retry.
func (c *Cache) Do(key string, fn func() (Entry, error)) (Entry, error) {
	c.mu.RLock()
	e, ok := c.entries[key]
	c.mu.RUnlock()
	if ok {
		return e, nil
	}

	c.mu.Lock()
	if inf, ok := c.inflight[key]; ok {
		c.mu.Unlock()
		inf.wg.Wait()
		if inf.err != nil {
			return Entry{}, inf.err
		}
		return inf.val, nil
	}
	inf := &call{}
	inf.wg.Add(1)
	c.inflight[key] = inf
	c.mu.Unlock()

	inf.val, inf.err = fn()
	inf.wg.Done()

	c.mu.Lock()
	delete(c.inflight, key)
	if inf.err == nil {
		c.entries[key] = inf.val
	}
	c.mu.Unlock()

	if inf.err != nil {
		return Entry{}, inf.err
	}
	return inf.val, nil
}
