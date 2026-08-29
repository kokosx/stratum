package media

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

// ListParams controls the library query.
type ListParams struct {
	Search string
	Filter string // "all" | "missing_alt" | "unused"
	Limit  int
	Offset int
}

// ListFiltered returns a paginated, filtered, searched slice and the total count before pagination.
// It is the single query entry point for both library and picker (different callers pass different filters).
func (s *Service) ListFiltered(ctx context.Context, p ListParams) ([]Asset, int64, error) {
	if p.Limit <= 0 {
		p.Limit = 40
	}
	if p.Limit > 100 {
		p.Limit = 100
	}
	if p.Offset < 0 {
		p.Offset = 0
	}
	search := strings.TrimSpace(p.Search)
	filter := strings.TrimSpace(p.Filter)
	if filter == "" {
		filter = "all"
	}

	// If we have a DB handle, use SQL with search + missing_alt filtering andpaginate at DB level for efficiency.
	// For unused filter we need post-filter via usage scanning, so we fetch candidates then filter in Go.
	if s.db != nil {
		return s.listFilteredWithDB(ctx, search, filter, p.Limit, p.Offset)
	}
	// Fallback for tests without DB handle: fetch via queries and filter in memory.
	return s.listFilteredFallback(ctx, search, filter, p.Limit, p.Offset)
}

func (s *Service) listFilteredWithDB(ctx context.Context, search, filter string, limit, offset int) ([]Asset, int64, error) {
	// Build WHERE clauses
	var where []string
	var args []any
	if search != "" {
		like := "%" + search + "%"
		where = append(where, "(original_filename LIKE ? COLLATE NOCASE OR title LIKE ? COLLATE NOCASE OR alt_text LIKE ? COLLATE NOCASE OR caption LIKE ? COLLATE NOCASE)")
		args = append(args, like, like, like, like)
	}
	if filter == "missing_alt" {
		where = append(where, "alt_text = '' AND asset_type = 'image'")
	}
	whereSQL := ""
	if len(where) > 0 {
		whereSQL = "WHERE " + strings.Join(where, " AND ")
	}

	// For unused, we need to compute unused IDs separately and filter in Go after fetching.
	// We'll fetch candidates without pagination first if unused, then filter.
	if filter == "unused" {
		// Fetch all candidates matching search (and missing_alt not applicable together? If both, missing_alt not relevant)
		query := fmt.Sprintf("SELECT id, original_filename, storage_key, mime_type, asset_type, file_size, width, height, alt_text, title, caption, description, author_id, created_at, updated_at FROM media %s ORDER BY created_at DESC", whereSQL)
		rows, err := s.db.QueryContext(ctx, query, args...)
		if err != nil {
			return nil, 0, err
		}
		defer rows.Close()
		var all []Asset
		for rows.Next() {
			var w, h sql.NullInt64
			var author sql.NullString
			var a Asset
			var created, updated int64
			var id, fn, sk, mime, title, caption, desc, alt string
			var fsize int64
			var assetType string
			if err := rows.Scan(&id, &fn, &sk, &mime, &assetType, &fsize, &w, &h, &alt, &title, &caption, &desc, &author, &created, &updated); err != nil {
				return nil, 0, err
			}
			a.ID = id
			a.OriginalFilename = fn
			a.StorageKey = sk
			a.MimeType = mime
			a.AssetType = assetType
			a.FileSize = fsize
			if w.Valid {
				a.Width = int(w.Int64)
			}
			if h.Valid {
				a.Height = int(h.Int64)
			}
			a.AltText = alt
			a.Title = title
			a.Caption = caption
			a.Description = desc
			a.AuthorID = author.String
			if author.Valid {
				a.AuthorID = author.String
			}
			// created/updated not needed for Asset but keep
			_ = created
			_ = updated
			all = append(all, a)
		}
		if err := rows.Err(); err != nil {
			return nil, 0, err
		}
		// Filter unused via usage scan
		unused := make([]Asset, 0)
		for _, a := range all {
			used, err := s.isUsed(ctx, a.ID)
			if err != nil {
				return nil, 0, err
			}
			if !used {
				unused = append(unused, a)
			}
		}
		total := int64(len(unused))
		// Paginate in memory
		if offset >= len(unused) {
			return []Asset{}, total, nil
		}
		end := offset + limit
		if end > len(unused) {
			end = len(unused)
		}
		return unused[offset:end], total, nil
	}

	// Normal path: count then paginate at DB level
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM media %s", whereSQL)
	var total int64
	if err := s.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	query := fmt.Sprintf("SELECT id, original_filename, storage_key, mime_type, asset_type, file_size, width, height, alt_text, title, caption, description, author_id, created_at, updated_at FROM media %s ORDER BY created_at DESC LIMIT ? OFFSET ?", whereSQL)
	args = append(args, limit, offset)
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []Asset
	for rows.Next() {
		var w, h sql.NullInt64
		var author sql.NullString
		var a Asset
		var id, fn, sk, mime, assetType, alt, title, caption, desc string
		var fsize int64
		var created, updated int64
		if err := rows.Scan(&id, &fn, &sk, &mime, &assetType, &fsize, &w, &h, &alt, &title, &caption, &desc, &author, &created, &updated); err != nil {
			return nil, 0, err
		}
		a.ID = id
		a.OriginalFilename = fn
		a.StorageKey = sk
		a.MimeType = mime
		a.AssetType = assetType
		a.FileSize = fsize
		if w.Valid {
			a.Width = int(w.Int64)
		}
		if h.Valid {
			a.Height = int(h.Int64)
		}
		a.AltText = alt
		a.Title = title
		a.Caption = caption
		a.Description = desc
		if author.Valid {
			a.AuthorID = author.String
		}
		_ = created
		_ = updated
		out = append(out, a)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return out, total, nil
}

func (s *Service) listFilteredFallback(ctx context.Context, search, filter string, limit, offset int) ([]Asset, int64, error) {
	// Load via existing List with large limit then filter in Go
	all, err := s.List(ctx, 10000, 0)
	if err != nil {
		return nil, 0, err
	}
	var filtered []Asset
	lowerSearch := strings.ToLower(search)
	for _, a := range all {
		if search != "" {
			if !strings.Contains(strings.ToLower(a.OriginalFilename), lowerSearch) &&
				!strings.Contains(strings.ToLower(a.Title), lowerSearch) &&
				!strings.Contains(strings.ToLower(a.AltText), lowerSearch) &&
				!strings.Contains(strings.ToLower(a.Caption), lowerSearch) {
				continue
			}
		}
		if filter == "missing_alt" {
			if strings.TrimSpace(a.AltText) != "" || a.AssetType != string(AssetTypeImage) {
				continue
			}
		}
		if filter == "unused" {
			used, err := s.isUsed(ctx, a.ID)
			if err != nil {
				return nil, 0, err
			}
			if used {
				continue
			}
		}
		filtered = append(filtered, a)
	}
	total := int64(len(filtered))
	if offset >= len(filtered) {
		return []Asset{}, total, nil
	}
	end := offset + limit
	if end > len(filtered) {
		end = len(filtered)
	}
	return filtered[offset:end], total, nil
}

// isUsed reports whether media is referenced anywhere (published or draft)
func (s *Service) isUsed(ctx context.Context, id string) (bool, error) {
	refs, err := s.UsageRefs(ctx, id)
	if err != nil {
		return false, err
	}
	return len(refs) > 0, nil
}
