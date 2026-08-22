package runtimehub

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"sync"

	"github.com/kokosx/stratum/internal/blocks"
	"github.com/kokosx/stratum/internal/pagecache"
	"github.com/kokosx/stratum/internal/themes"
)

// PageCache, SitemapCache and RobotsCache are the cached runtime artifacts. They
// are rebuildable from the database and never a source of truth.
type (
	PageCache    = pagecache.Cache
	SitemapCache = sitemapCache
	RobotsCache  = robotsCache
)

// AssetManifest holds the fingerprinted, immutable CSS/JS assets. The hash is
// derived from the real content, so a changed theme or block set produces new
// URLs and any stale reference is never served with an immutable header.
type AssetManifest struct {
	mu           sync.RWMutex
	blocksHash   string
	themeCSSHash string
	themeJSHash  string
	blocks       []byte
	themeCSS     []byte
	themeJS      []byte
}

func NewAssetManifest(blocksReg *blocks.Registry, runtime *themes.Runtime) *AssetManifest {
	m := &AssetManifest{}
	m.Rebuild(blocksReg, runtime)
	return m
}

// Rebuild recomputes the fingerprinted assets from the current block and theme
// snapshots. Call it after any change to blocks or theme customization.
func (m *AssetManifest) Rebuild(blocksReg *blocks.Registry, runtime *themes.Runtime) {
	blocksCSS := []byte(blocksReg.Styles())
	themeCSS := []byte(runtime.Styles())
	themeJS := []byte(runtime.JavaScript())
	m.mu.Lock()
	defer m.mu.Unlock()
	m.blocks, m.themeCSS, m.themeJS = blocksCSS, themeCSS, themeJS
	m.blocksHash = hashHex(blocksCSS)
	m.themeCSSHash = hashHex(themeCSS)
	m.themeJSHash = hashHex(themeJS)
}

// URLs returns the current fingerprinted asset URLs.
func (m *AssetManifest) URLs() (blocksCSS, themeCSS, themeJS string) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return "/stratum/blocks." + m.blocksHash + ".css",
		"/stratum/theme." + m.themeCSSHash + ".css",
		"/stratum/theme." + m.themeJSHash + ".js"
}

// Serve writes a fingerprinted asset if path matches a current URL. It returns
// false when the path is not a known fingerprinted asset.
func (m *AssetManifest) Serve(w http.ResponseWriter, r *http.Request) bool {
	m.mu.RLock()
	blocksURL := "/stratum/blocks." + m.blocksHash + ".css"
	themeCSSURL := "/stratum/theme." + m.themeCSSHash + ".css"
	themeJSURL := "/stratum/theme." + m.themeJSHash + ".js"
	var body []byte
	var ctype string
	switch r.URL.Path {
	case blocksURL:
		body, ctype = m.blocks, "text/css; charset=utf-8"
	case themeCSSURL:
		body, ctype = m.themeCSS, "text/css; charset=utf-8"
	case themeJSURL:
		body, ctype = m.themeJS, "text/javascript; charset=utf-8"
	default:
		m.mu.RUnlock()
		return false
	}
	m.mu.RUnlock()

	w.Header().Set("Content-Type", ctype)
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	w.Header().Set("ETag", hashETag(body))
	if r.Method == http.MethodHead {
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(body)))
		return true
	}
	_, _ = io.WriteString(w, string(body))
	return true
}

// LegacyRedirect returns the fingerprinted URL for a legacy /stratum/*.css|js
// path, or "" if the path is not a known legacy asset. Callers redirect (302) so
// the immutable fingerprinted URL is what browsers cache long-term.
func (m *AssetManifest) LegacyRedirect(path string) string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	switch path {
	case "/stratum/blocks.css":
		return "/stratum/blocks." + m.blocksHash + ".css"
	case "/stratum/theme.css":
		return "/stratum/theme." + m.themeCSSHash + ".css"
	case "/stratum/theme.js":
		return "/stratum/theme." + m.themeJSHash + ".js"
	}
	return ""
}

func hashHex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:16])
}

func hashETag(b []byte) string {
	sum := sha256.Sum256(b)
	return `"` + hex.EncodeToString(sum[:16]) + `"`
}
