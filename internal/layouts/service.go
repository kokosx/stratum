package layouts

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/kokosx/stratum/internal/blocks"
	"github.com/kokosx/stratum/internal/document"
	db "github.com/kokosx/stratum/internal/storage/sqlc"
)

// Reader is the minimal boundary for layout template loading. Generic code
// depends on this interface, not on *sqlc.Queries, so the layout domain is
// not coupled to the storage representation (sqlc rows).
type Reader interface {
	GetPublished(ctx context.Context, templateID string) (*document.Document, string, error)
	GetLatest(ctx context.Context, templateID string) (*document.Document, error)
}

// Service holds the layout-template use-cases. HTTP handlers parse/validate
// input, call the service, and map errors to responses; they do not own
// transactions, revision numbering, or composition validation.
type Service struct {
	db      *sql.DB
	queries *db.Queries
	blocks  *blocks.Registry
}

// NewService creates a service. db may be nil for read-only callers (e.g.
// public render) that only need Reader behaviour.
func NewService(db *sql.DB, queries *db.Queries, blocks *blocks.Registry) *Service {
	return &Service{db: db, queries: queries, blocks: blocks}
}

// randomID is the local helper for IDs; injected for testability.
var randomID = defaultRandomID

func defaultRandomID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// SetRandomID overrides the ID generator (used in tests).
func SetRandomID(fn func() (string, error)) { randomID = fn }

