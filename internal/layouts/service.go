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

type Service struct {
	db      *sql.DB
	queries *db.Queries
	blocks  *blocks.Registry
}

func NewService(db *sql.DB, queries *db.Queries, blocks *blocks.Registry) *Service {
	return &Service{db: db, queries: queries, blocks: blocks}
}

var randomID = defaultRandomID

func defaultRandomID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func SetRandomID(fn func() (string, error)) { randomID = fn }

func (s *Service) Create(ctx context.Context, name, contentTypeID string) (string, error) {
	return s.CreateWithKind(ctx, name, contentTypeID, "single")
}

func (s *Service) CreateWithKind(ctx context.Context, name, contentTypeID, kind string) (string, error) {
	if stringsTrim(name) == "" {
		return "", errors.New("name is required")
	}
	if contentTypeID == "" {
		return "", errors.New("content type is required")
	}
	if kind == "" {
		kind = "single"
	}
	if kind != "single" && kind != "archive" {
		return "", errors.New("invalid template kind")
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
	var docJSON string
	if kind == "single" {
		slotID, err := randomID()
		if err != nil {
			return "", err
		}
		docJSON = `{"version":1,"nodes":[{"id":"` + slotID + `","block":"core/content-slot","version":1,"props":{},"settings":{}}]}`
		// Validate but allow HasContent logic later; for now use generic validation
		if d, err := document.Decode([]byte(docJSON)); err == nil {
			if err := ValidateLayoutTemplateDocumentForKind(s.blocks, d, kind, nil); err != nil {
				return "", err
			}
		}
	} else {
		// archive starter: archive title + collection(context)
		titleID, _ := randomID()
		collID, _ := randomID()
		entryTitleID, _ := randomID()
		docJSON = `{"version":1,"nodes":[{"id":"` + titleID + `","block":"core/archive-title","version":1,"props":{},"settings":{}},{"id":"` + collID + `","block":"core/collection","version":2,"props":{},"settings":{"source":"context","limit":10},"children":[{"id":"` + entryTitleID + `","block":"core/entry-title","version":1,"props":{},"settings":{}}]}]}`
		if d, err := document.Decode([]byte(docJSON)); err == nil {
			if err := ValidateLayoutTemplateDocumentForKind(s.blocks, d, kind, nil); err != nil {
				// fallback to empty
				docJSON = `{"version":1,"nodes":[]}`
			}
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
	if err := qtx.CreateLayoutTemplate(ctx, db.CreateLayoutTemplateParams{ID: id, Name: name, ContentTypeID: contentTypeID, Kind: kind, PublishedRevisionID: sql.NullString{}, CreatedAt: now, UpdatedAt: now}); err != nil {
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
	tmpl, err := s.queries.GetLayoutTemplate(ctx, templateID)
	if err != nil {
		return err
	}
	if err := ValidateTemplateDocument(s.blocks, doc, tmpl.Kind, nil); err != nil {
		return err
	}
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
		if err := qtx.UpdateLayoutTemplate(ctx, db.UpdateLayoutTemplateParams{Name: name, UpdatedAt: now, ID: templateID}); err != nil {
			return err
		}
	}
	revID, err := randomID()
	if err != nil {
		return err
	}
	var createdBy sql.NullString
	if authorID != "" {
		createdBy = sql.NullString{String: authorID, Valid: true}
	}
	encoded, err := json.Marshal(doc)
	if err != nil {
		return fmt.Errorf("marshal document: %w", err)
	}
	if err := qtx.CreateLayoutTemplateRevision(ctx, db.CreateLayoutTemplateRevisionParams{ID: revID, TemplateID: templateID, RevisionNumber: nextRev, DocumentJson: string(encoded), CreatedBy: createdBy, CreatedAt: now}); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Service) Publish(ctx context.Context, templateID, name, docJSON, authorID string) error {
	tmpl, err := s.queries.GetLayoutTemplate(ctx, templateID)
	if err != nil {
		return err
	}
	if name == "" {
		name = tmpl.Name
	}
	var docString string
	if docJSON != "" {
		d, err := document.Decode([]byte(docJSON))
		if err != nil {
			return fmt.Errorf("invalid document: %w", err)
		}
		if err := ValidateTemplateDocument(s.blocks, d, tmpl.Kind, nil); err != nil {
			return err
		}
		enc, err := json.Marshal(d)
		if err != nil {
			return fmt.Errorf("marshal document: %w", err)
		}
		docString = string(enc)
	} else {
		latest, err := s.queries.GetLatestLayoutTemplateRevision(ctx, templateID)
		if err != nil {
			return err
		}
		docString = latest.DocumentJson
		if d, err := document.Decode([]byte(docString)); err == nil {
			if err := ValidateTemplateDocument(s.blocks, d, tmpl.Kind, nil); err != nil {
				return err
			}
		}
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
	needNewRev := docJSON != "" && docString != latest.DocumentJson
	revID := latest.ID
	if needNewRev {
		var nidErr error
		revID, nidErr = randomID()
		if nidErr != nil {
			return nidErr
		}
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
	return tx.Commit()
}

func (s *Service) SetDefault(ctx context.Context, templateID string) error {
	tmpl, err := s.queries.GetLayoutTemplate(ctx, templateID)
	if err != nil {
		return err
	}
	if tmpl.Kind != "single" {
		return errors.New("only Single templates can be set as single default")
	}
	if !tmpl.PublishedRevisionID.Valid {
		return errors.New("template must be published to be default")
	}
	now := time.Now().Unix()
	return s.queries.SetContentTypeDefaultLayoutTemplate(ctx, db.SetContentTypeDefaultLayoutTemplateParams{DefaultLayoutTemplateID: sql.NullString{String: templateID, Valid: true}, UpdatedAt: now, ID: tmpl.ContentTypeID})
}

func (s *Service) SetDefaultArchive(ctx context.Context, templateID string) error {
	tmpl, err := s.queries.GetLayoutTemplate(ctx, templateID)
	if err != nil {
		return err
	}
	if tmpl.Kind != "archive" {
		return errors.New("only Archive templates can be set as archive default")
	}
	if !tmpl.PublishedRevisionID.Valid {
		return errors.New("template must be published to be default")
	}
	now := time.Now().Unix()
	return s.queries.SetContentTypeDefaultArchiveTemplate(ctx, db.SetContentTypeDefaultArchiveTemplateParams{DefaultArchiveTemplateID: sql.NullString{String: templateID, Valid: true}, UpdatedAt: now, ID: tmpl.ContentTypeID})
}

func (s *Service) ClearDefaultArchive(ctx context.Context, contentTypeID string) error {
	now := time.Now().Unix()
	return s.queries.ClearContentTypeDefaultArchiveTemplate(ctx, db.ClearContentTypeDefaultArchiveTemplateParams{UpdatedAt: now, ID: contentTypeID})
}

func (s *Service) ResolveEffectiveDocument(ctx context.Context, entryDoc *document.Document, contentTypeID string, layoutTemplateID sql.NullString) (*document.Document, string, error) {
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
