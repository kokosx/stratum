package media

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/kokosx/stratum/internal/rendering"
	db "github.com/kokosx/stratum/internal/storage/sqlc"
)

// mediaViewCache caches resolved rendering.MediaView values by media id. Each
// entry depends only on the media row and its variants, which change only on
// upload, metadata update, variant regeneration, or delete — so the cache is
// invalidated by id. It removes the N+1 GetMedia/ListMediaVariants queries that
// block image rendering used to issue.
type mediaViewCache struct {
	mu    sync.RWMutex
	views map[string]rendering.MediaView
}

// faviconViewCache caches the generated site-icon URLs per asset.
type faviconViewCache struct {
	mu    sync.RWMutex
	views map[string]rendering.FaviconView
}

// Service is the media domain entry point. It validates uploads, stores blobs in
// the Storage backend, persists metadata, and resolves assets for rendering.
type Service struct {
	db               *sql.DB
	queries          *db.Queries
	store            Storage
	views            *mediaViewCache
	favicon          *faviconViewCache
	serve            *serveCache
	usageIndexBuilds int64
}

func NewService(queries *db.Queries, store Storage) *Service {
	return &Service{queries: queries, store: store, views: &mediaViewCache{views: make(map[string]rendering.MediaView)}, favicon: &faviconViewCache{views: make(map[string]rendering.FaviconView)}, serve: newServeCache()}
}

// NewServiceWithDB is the full constructor that wires the DB for transactional replace/regenerate.
func NewServiceWithDB(database *sql.DB, queries *db.Queries, store Storage) *Service {
	return &Service{db: database, queries: queries, store: store, views: &mediaViewCache{views: make(map[string]rendering.MediaView)}, favicon: &faviconViewCache{views: make(map[string]rendering.FaviconView)}, serve: newServeCache()}
}

// SetDB wires the DB after construction (for callers that already created via NewService).
func (s *Service) SetDB(database *sql.DB) { s.db = database }

// InvalidateView drops the cached rendering view for one asset. Called after
// metadata updates, variant regeneration, favicon rebuilds, or deletes.
func (s *Service) InvalidateView(id string) {
	s.views.invalidate(id)
	s.favicon.invalidate(id)
	s.serve.deleteByMediaID(id)
}

// InvalidateAllViews drops every cached view. Used on bulk changes.
func (s *Service) InvalidateAllViews() {
	s.views.invalidateAll()
	s.favicon.invalidateAll()
	s.serve.invalidateAll()
}

func (c *mediaViewCache) get(id string) (rendering.MediaView, bool) {
	c.mu.RLock()
	v, ok := c.views[id]
	c.mu.RUnlock()
	return v, ok
}

func (c *mediaViewCache) set(id string, v rendering.MediaView) {
	c.mu.Lock()
	c.views[id] = v
	c.mu.Unlock()
}

func (c *mediaViewCache) invalidate(id string) {
	c.mu.Lock()
	delete(c.views, id)
	c.mu.Unlock()
}

func (c *mediaViewCache) invalidateAll() {
	c.mu.Lock()
	c.views = make(map[string]rendering.MediaView)
	c.mu.Unlock()
}

func (c *faviconViewCache) get(id string) (rendering.FaviconView, bool) {
	c.mu.RLock()
	v, ok := c.views[id]
	c.mu.RUnlock()
	return v, ok
}

func (c *faviconViewCache) set(id string, v rendering.FaviconView) {
	c.mu.Lock()
	c.views[id] = v
	c.mu.Unlock()
}

func (c *faviconViewCache) invalidate(id string) {
	c.mu.Lock()
	delete(c.views, id)
	c.mu.Unlock()
}

func (c *faviconViewCache) invalidateAll() {
	c.mu.Lock()
	c.views = make(map[string]rendering.FaviconView)
	c.mu.Unlock()
}

