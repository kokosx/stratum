package runtimehub

import "sync"

// ArtifactCache is a generic, reusable in-memory cache for text artifacts
// (sitemap.xml, robots.txt, feed.xml). Each instance corresponds to one logical
// artifact; the three domain-named instances in Runtime (Sitemap, Robots, Feed)
// remain for readability, but they share this implementation so the code is not
// duplicated. The cache stores the rendered bytes, gzip + brotli copies, and an ETag.
type ArtifactCache struct {
	mu      sync.RWMutex
	body    []byte
	gzip    []byte
	brotli  []byte
	etag    string
	present bool
}

// NewArtifactCache creates an empty artifact cache.
func NewArtifactCache() *ArtifactCache { return &ArtifactCache{} }

// Get returns the cached artifact and whether it is present. Brotli is returned
// as the third return value extension via GetBrotli.
func (c *ArtifactCache) Get() ([]byte, []byte, string, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.body, c.gzip, c.etag, c.present
}

// GetBrotli returns body, gzip, brotli, etag, present.
func (c *ArtifactCache) GetBrotli() ([]byte, []byte, []byte, string, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.body, c.gzip, c.brotli, c.etag, c.present
}

// Set stores the artifact with gzip only (legacy); brotli is empty.
func (c *ArtifactCache) Set(body []byte, gzip []byte, etag string) {
	c.mu.Lock()
	c.body, c.gzip, c.brotli, c.etag, c.present = body, gzip, nil, etag, true
	c.mu.Unlock()
}

// SetWithBrotli stores artifact with both compressions.
func (c *ArtifactCache) SetWithBrotli(body, gz, br []byte, etag string) {
	c.mu.Lock()
	c.body, c.gzip, c.brotli, c.etag, c.present = body, gz, br, etag, true
	c.mu.Unlock()
}

// Invalidate drops the cached artifact.
func (c *ArtifactCache) Invalidate() {
	c.mu.Lock()
	c.body, c.gzip, c.brotli, c.etag, c.present = nil, nil, nil, "", false
	c.mu.Unlock()
}

// Keep old type aliases for backward compatibility; new code should use ArtifactCache.
type SitemapCache = ArtifactCache
type RobotsCache = ArtifactCache
type FeedCache = ArtifactCache

func NewSitemapCache() *ArtifactCache { return NewArtifactCache() }
func NewRobotsCache() *ArtifactCache  { return NewArtifactCache() }
func NewFeedCache() *ArtifactCache    { return NewArtifactCache() }
