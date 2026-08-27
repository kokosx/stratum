package layouts

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/kokosx/stratum/internal/blocks"
	"github.com/kokosx/stratum/internal/content"
	"github.com/kokosx/stratum/internal/document"
	db "github.com/kokosx/stratum/internal/storage/sqlc"
)

type Service struct {
	db      *sql.DB
	queries *db.Queries
	blocks  *blocks.Registry
}

type Usage struct {
	DefaultSingleFor  string
	DefaultArchiveFor string
	ExplicitEntries   int64
}

func (u Usage) InUse() bool {
	return u.DefaultSingleFor != "" || u.DefaultArchiveFor != "" || u.ExplicitEntries > 0
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
	name = strings.TrimSpace(name)
	if name == "" {
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
	definition, err := content.NewCatalog(s.queries).GetDefinition(ctx, contentTypeID)
	if err != nil {
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
	if kind == "single" && definition.Capabilities.HasContent {
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
	} else if kind == "single" {
		docJSON = `{"version":1,"nodes":[]}`
	} else if kind == "archive" {
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
	if err := qtx.UpdateLayoutTemplate(ctx, db.UpdateLayoutTemplateParams{Name: name, UpdatedAt: now, ID: templateID}); err != nil {
		return err
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
	definition, err := content.NewCatalog(s.queries).GetDefinition(ctx, tmpl.ContentTypeID)
	if err != nil {
		return fmt.Errorf("load content type: %w", err)
	}
	hasContent := definition.Capabilities.HasContent
	var docString string
	if docJSON != "" {
		d, err := document.Decode([]byte(docJSON))
		if err != nil {
			return fmt.Errorf("invalid document: %w", err)
		}
		if err := ValidateTemplateDocument(s.blocks, d, tmpl.Kind, &hasContent); err != nil {
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
			if err := ValidateTemplateDocument(s.blocks, d, tmpl.Kind, &hasContent); err != nil {
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
	definition, err := content.NewCatalog(s.queries).GetDefinition(ctx, tmpl.ContentTypeID)
	if err != nil {
		return err
	}
	if !definition.Routing.Archive {
		return errors.New("this content type does not have archive routing enabled")
	}
	now := time.Now().Unix()
	return s.queries.SetContentTypeDefaultArchiveTemplate(ctx, db.SetContentTypeDefaultArchiveTemplateParams{DefaultArchiveTemplateID: sql.NullString{String: templateID, Valid: true}, UpdatedAt: now, ID: tmpl.ContentTypeID})
}

func (s *Service) ClearDefaultArchive(ctx context.Context, contentTypeID string) error {
	now := time.Now().Unix()
	return s.queries.ClearContentTypeDefaultArchiveTemplate(ctx, db.ClearContentTypeDefaultArchiveTemplateParams{UpdatedAt: now, ID: contentTypeID})
}

func (s *Service) ClearDefault(ctx context.Context, contentTypeID string) error {
	now := time.Now().Unix()
	return s.queries.ClearContentTypeDefaultLayoutTemplate(ctx, db.ClearContentTypeDefaultLayoutTemplateParams{UpdatedAt: now, ID: contentTypeID})
}

func (s *Service) Usage(ctx context.Context, templateID string) (Usage, error) {
	var usage Usage
	if s.db == nil {
		return usage, errors.New("database is not configured")
	}
	rows, err := s.db.QueryContext(ctx, `SELECT display_name, default_layout_template_id, default_archive_template_id FROM content_types WHERE default_layout_template_id = ? OR default_archive_template_id = ?`, templateID, templateID)
	if err != nil {
		return usage, err
	}
	for rows.Next() {
		var name string
		var single, archive sql.NullString
		if err := rows.Scan(&name, &single, &archive); err != nil {
			_ = rows.Close()
			return usage, err
		}
		if single.Valid && single.String == templateID {
			usage.DefaultSingleFor = name
		}
		if archive.Valid && archive.String == templateID {
			usage.DefaultArchiveFor = name
		}
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return usage, err
	}
	_ = rows.Close()
	err = s.db.QueryRowContext(ctx, `SELECT COUNT(DISTINCT r.entry_id) FROM entry_revisions r JOIN entries e ON e.id = r.entry_id WHERE r.layout_template_id = ? AND (r.id = e.published_revision_id OR r.revision_number = (SELECT MAX(r2.revision_number) FROM entry_revisions r2 WHERE r2.entry_id = r.entry_id))`, templateID).Scan(&usage.ExplicitEntries)
	return usage, err
}

func (s *Service) Delete(ctx context.Context, templateID string) error {
	if _, err := s.queries.GetLayoutTemplate(ctx, templateID); err != nil {
		return err
	}
	usage, err := s.Usage(ctx, templateID)
	if err != nil {
		return err
	}
	if usage.InUse() {
		return errors.New("this template cannot be deleted because it is currently in use")
	}
	_, err = s.db.ExecContext(ctx, `DELETE FROM layout_templates WHERE id = ?`, templateID)
	return err
}

func (s *Service) RestoreRevision(ctx context.Context, templateID, revisionID, authorID string) (string, error) {
	revision, err := s.queries.GetLayoutTemplateRevision(ctx, revisionID)
	if err != nil {
		return "", err
	}
	if revision.TemplateID != templateID {
		return "", errors.New("revision does not belong to this template")
	}
	latest, err := s.queries.GetLatestLayoutTemplateRevision(ctx, templateID)
	if err != nil {
		return "", err
	}
	id, err := randomID()
	if err != nil {
		return "", err
	}
	createdBy := sql.NullString{}
	if authorID != "" {
		createdBy = sql.NullString{String: authorID, Valid: true}
	}
	err = s.queries.CreateLayoutTemplateRevision(ctx, db.CreateLayoutTemplateRevisionParams{ID: id, TemplateID: templateID, RevisionNumber: latest.RevisionNumber + 1, DocumentJson: revision.DocumentJson, CreatedBy: createdBy, CreatedAt: time.Now().Unix()})
	return id, err
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
