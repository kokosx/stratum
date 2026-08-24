package runtimehub

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"

	"github.com/kokosx/stratum/internal/blocks"
	"github.com/kokosx/stratum/internal/compress"
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
	mu             sync.RWMutex
	themeCSSHash   string
	themeJSHash    string
	themeCSS       []byte
	themeCSSGzip   []byte
	themeCSSBrotli []byte
	themeJS        []byte
	themeJSGzip    []byte
	themeJSBrotli  []byte

	// pageBlocks maps content-hash → minified CSS (+ gzip + brotli) for per-page bundles.
	pageBlocks map[string]assetBody

	// blocksReg is retained so per-page CSS can be resolved from UsedBlocks.
	blocksReg *blocks.Registry

	// blocksMemo caches UsedBlocks signature -> hash to avoid repeated StylesFor/minify.
	blocksMemo       map[string]string
	blocksGeneration uint64
}

type assetBody struct {
	raw    []byte
	gzip   []byte
	brotli []byte
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
	themeCSSGzip, _ := compress.Gzip(themeCSS)
	themeCSSBrotli, _ := compress.Brotli(themeCSS)
	themeJSGzip, _ := compress.Gzip(themeJS)
	themeJSBrotli, _ := compress.Brotli(themeJS)

	m.mu.Lock()
	defer m.mu.Unlock()
	m.blocksReg = blocksReg
	m.themeCSS, m.themeCSSGzip, m.themeCSSBrotli = themeCSS, themeCSSGzip, themeCSSBrotli
	m.themeJS, m.themeJSGzip, m.themeJSBrotli = themeJS, themeJSGzip, themeJSBrotli
	m.themeCSSHash = hashHex(themeCSS)
	m.themeJSHash = hashHex(themeJS)
	// Drop per-page bundles so the next request rebuilds from the new StylesFor.
	m.pageBlocks = make(map[string]assetBody)
	m.blocksMemo = make(map[string]string)
	if blocksReg != nil {
		m.blocksGeneration = blocksReg.Generation()
	}
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
// A small signature memo avoids repeated StylesFor/minify/gzip for identical sets.
func (m *AssetManifest) BlocksCSSFor(keys []rendering.BlockKey) string {
	if m.blocksReg == nil || len(keys) == 0 {
		return ""
	}
	// Fast memo check: signature includes generation + sorted keys.
	gen := m.blocksReg.Generation()
	// keys are already sorted by Prepare, but ensure stable signature.
	sig := blocksSignature(gen, keys)
	m.mu.RLock()
	if cachedHash, ok := m.blocksMemo[sig]; ok {
		// Check that the hash still has a body (should, but guard)
		if _, ok2 := m.pageBlocks[cachedHash]; ok2 {
			m.mu.RUnlock()
			return "/stratum/blocks." + cachedHash + ".css"
		}
	}
	m.mu.RUnlock()

	src := m.blocksReg.StylesFor(keys)
	if strings.TrimSpace(src) == "" {
		return ""
	}
	minified := cssmin.CSS([]byte(src))
	hash := hashHex(minified)
	gz, _ := compress.Gzip(minified)
	br, _ := compress.Brotli(minified)

	m.mu.Lock()
	if _, ok := m.pageBlocks[hash]; !ok {
		m.pageBlocks[hash] = assetBody{raw: minified, gzip: gz, brotli: br}
	}
	if m.blocksMemo == nil {
		m.blocksMemo = make(map[string]string)
	}
	m.blocksMemo[sig] = hash
	m.mu.Unlock()
	return "/stratum/blocks." + hash + ".css"
}

func blocksSignature(gen uint64, keys []rendering.BlockKey) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("%d:", gen))
	for i, k := range keys {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(k.Name)
		b.WriteByte('@')
		b.WriteString(fmt.Sprintf("%d", k.Version))
	}
	return b.String()
}

// Serve writes a fingerprinted asset if path matches a known URL. It returns
// false when the path is not a known fingerprinted asset.
// It supports precompressed Brotli and Gzip with correct negotiation.
func (m *AssetManifest) Serve(w http.ResponseWriter, r *http.Request) bool {
	path := r.URL.Path
	if !strings.HasPrefix(path, "/stratum/") {
		return false
	}

	m.mu.RLock()
	var body, gz, br []byte
	var ctype string
	switch {
	case path == "/stratum/theme."+m.themeCSSHash+".css":
		body, gz, br, ctype = m.themeCSS, m.themeCSSGzip, m.themeCSSBrotli, "text/css; charset=utf-8"
	case path == "/stratum/theme."+m.themeJSHash+".js":
		body, gz, br, ctype = m.themeJS, m.themeJSGzip, m.themeJSBrotli, "text/javascript; charset=utf-8"
	case strings.HasPrefix(path, "/stratum/blocks.") && strings.HasSuffix(path, ".css"):
		hash := strings.TrimSuffix(strings.TrimPrefix(path, "/stratum/blocks."), ".css")
		if ab, ok := m.pageBlocks[hash]; ok {
			body, gz, br, ctype = ab.raw, ab.gzip, ab.brotli, "text/css; charset=utf-8"
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

	enc := compress.NegotiateEncoding(r.Header.Get("Accept-Encoding"))
	switch enc {
	case "br":
		if len(br) > 0 {
			w.Header().Set("Content-Encoding", "br")
			w.Header().Set("Content-Length", fmt.Sprintf("%d", len(br)))
			if r.Method != http.MethodHead {
				_, _ = w.Write(br)
			}
			return true
		}
	case "gzip":
		if len(gz) > 0 {
			w.Header().Set("Content-Encoding", "gzip")
			w.Header().Set("Content-Length", fmt.Sprintf("%d", len(gz)))
			if r.Method != http.MethodHead {
				_, _ = w.Write(gz)
			}
			return true
		}
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
	return compress.Gzip(b)
}

// brotliBytes kept for legacy callers (none)
func brotliBytes(b []byte) ([]byte, error) {
	return compress.Brotli(b)
}

var _ = bytes.MinRead