// Create creates a new layout template with an initial single-slot revision.
func (s *Service) Create(ctx context.Context, name, contentTypeID string) (string, error) {
	if stringsTrim(name) == "" {
		return "", errors.New("name is required")
	}
	if contentTypeID == "" {
		return "", errors.New("content type is required")
	}
	if _, err := s.queries.GetContentType(ctx, contentTypeID); err != nil {
		return "", errors.New("invalid content type")
	}
	id, err := randomID()
	if err != nil {
		return "", err
	}
	revID, err := randomID()
	if err != nil {
		return "", err
	}
	now := time.Now().Unix()
	slotID, _ := randomID()
	docJSON := `{"version":1,"nodes":[{"id":"` + slotID + `","block":"core/content-slot","version":1,"props":{},"settings":{}}]}`
	if d, err := document.Decode([]byte(docJSON)); err == nil {
		if err := ValidateLayoutTemplateDocument(s.blocks, d); err != nil {
			return "", err
		}
	}
	if s.db == nil {
		return "", errors.New("database is not configured")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	qtx := s.queries.WithTx(tx)
	if err := qtx.CreateLayoutTemplate(ctx, db.CreateLayoutTemplateParams{ID: id, Name: name, ContentTypeID: contentTypeID, PublishedRevisionID: sql.NullString{}, CreatedAt: now, UpdatedAt: now}); err != nil {
		return "", err
	}
	if err := qtx.CreateLayoutTemplateRevision(ctx, db.CreateLayoutTemplateRevisionParams{ID: revID, TemplateID: id, RevisionNumber: 1, DocumentJson: docJSON, CreatedAt: now}); err != nil {
		return "", err
	}
	if err := tx.Commit(); err != nil {
		return "", err
	}
	return id, nil
}

// SaveDraft creates a new draft revision for the template.
func (s *Service) SaveDraft(ctx context.Context, templateID, name, docJSON, authorID string) error {
	if name == "" {
		return errors.New("name is required")
	}
	if docJSON == "" {
		return errors.New("document is required")
	}
	doc, err := document.Decode([]byte(docJSON))
	if err != nil {
		return fmt.Errorf("invalid document: %w", err)
	}
	if err := ValidateLayoutTemplateDocument(s.blocks, doc); err != nil {
		return err
	}
	tmpl, err := s.queries.GetLayoutTemplate(ctx, templateID)
	if err != nil {
		return err
	}
	if s.db == nil {
		return errors.New("database is not configured")
	}
	now := time.Now().Unix()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	qtx := s.queries.WithTx(tx)
	latest, err := qtx.GetLatestLayoutTemplateRevision(ctx, templateID)
	if err != nil {
		return err
	}
	nextRev := latest.RevisionNumber + 1
	if tmpl.Name != name {
		if err := qtx.UpdateLayoutTemplate(ctx, db.UpdateLayoutTemplateParams{Name: name, UpdatedAt: now, ID: templateID}); err != nil {
			return err
		}
	} else {
		_ = qtx.UpdateLayoutTemplate(ctx, db.UpdateLayoutTemplateParams{Name: name, UpdatedAt: now, ID: templateID})
	}
	revID, _ := randomID()
	var createdBy sql.NullString
	if authorID != "" {
		createdBy = sql.NullString{String: authorID, Valid: true}
	}
	encoded, _ := json.Marshal(doc)
	if err := qtx.CreateLayoutTemplateRevision(ctx, db.CreateLayoutTemplateRevisionParams{ID: revID, TemplateID: templateID, RevisionNumber: nextRev, DocumentJson: string(encoded), CreatedBy: createdBy, CreatedAt: now}); err != nil {
		return err
	}
	return tx.Commit()
}

// Publish publishes the template. If docJSON is empty, the latest revision is used.
// If docJSON differs from latest, a new revision is created first. Duplicate
// consecutive publishes with identical document do not create a new revision.
func (s *Service) Publish(ctx context.Context, templateID, name, docJSON, authorID string) error {
	tmpl, err := s.queries.GetLayoutTemplate(ctx, templateID)
	if err != nil {
		return err
	}
	if name == "" {
		name = tmpl.Name
	}
	var doc *document.Document
	var docString string
	if docJSON != "" {
		d, err := document.Decode([]byte(docJSON))
		if err != nil {
			return fmt.Errorf("invalid document: %w", err)
		}
		if err := ValidateLayoutTemplateDocument(s.blocks, d); err != nil {
			return err
		}
		doc = d
		enc, _ := json.Marshal(d)
		docString = string(enc)
	} else {
		latest, err := s.queries.GetLatestLayoutTemplateRevision(ctx, templateID)
		if err != nil {
			return err
		}
		docString = latest.DocumentJson
		d, _ := document.Decode([]byte(docString))
		doc = d
	}
	if s.db == nil {
		return errors.New("database is not configured")
	}
	now := time.Now().Unix()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	qtx := s.queries.WithTx(tx)
	latest, err := qtx.GetLatestLayoutTemplateRevision(ctx, templateID)
	if err != nil {
		return err
	}
	// If docJSON was supplied and equals latest, no new revision needed.
	needNewRev := docJSON != "" && docString != latest.DocumentJson
	revID := latest.ID
	if needNewRev {
		revID, _ = randomID()
		nextRev := latest.RevisionNumber + 1
		var createdBy sql.NullString
		if authorID != "" {
			createdBy = sql.NullString{String: authorID, Valid: true}
		}
		if err := qtx.CreateLayoutTemplateRevision(ctx, db.CreateLayoutTemplateRevisionParams{ID: revID, TemplateID: templateID, RevisionNumber: nextRev, DocumentJson: docString, CreatedBy: createdBy, CreatedAt: now}); err != nil {
			return err
		}
		if err := qtx.UpdateLayoutTemplate(ctx, db.UpdateLayoutTemplateParams{Name: name, UpdatedAt: now, ID: templateID}); err != nil {
			return err
		}
	} else if tmpl.Name != name {
		if err := qtx.UpdateLayoutTemplate(ctx, db.UpdateLayoutTemplateParams{Name: name, UpdatedAt: now, ID: templateID}); err != nil {
			return err
		}
	}
	if err := qtx.SetLayoutTemplatePublishedRevision(ctx, db.SetLayoutTemplatePublishedRevisionParams{PublishedRevisionID: sql.NullString{String: revID, Valid: true}, UpdatedAt: now, ID: templateID}); err != nil {
		return err
	}
	_ = doc
	return tx.Commit()
}

// SetDefault marks the template as the default for its content type.
func (s *Service) SetDefault(ctx context.Context, templateID string) error {
	tmpl, err := s.queries.GetLayoutTemplate(ctx, templateID)
	if err != nil {
		return err
	}
	if !tmpl.PublishedRevisionID.Valid {
		return errors.New("template must be published to be default")
	}
	now := time.Now().Unix()
	return s.queries.SetContentTypeDefaultLayoutTemplate(ctx, db.SetContentTypeDefaultLayoutTemplateParams{DefaultLayoutTemplateID: sql.NullString{String: templateID, Valid: true}, UpdatedAt: now, ID: tmpl.ContentTypeID})
}

// ResolveEffectiveDocument is the single application-boundary resolver for
// Entry → Layout composition. It loads the published layout revision,
// composes, and **always** validates the composed SDT via block registry.
// Handlers must not replicate this logic.
func (s *Service) ResolveEffectiveDocument(ctx context.Context, entryDoc *document.Document, contentTypeID string, layoutTemplateID sql.NullString) (*document.Document, string, error) {
	if !layoutTemplateID.Valid || layoutTemplateID.String == "" {
		return entryDoc, "", nil
	}
	doc, revID, err := ResolveEffectiveDocumentWithID(ctx, s.queries, entryDoc, contentTypeID, layoutTemplateID)
	if err != nil {
		return nil, "", err
	}
	if doc != entryDoc && s.blocks != nil {
		if err := s.blocks.ValidateDocument(doc); err != nil {
			return nil, "", fmt.Errorf("composed document invalid: %w", err)
		}
	}
	return doc, revID, nil
}

func stringsTrim(s string) string {
	// local helper to avoid importing strings for a single call
	if len(s) == 0 {
		return s
	}
	j := 0
	for j < len(s) && (s[j] == ' ' || s[j] == '\n' || s[j] == '\t' || s[j] == '\r') {
		j++
	}
	k := len(s)
	for k > j && (s[k-1] == ' ' || s[k-1] == '\n' || s[k-1] == '\t' || s[k-1] == '\r') {
		k--
	}
	return s[j:k]
}
