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
	oldVariants, _ := s.queries.ListMediaVariants(ctx, id)
	oldStorageKey := existing.StorageKey
	oldVariantKeys := make(map[string]string, len(oldVariants))
	for _, v := range oldVariants {
		oldVariantKeys[v.ID] = v.StorageKey
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
		// Update media row – preserve alt/title/caption/description/author/created_at, update filename/storage/mime/size/dimensions/updated_at
		_, err = tx.ExecContext(ctx, `UPDATE media SET original_filename = ?, storage_key = ?, mime_type = ?, file_size = ?, width = ?, height = ?, updated_at = ? WHERE id = ?`,
			newFilename, newOrigKey, mimeForFormat(processed.Format), int64(len(processed.Original)), width, height, now, id)
		if err != nil {
			cleanupNew()
			return nil, fmt.Errorf("update media: %w", err)
		}
		// Delete old non-favicon variants
		for _, v := range oldVariants {
			if strings.HasPrefix(v.Kind, "favicon-") {
				continue
			}
			if _, err := tx.ExecContext(ctx, `DELETE FROM media_variants WHERE id = ?`, v.ID); err != nil {
				cleanupNew()
				return nil, err
			}
		}
		// Insert new variants
		for _, p := range pending {
			_, err := tx.ExecContext(ctx, `INSERT INTO media_variants (id, media_id, kind, storage_key, mime_type, width, height, file_size, content_hash, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				p.ID, id, p.Kind, p.StorageKey, p.Mime, sql.NullInt64{Int64: int64(p.Width), Valid: p.Width > 0}, sql.NullInt64{Int64: int64(p.Height), Valid: p.Height > 0}, int64(len(p.Data)), p.Hash, now)
			if err != nil {
				cleanupNew()
				return nil, fmt.Errorf("create variant: %w", err)
			}
		}
		if err := tx.Commit(); err != nil {
			cleanupNew()
			return nil, fmt.Errorf("commit: %w", err)
		}
	} else {
		// Fallback without transaction – not supported; tests use DB handle
		cleanupNew()
		return nil, fmt.Errorf("replace requires DB handle")
	}

	// Invalidate caches
	s.InvalidateView(id)

	// If this media is currently used as Site Icon, regenerate favicon variants automatically
	if s.db != nil {
		var iconID sql.NullString
		_ = s.db.QueryRowContext(ctx, `SELECT site_icon_media_id FROM site_settings WHERE id=1`).Scan(&iconID)
		if iconID.Valid && iconID.String == id {
			// GenerateFaviconVariants reads from the new storage key
			_ = s.GenerateFaviconVariants(ctx, id)
		}
	}

	// Delete old blobs best-effort after commit
	_ = s.store.Delete(ctx, oldStorageKey)
	for _, v := range oldVariants {
		if strings.HasPrefix(v.Kind, "favicon-") {
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
	oldVariants, _ := s.queries.ListMediaVariants(ctx, id)
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
	// Delete old blobs after commit
	for _, k := range oldKeys {
		_ = s.store.Delete(ctx, k)
	}
	return nil
}
