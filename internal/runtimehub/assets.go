package runtimehub

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"

	"github.com/kokosx/stratum/internal/blocks"
	"github.com/kokosx/stratum/internal/cssmin"
	"github.com/kokosx/stratum/internal/pagecache"
	"github.com/kokosx/stratum/internal/rendering"
	"github.com/kokosx/stratum/internal/themes"
)

// PageCache, SitemapCache and RobotsCache are the cached runtime artifacts. They
// are rebuildable from the database and never a source of truth.
// Sitemap/Robots/Feed share the single ArtifactCache implementation (no duplication).
type (
	PageCache = pagecache.Cache
)

// AssetManifest holds fingerprinted, immutable CSS/JS assets. Hashes are always
// derived from final (minified) bytes so a changed transform never reuses a URL
// with different content.
//
// Theme CSS/JS are global. Block CSS is served per-page from UsedBlocks and
// deduplicated by content hash: two pages with the same used-block styles share
// one /stratum/blocks.<hash>.css URL.
type AssetManifest struct {
	mu           sync.RWMutex
	themeCSSHash string
	themeJSHash  string
	themeCSS     []byte
	themeCSSGzip []byte
	themeJS      []byte
	themeJSGzip  []byte

	// pageBlocks maps content-hash → minified CSS (+ gzip) for per-page bundles.
	pageBlocks map[string]assetBody

	// blocksReg is retained so per-page CSS can be resolved from UsedBlocks.
	blocksReg *blocks.Registry
}

type assetBody struct {
	raw  []byte
	gzip []byte
}

func NewAssetManifest(blocksReg *blocks.Registry, runtime *themes.Runtime) *AssetManifest {
	m := &AssetManifest{
		pageBlocks: make(map[string]assetBody),
		blocksReg:  blocksReg,
	}
	m.Rebuild(blocksReg, runtime)
	return m
}

// Rebuild recomputes the fingerprinted theme assets from the current theme
// snapshot and clears the per-page block CSS cache (block styles may have
// changed). Call after any change to blocks or theme customization.
func (m *AssetManifest) Rebuild(blocksReg *blocks.Registry, runtime *themes.Runtime) {
	themeCSS := cssmin.CSS([]byte(runtime.Styles()))
	themeJS := []byte(runtime.JavaScript())
	themeCSSGzip, _ := gzipBytes(themeCSS)
	themeJSGzip, _ := gzipBytes(themeJS)

	m.mu.Lock()
	defer m.mu.Unlock()
	m.blocksReg = blocksReg
	m.themeCSS, m.themeCSSGzip = themeCSS, themeCSSGzip
	m.themeJS, m.themeJSGzip = themeJS, themeJSGzip
	m.themeCSSHash = hashHex(themeCSS)
	m.themeJSHash = hashHex(themeJS)
	// Drop per-page bundles so the next request rebuilds from the new StylesFor.
	m.pageBlocks = make(map[string]assetBody)
}

// URLs returns the current fingerprinted theme asset URLs. BlocksCSS is empty;
// callers that still expect a global blocks URL should use BlocksCSSFor.
func (m *AssetManifest) URLs() (blocksCSS, themeCSS, themeJS string) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return "",
		"/stratum/theme." + m.themeCSSHash + ".css",
		"/stratum/theme." + m.themeJSHash + ".js"
}

// BlocksCSSFor returns the fingerprinted URL for the CSS of the given used
// blocks. Identical UsedBlocks sets share one hash/URL. Empty styles yield "".
func (m *AssetManifest) BlocksCSSFor(keys []rendering.BlockKey) string {
	if m.blocksReg == nil || len(keys) == 0 {
		return ""
	}
	src := m.blocksReg.StylesFor(keys)
	if strings.TrimSpace(src) == "" {
		return ""
	}
	minified := cssmin.CSS([]byte(src))
	hash := hashHex(minified)
	gz, _ := gzipBytes(minified)

	m.mu.Lock()
	if _, ok := m.pageBlocks[hash]; !ok {
		m.pageBlocks[hash] = assetBody{raw: minified, gzip: gz}
	}
	m.mu.Unlock()
	return "/stratum/blocks." + hash + ".css"
}

// Serve writes a fingerprinted asset if path matches a known URL. It returns
// false when the path is not a known fingerprinted asset.
func (m *AssetManifest) Serve(w http.ResponseWriter, r *http.Request) bool {
	path := r.URL.Path
	if !strings.HasPrefix(path, "/stratum/") {
		return false
	}

	m.mu.RLock()
	var body, gz []byte
	var ctype string
	switch {
	case path == "/stratum/theme."+m.themeCSSHash+".css":
		body, gz, ctype = m.themeCSS, m.themeCSSGzip, "text/css; charset=utf-8"
	case path == "/stratum/theme."+m.themeJSHash+".js":
		body, gz, ctype = m.themeJS, m.themeJSGzip, "text/javascript; charset=utf-8"
	case strings.HasPrefix(path, "/stratum/blocks.") && strings.HasSuffix(path, ".css"):
		hash := strings.TrimSuffix(strings.TrimPrefix(path, "/stratum/blocks."), ".css")
		if ab, ok := m.pageBlocks[hash]; ok {
			body, gz, ctype = ab.raw, ab.gzip, "text/css; charset=utf-8"
		}
	}
	m.mu.RUnlock()
	if body == nil {
		return false
	}

	w.Header().Set("Content-Type", ctype)
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	w.Header().Set("ETag", hashETag(body))
	w.Header().Set("Vary", "Accept-Encoding")

	acceptGzip := strings.Contains(r.Header.Get("Accept-Encoding"), "gzip")
	if acceptGzip && len(gz) > 0 {
		w.Header().Set("Content-Encoding", "gzip")
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(gz)))
		if r.Method != http.MethodHead {
			_, _ = w.Write(gz)
		}
		return true
	}
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(body)))
	if r.Method == http.MethodHead {
		return true
	}
	_, _ = io.Copy(w, bytes.NewReader(body))
	return true
}

// LegacyRedirect returns the fingerprinted URL for a legacy /stratum/*.css|js
// path, or "" if the path is not a known legacy asset. Global blocks.css has no
// single replacement (per-page); theme paths still redirect.
func (m *AssetManifest) LegacyRedirect(path string) string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	switch path {
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

func gzipBytes(b []byte) ([]byte, error) {
	if len(b) == 0 {
		return nil, nil
	}
	var buf bytes.Buffer
	gw, err := gzip.NewWriterLevel(&buf, gzip.BestSpeed)
	if err != nil {
		return nil, err
	}
	if _, err := gw.Write(b); err != nil {
		return nil, err
	}
	if err := gw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