// Upload ingests an image: verifies its real format, stores the original and
// generated variants, and records metadata. authorID may be empty.
func (s *Service) Upload(ctx context.Context, originalName, authorID string, r io.Reader) (*Asset, error) {
	data, err := readLimited(r, maxImageBytes)
	if err != nil {
		return nil, fmt.Errorf("read upload: %w", err)
	}
	if int64(len(data)) > maxImageBytes {
		return nil, ErrTooLarge
	}

	processed, err := ProcessImage(data)
	if err != nil {
		return nil, err
	}

	token, err := randomToken(18)
	if err != nil {
		return nil, err
	}
	mediaID := "media_" + token
	now := time.Now().Unix()

	origKey := "originals/" + mediaID + extForFormat(processed.Format)
	if err := s.store.Put(ctx, origKey, processed.Original); err != nil {
		return nil, fmt.Errorf("store original: %w", err)
	}

	author := sql.NullString{}
	if authorID != "" {
		author = sql.NullString{String: authorID, Valid: true}
	}
	width := sql.NullInt64{}
	if processed.Width > 0 {
		width = sql.NullInt64{Int64: int64(processed.Width), Valid: true}
	}
	height := sql.NullInt64{}
	if processed.Height > 0 {
		height = sql.NullInt64{Int64: int64(processed.Height), Valid: true}
	}

	if _, err := s.queries.CreateMedia(ctx, db.CreateMediaParams{
		ID:               mediaID,
		OriginalFilename: originalName,
		StorageKey:       origKey,
		MimeType:         mimeForFormat(processed.Format),
		AssetType:        string(AssetTypeImage),
		FileSize:         int64(len(processed.Original)),
		Width:            width,
		Height:           height,
		AltText:          "",
		Title:            "",
		Caption:          "",
		Description:      "",
		AuthorID:         author,
		CreatedAt:        now,
		UpdatedAt:        now,
	}); err != nil {
		_ = s.store.Delete(ctx, origKey)
		return nil, fmt.Errorf("create media row: %w", err)
	}

	for _, v := range processed.Variants {
		kind := v.Kind
		if kind == "" {
			kind = strconv.Itoa(v.Width)
		}
		vid := mediaID + "-v" + kind
		if kind == socialKind {
			vid = mediaID + "-social"
		}
		vkey := "generated/" + mediaID + "-" + kind + extForMime(v.Mime)
		if err := s.store.Put(ctx, vkey, v.Data); err != nil {
			s.rollback(ctx, mediaID, origKey)
			return nil, fmt.Errorf("store variant: %w", err)
		}
		if _, err := s.queries.CreateMediaVariant(ctx, db.CreateMediaVariantParams{
			ID:          vid,
			MediaID:     mediaID,
			Kind:        kind,
			StorageKey:  vkey,
			MimeType:    v.Mime,
			Width:       sql.NullInt64{Int64: int64(v.Width), Valid: true},
			Height:      sql.NullInt64{Int64: int64(v.Height), Valid: true},
			FileSize:    int64(len(v.Data)),
			ContentHash: contentHash(v.Data),
			CreatedAt:   now,
		}); err != nil {
			s.rollback(ctx, mediaID, origKey)
			return nil, fmt.Errorf("create variant row: %w", err)
		}
	}

	return s.Get(ctx, mediaID)
}

// rollback removes every trace of a partially uploaded asset.
func (s *Service) rollback(ctx context.Context, mediaID, origKey string) {
	variants, _ := s.queries.ListMediaVariants(ctx, mediaID)
	for _, v := range variants {
		_ = s.store.Delete(ctx, v.StorageKey)
	}
	_ = s.store.Delete(ctx, origKey)
	_ = s.queries.DeleteMedia(ctx, mediaID)
}

// Get returns a full asset with its variants.
func (s *Service) Get(ctx context.Context, id string) (*Asset, error) {
	m, err := s.queries.GetMedia(ctx, id)
	if err != nil {
		return nil, err
	}
	variants, err := s.queries.ListMediaVariants(ctx, id)
	if err != nil {
		return nil, err
	}
	return assetFromModel(m, variants), nil
}

// List returns recent assets (newest first) for the Media Library and picker.
func (s *Service) List(ctx context.Context, limit, offset int) ([]Asset, error) {
	rows, err := s.queries.ListMedia(ctx, db.ListMediaParams{
		Limit:  int64(limit),
		Offset: int64(offset),
	})
	if err != nil {
		return nil, err
	}
	assets := make([]Asset, 0, len(rows))
	for _, m := range rows {
		assets = append(assets, *assetFromModel(m, nil))
	}
	return assets, nil
}

