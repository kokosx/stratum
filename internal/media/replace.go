package media

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"strings"
	"time"
)

// Replace replaces the bytes of an existing media asset with a new image file.
// It preserves the Media ID and editorial metadata (alt, title, caption, description) while updating
// storage key, mime, size, dimensions, variants, and hashes. It uses fresh storage keys and a safe
// two-phase commit: new blobs are written first, then DB transaction, then old blobs cleaned up.
// If the media is currently the Site Icon, favicon derivatives are regenerated atomically
// as part of the same commit so a failure cannot destroy the current working favicon set.
func (s *Service) Replace(ctx context.Context, id, newFilename string, r io.Reader) (*Asset, error) {
	if id == "" {
		return nil, fmt.Errorf("media id is required")
	}
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
	// Load existing
	existing, err := s.queries.GetMedia(ctx, id)
	if err != nil {
		return nil, err
	}
	oldVariants, err := s.queries.ListMediaVariants(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("list variants: %w", err)
	}
	oldStorageKey := existing.StorageKey

	// Determine if this media is the Site Icon before touching storage
	isSiteIcon := false
	if s.db != nil {
		var iconID sql.NullString
		if err := s.db.QueryRowContext(ctx, `SELECT site_icon_media_id FROM site_settings WHERE id=1`).Scan(&iconID); err == nil && iconID.Valid && iconID.String == id {
			isSiteIcon = true
		} else if err != nil && err != sql.ErrNoRows {
			return nil, fmt.Errorf("check site icon: %w", err)
		}
	}

	// Prepare favicon bytes upfront if site icon (before any storage writes)
	type favPending struct {
		size int
		data []byte
		mime string
	}
	var favs []favPending
	if isSiteIcon {
		for _, size := range faviconSizes {
			b, mime, err := FaviconVariant(processed.Original, size)
			if err != nil {
				return nil, fmt.Errorf("favicon %d: %w", size, err)
			}
			favs = append(favs, favPending{size: size, data: b, mime: mime})
		}
	}

	// Generate fresh keys
	token, err := randomToken(6)
	if err != nil {
		return nil, err
	}
	newOrigKey := "originals/" + id + "-" + token + extForFormat(processed.Format)

	var newKeys []string
	cleanupNew := func() {
		for _, k := range newKeys {
			_ = s.store.Delete(ctx, k)
		}
	}

	if err := s.store.Put(ctx, newOrigKey, processed.Original); err != nil {
		return nil, fmt.Errorf("store original: %w", err)
	}
	newKeys = append(newKeys, newOrigKey)

	type pendingV struct {
		ID, Kind, StorageKey, Mime string
		Width, Height              int
		Data                       []byte
		Hash                       string
	}
	var pending []pendingV
	for _, v := range processed.Variants {
		kind := v.Kind
		vid := id + "-v" + kind
		if kind == socialKind {
			vid = id + "-social"
		}
		vkey := "generated/" + id + "-" + kind + "-" + token + extForMime(v.Mime)
		if err := s.store.Put(ctx, vkey, v.Data); err != nil {
			cleanupNew()
			return nil, fmt.Errorf("store variant %s: %w", kind, err)
		}
		newKeys = append(newKeys, vkey)
		pending = append(pending, pendingV{ID: vid, Kind: kind, StorageKey: vkey, Mime: v.Mime, Width: v.Width, Height: v.Height, Data: v.Data, Hash: contentHash(v.Data)})
	}

	// Prepare favicon pending stores
	var pendingFavs []pendingV
	for _, f := range favs {
		kind := "favicon-" + fmt.Sprintf("%d", f.size)
		vid := id + "-fav-" + fmt.Sprintf("%d", f.size)
		vkey := "generated/" + id + "-favicon-" + fmt.Sprintf("%d", f.size) + "-" + token + extForMime(f.mime)
		if err := s.store.Put(ctx, vkey, f.data); err != nil {
			cleanupNew()
			return nil, fmt.Errorf("store favicon %d: %w", f.size, err)
		}
		newKeys = append(newKeys, vkey)
		pendingFavs = append(pendingFavs, pendingV{ID: vid, Kind: kind, StorageKey: vkey, Mime: f.mime, Width: f.size, Height: f.size, Data: f.data, Hash: contentHash(f.data)})
	}

	// Transactionally update DB
	now := time.Now().Unix()
	if s.db != nil {
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			cleanupNew()
			return nil, fmt.Errorf("begin tx: %w", err)
		}
		defer tx.Rollback()
		width := sql.NullInt64{}
		if processed.Width > 0 {
			width = sql.NullInt64{Int64: int64(processed.Width), Valid: true}
		}
		height := sql.NullInt64{}
		if processed.Height > 0 {
			height = sql.NullInt64{Int64: int64(processed.Height), Valid: true}
		}
		_, err = tx.ExecContext(ctx, `UPDATE media SET original_filename = ?, storage_key = ?, mime_type = ?, file_size = ?, width = ?, height = ?, updated_at = ? WHERE id = ?`,
			newFilename, newOrigKey, mimeForFormat(processed.Format), int64(len(processed.Original)), width, height, now, id)
		if err != nil {
			cleanupNew()
			return nil, fmt.Errorf("update media: %w", err)
		}
		// Delete old variants: keep favicon unless we're replacing it as site icon
		for _, v := range oldVariants {
			if strings.HasPrefix(v.Kind, "favicon-") && !isSiteIcon {
				continue
			}
			if _, err := tx.ExecContext(ctx, `DELETE FROM media_variants WHERE id = ?`, v.ID); err != nil {
				cleanupNew()
				return nil, err
			}
		}
		// Insert new responsive/social variants
		for _, p := range pending {
			_, err := tx.ExecContext(ctx, `INSERT INTO media_variants (id, media_id, kind, storage_key, mime_type, width, height, file_size, content_hash, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				p.ID, id, p.Kind, p.StorageKey, p.Mime, sql.NullInt64{Int64: int64(p.Width), Valid: p.Width > 0}, sql.NullInt64{Int64: int64(p.Height), Valid: p.Height > 0}, int64(len(p.Data)), p.Hash, now)
			if err != nil {
				cleanupNew()
				return nil, fmt.Errorf("create variant: %w", err)
			}
		}
		// Insert favicon variants if site icon
		for _, p := range pendingFavs {
			_, err := tx.ExecContext(ctx, `INSERT INTO media_variants (id, media_id, kind, storage_key, mime_type, width, height, file_size, content_hash, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				p.ID, id, p.Kind, p.StorageKey, p.Mime, sql.NullInt64{Int64: int64(p.Width), Valid: true}, sql.NullInt64{Int64: int64(p.Height), Valid: true}, int64(len(p.Data)), p.Hash, now)
			if err != nil {
				cleanupNew()
				return nil, fmt.Errorf("create favicon %s: %w", p.Kind, err)
			}
		}
		if err := tx.Commit(); err != nil {
			cleanupNew()
			return nil, fmt.Errorf("commit: %w", err)
		}
	} else {
		cleanupNew()
		return nil, fmt.Errorf("replace requires DB handle")
	}

	s.InvalidateView(id)
	s.favicon.invalidate(id)

	// Delete old blobs best-effort after commit
	_ = s.store.Delete(ctx, oldStorageKey)
	for _, v := range oldVariants {
		if strings.HasPrefix(v.Kind, "favicon-") && !isSiteIcon {
			continue
		}
		_ = s.store.Delete(ctx, v.StorageKey)
	}

	return s.Get(ctx, id)
}

