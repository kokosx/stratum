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
	Brotli      []byte
	ETag        string
	Robots      string
	ContentType string
	// Tags are used for selective invalidation and are not sent to clients.
	Tags []string `json:"-"`
}

// Cache is a concurrency-safe, in-memory full-page cache. It is safe for
// concurrent use by many request goroutines.
// It supports optional tag-based invalidation: each entry may carry tags such
// as "entry:<id>", "content-type:post", "site", "navigation", "theme".
type Cache struct {
	mu       sync.RWMutex
	entries  map[string]Entry
	inflight map[string]*call
	tagIndex map[string]map[string]struct{}
	keyTags  map[string]map[string]struct{}
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
		tagIndex: make(map[string]map[string]struct{}),
		keyTags:  make(map[string]map[string]struct{}),
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

// Set stores a rendered entry with optional tags.
func (c *Cache) Set(key string, e Entry, tags ...string) {
	effectiveTags := tags
	if len(effectiveTags) == 0 && len(e.Tags) > 0 {
		effectiveTags = e.Tags
	}
	c.mu.Lock()
	// remove old tags for this key
	if old, ok := c.keyTags[key]; ok {
		for t := range old {
			if set, ok2 := c.tagIndex[t]; ok2 {
				delete(set, key)
				if len(set) == 0 {
					delete(c.tagIndex, t)
				}
			}
		}
	}
	c.entries[key] = e
	if len(effectiveTags) > 0 {
		if c.tagIndex == nil {
			c.tagIndex = make(map[string]map[string]struct{})
		}
		if c.keyTags == nil {
			c.keyTags = make(map[string]map[string]struct{})
		}
		tagSet := make(map[string]struct{}, len(effectiveTags))
		for _, t := range effectiveTags {
			if t == "" {
				continue
			}
			tagSet[t] = struct{}{}
			if c.tagIndex[t] == nil {
				c.tagIndex[t] = make(map[string]struct{})
			}
			c.tagIndex[t][key] = struct{}{}
		}
		c.keyTags[key] = tagSet
	} else {
		delete(c.keyTags, key)
	}
	c.mu.Unlock()
}

// InvalidateAll drops every cached page. It is called after any change that can
// affect the rendered HTML (publish, settings, theme, navigation, media, ...).
func (c *Cache) InvalidateAll() {
	c.mu.Lock()
	c.entries = make(map[string]Entry)
	c.tagIndex = make(map[string]map[string]struct{})
	c.keyTags = make(map[string]map[string]struct{})
	c.mu.Unlock()
}

// InvalidateTag drops all cached pages carrying the given tag.
func (c *Cache) InvalidateTag(tag string) {
	c.mu.Lock()
	keys, ok := c.tagIndex[tag]
	if !ok {
		c.mu.Unlock()
		return
	}
	for k := range keys {
		delete(c.entries, k)
		// remove key from all other tag indexes
		if tags, ok2 := c.keyTags[k]; ok2 {
			for t := range tags {
				if t == tag {
					continue
				}
				if set, ok3 := c.tagIndex[t]; ok3 {
					delete(set, k)
					if len(set) == 0 {
						delete(c.tagIndex, t)
					}
				}
			}
			delete(c.keyTags, k)
		}
	}
	delete(c.tagIndex, tag)
	c.mu.Unlock()
}

// InvalidateTags drops all cached pages carrying any of the given tags.
func (c *Cache) InvalidateTags(tags ...string) {
	for _, t := range tags {
		c.InvalidateTag(t)
	}
}

// Entries returns the number of cached pages.
func (c *Cache) Entries() int {
	c.mu.RLock()
	n := len(c.entries)
	c.mu.RUnlock()
	return n
}

// ApproxBytes returns an approximate memory usage in bytes for stored entries.
func (c *Cache) ApproxBytes() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	total := 0
	for _, e := range c.entries {
		total += len(e.HTML) + len(e.Gzip) + len(e.Brotli) + len(e.ETag) + len(e.Robots) + len(e.ContentType)
	}
	return total
}

// ComputeETag derives a stable ETag from final HTML bytes.
func ComputeETag(html []byte) string {
	sum := sha256.Sum256(html)
	return `"` + hex.EncodeToString(sum[:16]) + `"`
}

// Do coalesces concurrent misses for the same key. fn renders the page; its
// result (and error) is shared by every waiter. The entry is not stored on
// error so a later request can retry. Tags stored in Entry.Tags are indexed.
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
		// store with tag indexing (same logic as Set but without second lock)
		if old, ok := c.keyTags[key]; ok {
			for t := range old {
				if set, ok2 := c.tagIndex[t]; ok2 {
					delete(set, key)
					if len(set) == 0 {
						delete(c.tagIndex, t)
					}
				}
			}
		}
		c.entries[key] = inf.val
		tags := inf.val.Tags
		if len(tags) > 0 {
			if c.tagIndex == nil {
				c.tagIndex = make(map[string]map[string]struct{})
			}
			if c.keyTags == nil {
				c.keyTags = make(map[string]map[string]struct{})
			}
			tagSet := make(map[string]struct{}, len(tags))
			for _, t := range tags {
				if t == "" {
					continue
				}
				tagSet[t] = struct{}{}
				if c.tagIndex[t] == nil {
					c.tagIndex[t] = make(map[string]struct{})
				}
				c.tagIndex[t][key] = struct{}{}
			}
			c.keyTags[key] = tagSet
		} else {
			delete(c.keyTags, key)
		}
	}
	c.mu.Unlock()

	if inf.err != nil {
		return Entry{}, inf.err
	}
	return inf.val, nil
}
