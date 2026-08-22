package runtimehub

import "sync"

// sitemapCache stores the rendered sitemap.xml bytes (plus a precompressed
// copy and ETag) so it is built only when content or site settings change.
type sitemapCache struct {
	mu      sync.RWMutex
	body    []byte
	gzip    []byte
	etag    string
	present bool
}

func NewSitemapCache() *sitemapCache { return &sitemapCache{} }

func (c *sitemapCache) Get() ([]byte, []byte, string, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.body, c.gzip, c.etag, c.present
}

func (c *sitemapCache) Set(body []byte, gzip []byte, etag string) {
	c.mu.Lock()
	c.body, c.gzip, c.etag, c.present = body, gzip, etag, true
	c.mu.Unlock()
}

func (c *sitemapCache) Invalidate() {
	c.mu.Lock()
	c.body, c.gzip, c.etag, c.present = nil, nil, "", false
	c.mu.Unlock()
}

// robotsCache stores the rendered robots.txt bytes.
type robotsCache struct {
	mu      sync.RWMutex
	body    []byte
	gzip    []byte
	etag    string
	present bool
}

func NewRobotsCache() *robotsCache { return &robotsCache{} }

func (c *robotsCache) Get() ([]byte, []byte, string, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.body, c.gzip, c.etag, c.present
}

func (c *robotsCache) Set(body []byte, gzip []byte, etag string) {
	c.mu.Lock()
	c.body, c.gzip, c.etag, c.present = body, gzip, etag, true
	c.mu.Unlock()
}

func (c *robotsCache) Invalidate() {
	c.mu.Lock()
	c.body, c.gzip, c.etag, c.present = nil, nil, "", false
	c.mu.Unlock()
}