// RegenerateVariants rebuilds responsive and optimized versions from the original image.
// It uses fresh keys and a transaction so failure leaves old variants intact.
func (s *Service) RegenerateVariants(ctx context.Context, id string) error {
	if id == "" {
		return fmt.Errorf("media id is required")
	}
	m, err := s.queries.GetMedia(ctx, id)
	if err != nil {
		return err
	}
	data, err := s.store.Read(ctx, m.StorageKey)
	if err != nil {
		return fmt.Errorf("read original: %w", err)
	}
	processed, err := ProcessImage(data)
	if err != nil {
		return err
	}
	oldVariants, err := s.queries.ListMediaVariants(ctx, id)
	if err != nil {
		return fmt.Errorf("list variants: %w", err)
	}
	// Keep favicon variants untouched; only regenerate responsive/social
	var oldToDelete []string
	var oldKeys []string
	for _, v := range oldVariants {
		if strings.HasPrefix(v.Kind, "favicon-") {
			continue
		}
		oldToDelete = append(oldToDelete, v.ID)
		oldKeys = append(oldKeys, v.StorageKey)
	}

	token, err := randomToken(6)
	if err != nil {
		return err
	}
	type pendingV struct {
		ID, Kind, StorageKey, Mime string
		Width, Height              int
		Data                       []byte
		Hash                       string
	}
	var pending []pendingV
	var newKeys []string
	cleanupNew := func() {
		for _, k := range newKeys {
			_ = s.store.Delete(ctx, k)
		}
	}
	for _, v := range processed.Variants {
		kind := v.Kind
		vid := id + "-v" + kind
		if kind == socialKind {
			vid = id + "-social"
		}
		vkey := "generated/" + id + "-" + kind + "-" + token + extForMime(v.Mime)
		if err := s.store.Put(ctx, vkey, v.Data); err != nil {
			cleanupNew()
			return fmt.Errorf("store variant %s: %w", kind, err)
		}
		newKeys = append(newKeys, vkey)
		pending = append(pending, pendingV{ID: vid, Kind: kind, StorageKey: vkey, Mime: v.Mime, Width: v.Width, Height: v.Height, Data: v.Data, Hash: contentHash(v.Data)})
	}

	now := time.Now().Unix()
	if s.db != nil {
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			cleanupNew()
			return err
		}
		defer tx.Rollback()
		for _, vid := range oldToDelete {
			if _, err := tx.ExecContext(ctx, `DELETE FROM media_variants WHERE id = ?`, vid); err != nil {
				cleanupNew()
				return err
			}
		}
		for _, p := range pending {
			_, err := tx.ExecContext(ctx, `INSERT INTO media_variants (id, media_id, kind, storage_key, mime_type, width, height, file_size, content_hash, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				p.ID, id, p.Kind, p.StorageKey, p.Mime, sql.NullInt64{Int64: int64(p.Width), Valid: p.Width > 0}, sql.NullInt64{Int64: int64(p.Height), Valid: p.Height > 0}, int64(len(p.Data)), p.Hash, now)
			if err != nil {
				cleanupNew()
				return err
			}
		}
		if err := tx.Commit(); err != nil {
			cleanupNew()
			return err
		}
	} else {
		cleanupNew()
		return fmt.Errorf("regenerate requires DB handle")
	}

	s.InvalidateView(id)
	for _, k := range oldKeys {
		_ = s.store.Delete(ctx, k)
	}
	return nil
}
