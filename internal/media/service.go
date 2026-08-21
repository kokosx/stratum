package media

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/kokosx/stratum/internal/rendering"
	db "github.com/kokosx/stratum/internal/storage/sqlc"
)

// Service is the media domain entry point. It validates uploads, stores blobs in
// the Storage backend, persists metadata, and resolves assets for rendering.
type Service struct {
	queries *db.Queries
	store   Storage
}

func NewService(queries *db.Queries, store Storage) *Service {
	return &Service{queries: queries, store: store}
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
		vkey := "generated/" + mediaID + "-" + strconv.Itoa(v.Width) + extForMime(v.Mime)
		if err := s.store.Put(ctx, vkey, v.Data); err != nil {
			s.rollback(ctx, mediaID, origKey)
			return nil, fmt.Errorf("store variant: %w", err)
		}
		if _, err := s.queries.CreateMediaVariant(ctx, db.CreateMediaVariantParams{
			ID:         mediaID + "-v" + strconv.Itoa(v.Width),
			MediaID:    mediaID,
			Kind:       strconv.Itoa(v.Width),
			StorageKey: vkey,
			MimeType:   v.Mime,
			Width:      sql.NullInt64{Int64: int64(v.Width), Valid: true},
			Height:     sql.NullInt64{Int64: int64(v.Height), Valid: true},
			FileSize:   int64(len(v.Data)),
			CreatedAt:  now,
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
	return s.queries.UpdateMediaMetadata(ctx, db.UpdateMediaMetadataParams{
		AltText:     alt,
		Title:       title,
		Caption:     caption,
		Description: description,
		UpdatedAt:   time.Now().Unix(),
		ID:          id,
	})
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

// GenerateFaviconVariants (re)builds the square favicon sizes from the asset. It
// is safe to call whenever the Site Icon changes.
func (s *Service) GenerateFaviconVariants(ctx context.Context, id string) error {
	m, err := s.queries.GetMedia(ctx, id)
	if err != nil {
		return err
	}
	data, err := s.store.Read(ctx, m.StorageKey)
	if err != nil {
		return err
	}

	existing, _ := s.queries.ListMediaVariants(ctx, id)
	for _, v := range existing {
		if strings.HasPrefix(v.Kind, "favicon-") {
			_ = s.store.Delete(ctx, v.StorageKey)
			_ = s.queries.DeleteMediaVariant(ctx, v.ID)
		}
	}

	now := time.Now().Unix()
	for _, size := range faviconSizes {
		bytes, mime, err := FaviconVariant(data, size)
		if err != nil {
			return err
		}
		key := "generated/" + id + "-favicon-" + strconv.Itoa(size) + extForMime(mime)
		if err := s.store.Put(ctx, key, bytes); err != nil {
			return err
		}
		if _, err := s.queries.CreateMediaVariant(ctx, db.CreateMediaVariantParams{
			ID:         id + "-fav-" + strconv.Itoa(size),
			MediaID:    id,
			Kind:       "favicon-" + strconv.Itoa(size),
			StorageKey: key,
			MimeType:   mime,
			Width:      sql.NullInt64{Int64: int64(size), Valid: true},
			Height:     sql.NullInt64{Int64: int64(size), Valid: true},
			FileSize:   int64(len(bytes)),
			CreatedAt:  now,
		}); err != nil {
			return err
		}
	}
	return nil
}

// MediaView implements rendering.MediaProvider, resolving an asset id into the
// URLs and dimensions a block template needs.
func (s *Service) MediaView(ctx context.Context, id string) (rendering.MediaView, bool) {
	m, err := s.queries.GetMedia(ctx, id)
	if err != nil {
		return rendering.MediaView{}, false
	}
	variants, err := s.queries.ListMediaVariants(ctx, id)
	if err != nil {
		return rendering.MediaView{}, false
	}

	view := rendering.MediaView{
		ID:     m.ID,
		Alt:    m.AltText,
		Width:  intVal(m.Width),
		Height: intVal(m.Height),
	}

	type respVariant struct {
		width int
		url   string
	}
	resp := make([]respVariant, 0, len(variants))
	defaultWidth := 0
	for _, v := range variants {
		w := parseIntKind(v.Kind)
		if w <= 0 {
			continue
		}
		url := "/media/" + m.ID + "/" + v.Kind
		resp = append(resp, respVariant{width: w, url: url})
		if w > defaultWidth {
			defaultWidth = w
		}
	}
	sort.Slice(resp, func(i, j int) bool { return resp[i].width < resp[j].width })
	for _, r := range resp {
		if view.SrcSet != "" {
			view.SrcSet += ", "
		}
		view.SrcSet += r.url + " " + strconv.Itoa(r.width) + "w"
	}
	if defaultWidth > 0 {
		view.Src = "/media/" + m.ID + "/" + strconv.Itoa(defaultWidth)
	} else {
		view.Src = "/media/" + m.ID + "/original"
	}
	return view, true
}

// FaviconView returns the generated site-icon URLs for the theme <head>.
func (s *Service) FaviconView(ctx context.Context, id string) (rendering.FaviconView, bool) {
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
		url := "/media/" + id + "/" + v.Kind
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
	return view, true
}

// --- helpers ---

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
			ID:         v.ID,
			MediaID:    v.MediaID,
			Kind:       v.Kind,
			StorageKey: v.StorageKey,
			MimeType:   v.MimeType,
			Width:      intVal(v.Width),
			Height:     intVal(v.Height),
			FileSize:   v.FileSize,
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