// UpdateMetadata edits the human-authored fields of an asset.
func (s *Service) UpdateMetadata(ctx context.Context, id, alt, title, caption, description string) error {
	err := s.queries.UpdateMediaMetadata(ctx, db.UpdateMediaMetadataParams{
		AltText:     alt,
		Title:       title,
		Caption:     caption,
		Description: description,
		UpdatedAt:   time.Now().Unix(),
		ID:          id,
	})
	if err != nil {
		return err
	}
	s.InvalidateView(id)
	return nil
}

// CountUsage reports how many places reference the asset (content revisions and
// site settings). Used to warn before deletion.
func (s *Service) CountUsage(ctx context.Context, id string) (int64, error) {
	return s.queries.CountMediaUsage(ctx, sql.NullString{String: id, Valid: true})
}

// Delete removes an asset and all its stored variants. Callers should check
// CountUsage first and confirm with the user when it is non-zero.
func (s *Service) Delete(ctx context.Context, id string) error {
	variants, err := s.queries.ListMediaVariants(ctx, id)
	if err != nil {
		return err
	}
	m, err := s.queries.GetMedia(ctx, id)
	if err != nil {
		return err
	}
	for _, v := range variants {
		_ = s.store.Delete(ctx, v.StorageKey)
	}
	_ = s.store.Delete(ctx, m.StorageKey)
	s.InvalidateView(id)
	return s.queries.DeleteMedia(ctx, id)
}

// ReadVariant returns the bytes and content type for a stored derivative. kind is
// "original", a numeric responsive width, or "favicon-N".
func (s *Service) ReadVariant(ctx context.Context, id, kind string) ([]byte, string, error) {
	if kind == "" || kind == "original" {
		m, err := s.queries.GetMedia(ctx, id)
		if err != nil {
			return nil, "", err
		}
		data, err := s.store.Read(ctx, m.StorageKey)
		return data, m.MimeType, err
	}
	v, err := s.queries.GetMediaVariant(ctx, db.GetMediaVariantParams{MediaID: id, Kind: kind})
	if err != nil {
		return nil, "", err
	}
	data, err := s.store.Read(ctx, v.StorageKey)
	return data, v.MimeType, err
}

// OpenVariant returns a seekable, closable handle to a stored derivative for
// streaming (Range requests, no full read into RAM). The caller must close it.
// Warm serving uses the in-memory serve metadata index (zero DB).
func (s *Service) OpenVariant(ctx context.Context, id, kind string) (*os.File, int64, string, error) {
	if kind == "" {
		kind = "original"
	}
	// strip query string
	if idx := strings.Index(kind, "?"); idx != -1 {
		kind = kind[:idx]
	}

	// Fast path: metadata in RAM
	if meta, ok := s.serve.get(id, kind); ok {
		f, _, err := s.store.Open(ctx, meta.StorageKey)
		if err == nil {
			// Use cached size/mime; size from storage may differ but prefer cached for consistency
			return f, meta.Size, meta.MIME, nil
		}
		// stale entry (e.g. deleted on disk) – evict and fall through to DB
		s.serve.delete(id, kind)
	}

	if kind == "original" {
		m, err := s.queries.GetMedia(ctx, id)
		if err != nil {
			return nil, 0, "", err
		}
		// populate serve cache for future hits
		s.serve.set(id, kind, serveMeta{StorageKey: m.StorageKey, MIME: m.MimeType, Size: m.FileSize, ETag: etagForOriginal(m.StorageKey, m.FileSize)})
		f, size, err := s.store.Open(ctx, m.StorageKey)
		if err != nil {
			return nil, 0, "", err
		}
		return f, size, m.MimeType, nil
	}
	v, err := s.queries.GetMediaVariant(ctx, db.GetMediaVariantParams{MediaID: id, Kind: kind})
	if err != nil {
		return nil, 0, "", err
	}
	s.serve.set(id, kind, serveMeta{StorageKey: v.StorageKey, MIME: v.MimeType, Size: v.FileSize, ETag: etagFromHash(v.ContentHash)})
	f, size, err := s.store.Open(ctx, v.StorageKey)
	if err != nil {
		return nil, 0, "", err
	}
	return f, size, v.MimeType, nil
}

// ServeMeta returns the cached serve metadata for an asset variant if present (zero DB).
// It is used by the public handler to set immutable cache headers and ETag without DB.
func (s *Service) ServeMeta(ctx context.Context, id, kind string) (storageKey, mime, etag string, size int64, ok bool) {
	if kind == "" {
		kind = "original"
	}
	if idx := strings.Index(kind, "?"); idx != -1 {
		kind = kind[:idx]
	}
	if meta, hit := s.serve.get(id, kind); hit {
		return meta.StorageKey, meta.MIME, meta.ETag, meta.Size, true
	}
	return "", "", "", 0, false
}

// GenerateFaviconVariants (re)builds the square favicon sizes from the asset.
// It is failure-safe: old favicon variants remain valid until new set is committed.
func (s *Service) GenerateFaviconVariants(ctx context.Context, id string) error {
	m, err := s.queries.GetMedia(ctx, id)
	if err != nil {
		return err
	}
	data, err := s.store.Read(ctx, m.StorageKey)
	if err != nil {
		return err
	}

	// Generate ALL favicon bytes in memory first
	type pendingFav struct {
		size int
		data []byte
		mime string
	}
	var pending []pendingFav
	for _, size := range faviconSizes {
		b, mime, err := FaviconVariant(data, size)
		if err != nil {
			return err
		}
		pending = append(pending, pendingFav{size: size, data: b, mime: mime})
	}

	// Load existing favicon variants (fail-safe: abort if cannot load)
	existing, err := s.queries.ListMediaVariants(ctx, id)
	if err != nil {
		return fmt.Errorf("list variants: %w", err)
	}
	var oldFavIDs []string
	var oldFavKeys []string
	for _, v := range existing {
		if strings.HasPrefix(v.Kind, "favicon-") {
			oldFavIDs = append(oldFavIDs, v.ID)
			oldFavKeys = append(oldFavKeys, v.StorageKey)
		}
	}

	// Write ALL new variants to fresh storage keys
	token, err := randomToken(6)
	if err != nil {
		return err
	}
	type pendingStore struct {
		id, kind, key, mime string
		width, height       int
		data                []byte
		hash                string
	}
	var stores []pendingStore
	var newKeys []string
	cleanupNew := func() {
		for _, k := range newKeys {
			_ = s.store.Delete(ctx, k)
		}
	}
	for _, p := range pending {
		key := "generated/" + id + "-favicon-" + strconv.Itoa(p.size) + "-" + token + extForMime(p.mime)
		if err := s.store.Put(ctx, key, p.data); err != nil {
			cleanupNew()
			return fmt.Errorf("store favicon %d: %w", p.size, err)
		}
		newKeys = append(newKeys, key)
		stores = append(stores, pendingStore{
			id:   id + "-fav-" + strconv.Itoa(p.size),
			kind: "favicon-" + strconv.Itoa(p.size),
			key:  key, mime: p.mime, width: p.size, height: p.size,
			data: p.data, hash: contentHash(p.data),
		})
	}

	// Transactionally swap metadata
	now := time.Now().Unix()
	if s.db != nil {
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			cleanupNew()
			return fmt.Errorf("begin tx: %w", err)
		}
		defer tx.Rollback()
		for _, vid := range oldFavIDs {
			if _, err := tx.ExecContext(ctx, `DELETE FROM media_variants WHERE id = ?`, vid); err != nil {
				cleanupNew()
				return fmt.Errorf("delete old favicon: %w", err)
			}
		}
		for _, ps := range stores {
			if _, err := tx.ExecContext(ctx, `INSERT INTO media_variants (id, media_id, kind, storage_key, mime_type, width, height, file_size, content_hash, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				ps.id, id, ps.kind, ps.key, ps.mime, sql.NullInt64{Int64: int64(ps.width), Valid: true}, sql.NullInt64{Int64: int64(ps.height), Valid: true}, int64(len(ps.data)), ps.hash, now); err != nil {
				cleanupNew()
				return fmt.Errorf("create favicon %s: %w", ps.kind, err)
			}
		}
		if err := tx.Commit(); err != nil {
			cleanupNew()
			return fmt.Errorf("commit favicon: %w", err)
		}
	} else {
		// Fallback for callers without DB handle (tests): non-transactional but still fresh keys
		for _, vid := range oldFavIDs {
			_ = s.queries.DeleteMediaVariant(ctx, vid)
		}
		for _, ps := range stores {
			if _, err := s.queries.CreateMediaVariant(ctx, db.CreateMediaVariantParams{
				ID: ps.id, MediaID: id, Kind: ps.kind, StorageKey: ps.key, MimeType: ps.mime,
				Width: sql.NullInt64{Int64: int64(ps.width), Valid: true}, Height: sql.NullInt64{Int64: int64(ps.height), Valid: true},
				FileSize: int64(len(ps.data)), ContentHash: ps.hash, CreatedAt: now,
			}); err != nil {
				cleanupNew()
				return err
			}
		}
	}

	s.favicon.invalidate(id)
	s.InvalidateView(id)

	// Delete old blobs best-effort after commit
	for _, k := range oldFavKeys {
		_ = s.store.Delete(ctx, k)
	}
	return nil
}

func buildMediaView(m db.Medium, variants []db.MediaVariant) rendering.MediaView {
	view := rendering.MediaView{
		ID:     m.ID,
		Alt:    m.AltText,
		Width:  intVal(m.Width),
		Height: intVal(m.Height),
	}
	native := make([]respVariant, 0, len(variants))
	webp := make([]respVariant, 0, len(variants))
	defaultWidth := 0
	defaultURL := ""
	for _, v := range variants {
		if w := parseWebPKind(v.Kind); w > 0 {
			webp = append(webp, respVariant{width: w, url: variantURL(m.ID, v.Kind, v.ContentHash)})
			continue
		}
		w := parseIntKind(v.Kind)
		if w <= 0 {
			continue
		}
		url := variantURL(m.ID, v.Kind, v.ContentHash)
		native = append(native, respVariant{width: w, url: url})
		if w > defaultWidth {
			defaultWidth = w
			defaultURL = url
		}
	}
	sort.Slice(native, func(i, j int) bool { return native[i].width < native[j].width })
	sort.Slice(webp, func(i, j int) bool { return webp[i].width < webp[j].width })
	view.SrcSet = joinSrcSet(native)
	view.WebPSrcSet = joinSrcSet(webp)
	if defaultURL != "" {
		view.Src = defaultURL
	} else {
		view.Src = "/media/" + m.ID + "/original"
	}
	return view
}

// MediaView implements rendering.MediaProvider, resolving an asset id into the
// URLs and dimensions a block template needs. Src/SrcSet resolve the
// native-format responsive variants (falling back to the original), and
// WebPSrcSet resolves the WebP derivative set so templates can emit a
// <picture> source only when one exists.
func (s *Service) MediaView(ctx context.Context, id string) (rendering.MediaView, bool) {
	if view, ok := s.views.get(id); ok {
		return view, true
	}
	m, err := s.queries.GetMedia(ctx, id)
	if err != nil {
		return rendering.MediaView{}, false
	}
	variants, err := s.queries.ListMediaVariants(ctx, id)
	if err != nil {
		return rendering.MediaView{}, false
	}
	view := buildMediaView(m, variants)
	s.views.set(id, view)
	return view, true
}

// MediaViews returns MediaViews for the given ids in at most two DB queries.
// It checks the per-ID cache first, batch-fetches missing metadata via
// ListMediaByIDs / ListMediaVariantsByMediaIDs, populates the cache, and
// returns all available views. Warm callers pay zero DB.
func (s *Service) MediaViews(ctx context.Context, ids []string) map[string]rendering.MediaView {
	if len(ids) == 0 {
		return nil
	}
	// Deduplicate preserving input order for cache check
	seen := make(map[string]struct{}, len(ids))
	uniq := make([]string, 0, len(ids))
	for _, id := range ids {
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		uniq = append(uniq, id)
	}
	if len(uniq) == 0 {
		return nil
	}
	result := make(map[string]rendering.MediaView, len(uniq))
	missing := make([]string, 0, len(uniq))
	for _, id := range uniq {
		if v, ok := s.views.get(id); ok {
			result[id] = v
		} else {
			missing = append(missing, id)
		}
	}
	if len(missing) == 0 {
		return result
	}
	mediaRows, err := s.queries.ListMediaByIDs(ctx, missing)
	if err != nil {
		return result
	}
	variantRows, err := s.queries.ListMediaVariantsByMediaIDs(ctx, missing)
	if err != nil {
		return result
	}
	mediaByID := make(map[string]db.Medium, len(mediaRows))
	for _, m := range mediaRows {
		mediaByID[m.ID] = m
	}
	variantsByID := make(map[string][]db.MediaVariant, len(missing))
	for _, v := range variantRows {
		variantsByID[v.MediaID] = append(variantsByID[v.MediaID], v)
	}
	for _, id := range missing {
		m, ok := mediaByID[id]
		if !ok {
			continue
		}
		vars := variantsByID[id]
		view := buildMediaView(m, vars)
		s.views.set(id, view)
		result[id] = view
	}
	return result
}

// respVariant is one srcset candidate: derivative width and its public URL.
type respVariant struct {
	width int
	url   string
}

// variantURL builds the public URL for a derivative. Variants whose bytes can
// be regenerated carry their content hash as ?v= so new bytes never hide
// behind an immutable-cached URL. Originals are write-once and stay unversioned.
func variantURL(mediaID, kind, hash string) string {
	if hash == "" {
		return "/media/" + mediaID + "/" + kind
	}
	return "/media/" + mediaID + "/" + kind + "?v=" + hash
}

func joinSrcSet(items []respVariant) string {
	if len(items) == 0 {
		return ""
	}
	var b strings.Builder
	for i, r := range items {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(r.url)
		b.WriteString(" ")
		b.WriteString(strconv.Itoa(r.width))
		b.WriteString("w")
	}
	return b.String()
}

// FaviconView returns the generated site-icon URLs for the theme <head>.
// URLs are versioned with content hash so regenerated favicons get new immutable URLs.
func (s *Service) FaviconView(ctx context.Context, id string) (rendering.FaviconView, bool) {
	if view, ok := s.favicon.get(id); ok {
		return view, true
	}
	variants, err := s.queries.ListMediaVariants(ctx, id)
	if err != nil {
		return rendering.FaviconView{}, false
	}
	view := rendering.FaviconView{}
	found := false
	for _, v := range variants {
		if !strings.HasPrefix(v.Kind, "favicon-") {
			continue
		}
		found = true
		url := variantURL(id, v.Kind, v.ContentHash)
		switch strings.TrimPrefix(v.Kind, "favicon-") {
		case "16":
			view.Size16 = url
		case "32":
			view.Size32 = url
		case "180":
			view.Size180 = url
		case "192":
			view.Size192 = url
		case "512":
			view.Size512 = url
		}
	}
	if !found {
		return rendering.FaviconView{}, false
	}
	s.favicon.set(id, view)
	return view, true
}

// SocialImage is the resolved OG/Twitter image view. URL is still relative
// (/media/<id>/social) so callers can absolutize it with the site origin.
// Width/Height/Type are taken from the actual variant when present, so the
// theme can emit og:image:width/height/type and og:image:alt without guessing.
type SocialImage struct {
	URL    string
	Width  int
	Height int
	Type   string
	Alt    string
}

// SocialView resolves the dedicated social preview derivative for an asset.
// It prefers the 1200x630 "social" variant and falls back to the largest
// responsive variant or the original when that variant is missing (older rows,
// GIFs, tiny sources). The returned URL is always relative.
func (s *Service) SocialView(ctx context.Context, id string) (SocialImage, bool) {
	if id == "" {
		return SocialImage{}, false
	}
	m, err := s.queries.GetMedia(ctx, id)
	if err != nil {
		return SocialImage{}, false
	}
	variants, err := s.queries.ListMediaVariants(ctx, id)
	if err != nil {
		return SocialImage{}, false
	}
	// Prefer the dedicated social preview variant.
	for _, v := range variants {
		if v.Kind == socialKind {
			w := intVal(v.Width)
			h := intVal(v.Height)
			if w == 0 {
				w = socialWidth
			}
			if h == 0 {
				h = socialHeight
			}
			return SocialImage{URL: variantURL(id, socialKind, v.ContentHash), Width: w, Height: h, Type: v.MimeType, Alt: m.AltText}, true
		}
	}
	// Fallback: largest native responsive variant (never a ".webp" slug —
	// crawlers fetch og:image without content negotiation), otherwise original.
	bestW := 0
	var best *db.MediaVariant
	for i := range variants {
		w := parseIntKind(variants[i].Kind)
		if w > bestW {
			bestW = w
			best = &variants[i]
		}
	}
	if best != nil {
		return SocialImage{URL: variantURL(id, best.Kind, best.ContentHash), Width: intVal(best.Width), Height: intVal(best.Height), Type: best.MimeType, Alt: m.AltText}, true
	}
	// Tiny image or GIF with no variants: fall back to original.
	w := intVal(m.Width)
	h := intVal(m.Height)
	t := m.MimeType
	if t == "" {
		t = "image/jpeg"
	}
	return SocialImage{URL: "/media/" + id + "/original", Width: w, Height: h, Type: t, Alt: m.AltText}, true
}

// GenerateSocialVariant (re)builds the 1200x630 social preview from the stored
// original. It uses a fresh key and transactional swap so the existing social
// variant remains readable until the new one is committed.
func (s *Service) GenerateSocialVariant(ctx context.Context, id string, focal FocalPoint) error {
	m, err := s.queries.GetMedia(ctx, id)
	if err != nil {
		return err
	}
	data, err := s.store.Read(ctx, m.StorageKey)
	if err != nil {
		return err
	}
	bytes, mime, err := GenerateSocialVariant(data, focal)
	if err != nil {
		return err
	}
	existing, err := s.queries.GetMediaVariant(ctx, db.GetMediaVariantParams{MediaID: id, Kind: socialKind})
	hasExisting := err == nil
	var oldKey, oldID string
	if hasExisting {
		oldKey = existing.StorageKey
		oldID = existing.ID
	} else if err != sql.ErrNoRows {
		return fmt.Errorf("get social variant: %w", err)
	}
	token, err := randomToken(6)
	if err != nil {
		return err
	}
	key := "generated/" + id + "-" + socialKind + "-" + token + extForMime(mime)
	if err := s.store.Put(ctx, key, bytes); err != nil {
		return err
	}
	cleanupNew := func() { _ = s.store.Delete(ctx, key) }
	now := time.Now().Unix()
	if s.db != nil {
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			cleanupNew()
			return fmt.Errorf("begin tx: %w", err)
		}
		defer tx.Rollback()
		if hasExisting {
			if _, err := tx.ExecContext(ctx, `DELETE FROM media_variants WHERE id = ?`, oldID); err != nil {
				cleanupNew()
				return err
			}
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO media_variants (id, media_id, kind, storage_key, mime_type, width, height, file_size, content_hash, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			id+"-social", id, socialKind, key, mime, sql.NullInt64{Int64: socialWidth, Valid: true}, sql.NullInt64{Int64: socialHeight, Valid: true}, int64(len(bytes)), contentHash(bytes), now); err != nil {
			cleanupNew()
			return err
		}
		if err := tx.Commit(); err != nil {
			cleanupNew()
			return err
		}
	} else {
		// No DB handle – fallback to direct (old behavior) but clean up on failure
		if hasExisting {
			_ = s.queries.DeleteMediaVariant(ctx, oldID)
		}
		if _, err := s.queries.CreateMediaVariant(ctx, db.CreateMediaVariantParams{
			ID: id + "-social", MediaID: id, Kind: socialKind, StorageKey: key, MimeType: mime,
			Width: sql.NullInt64{Int64: socialWidth, Valid: true}, Height: sql.NullInt64{Int64: socialHeight, Valid: true},
			FileSize: int64(len(bytes)), ContentHash: contentHash(bytes), CreatedAt: now,
		}); err != nil {
			cleanupNew()
			return err
		}
	}
	s.InvalidateView(id)
	if hasExisting {
		_ = s.store.Delete(ctx, oldKey)
	}
	return nil
}

// --- helpers ---

// contentHash returns a short hex digest of the derivative bytes. It is stored
// on the variant row and appended to resolved URLs (?v=...) so regenerated
// derivatives get fresh immutable-cached URLs instead of serving stale bytes.
func contentHash(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])[:12]
}

func randomToken(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func assetFromModel(m db.Medium, variants []db.MediaVariant) *Asset {
	a := &Asset{
		ID:               m.ID,
		OriginalFilename: m.OriginalFilename,
		StorageKey:       m.StorageKey,
		MimeType:         m.MimeType,
		AssetType:        m.AssetType,
		FileSize:         m.FileSize,
		Width:            intVal(m.Width),
		Height:           intVal(m.Height),
		AltText:          m.AltText,
		Title:            m.Title,
		Caption:          m.Caption,
		Description:      m.Description,
		AuthorID:         nullStr(m.AuthorID),
		CreatedAt:        time.Unix(m.CreatedAt, 0),
		UpdatedAt:        time.Unix(m.UpdatedAt, 0),
	}
	for _, v := range variants {
		a.Variants = append(a.Variants, Variant{
			ID:          v.ID,
			MediaID:     v.MediaID,
			Kind:        v.Kind,
			StorageKey:  v.StorageKey,
			MimeType:    v.MimeType,
			Width:       intVal(v.Width),
			Height:      intVal(v.Height),
			FileSize:    v.FileSize,
			ContentHash: v.ContentHash,
		})
	}
	return a
}

func intVal(v sql.NullInt64) int {
	if v.Valid {
		return int(v.Int64)
	}
	return 0
}

// ThumbURL returns a URL suitable for a small preview/thumbnail. It prefers the
// smallest generated responsive variant and falls back to the original when no
// responsive variants exist (GIFs, or images smaller than the smallest
// responsive width). It never assumes a fixed variant width exists.
func (a *Asset) ThumbURL() string {
	bestKind := ""
	bestWidth := 0
	for _, v := range a.Variants {
		w, err := strconv.Atoi(v.Kind)
		if err != nil {
			continue
		}
		if bestKind == "" || w < bestWidth {
			bestKind = v.Kind
			bestWidth = w
		}
	}
	if bestKind != "" {
		return "/media/" + a.ID + "/" + bestKind
	}
	return "/media/" + a.ID + "/original"
}

func nullStr(v sql.NullString) string {
	if v.Valid {
		return v.String
	}
	return ""
}

func parseIntKind(kind string) int {
	if kind == "" {
		return 0
	}
	n, err := strconv.Atoi(kind)
	if err != nil {
		return 0
	}
	return n
}

// parseWebPKind returns the pixel width of a WebP derivative kind such as
// "480.webp", or 0 when kind is not a WebP responsive variant. Keeping these
// separate from parseIntKind lets resolvers pick native-format candidates
// (e.g. the og:image fallback) without ever selecting a ".webp" slug.
func parseWebPKind(kind string) int {
	if !strings.HasSuffix(kind, webpKindExt) {
		return 0
	}
	n, err := strconv.Atoi(strings.TrimSuffix(kind, webpKindExt))
	if err != nil || n <= 0 {
		return 0
	}
	return n
}

func mimeForFormat(format string) string {
	switch format {
	case "jpeg":
		return "image/jpeg"
	case "png":
		return "image/png"
	case "gif":
		return "image/gif"
	case "webp":
		return "image/webp"
	default:
		return "application/octet-stream"
	}
}

func extForFormat(format string) string {
	switch format {
	case "jpeg":
		return ".jpg"
	case "png":
		return ".png"
	case "gif":
		return ".gif"
	case "webp":
		return ".webp"
	default:
		return ""
	}
}

func extForMime(mime string) string {
	switch mime {
	case "image/jpeg":
		return ".jpg"
	case "image/png":
		return ".png"
	case "image/gif":
		return ".gif"
	case "image/webp":
		return ".webp"
	default:
		return ""
	}
}
