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
	"github.com/kokosx/stratum/internal/document"
	db "github.com/kokosx/stratum/internal/storage/sqlc"
)

// Reader is the minimal boundary for layout template loading.
type Reader interface {
	GetPublished(ctx context.Context, templateID string) (*document.Document, string, error)
	GetLatest(ctx context.Context, templateID string) (*document.Document, error)
}

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

const maxNestingDepth = 8

func (s *Service) Create(ctx context.Context, name, contentTypeID string) (string, error) {
	return s.CreateWithParent(ctx, name, contentTypeID, "")
}

func (s *Service) CreateWithParent(ctx context.Context, name, contentTypeID, parentID string) (string, error) {
	if stringsTrim(name) == "" {
		return "", errors.New("name is required")
	}
	if contentTypeID == "" {
		return "", errors.New("content type is required")
	}
	if _, err := s.queries.GetContentType(ctx, contentTypeID); err != nil {
		return "", errors.New("invalid content type")
	}
	if parentID != "" {
		if err := s.validateParent(ctx, "", parentID, contentTypeID); err != nil {
			return "", err
		}
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
	slotID, err := randomID()
	if err != nil {
		return "", err
	}
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
	parentNull := sql.NullString{}
	if parentID != "" {
		parentNull = sql.NullString{String: parentID, Valid: true}
	}
	if err := qtx.CreateLayoutTemplate(ctx, db.CreateLayoutTemplateParams{ID: id, Name: name, ContentTypeID: contentTypeID, PublishedRevisionID: sql.NullString{}, ParentTemplateID: parentNull, CreatedAt: now, UpdatedAt: now}); err != nil {
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
	return s.SaveDraftWithParent(ctx, templateID, name, docJSON, "", authorID, false)
}

func (s *Service) SaveDraftWithParent(ctx context.Context, templateID, name, docJSON, parentID string, authorID string, parentProvided bool) error {
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
	// Validate parent if provided
	if parentProvided {
		if parentID != "" {
			if err := s.validateParent(ctx, templateID, parentID, tmpl.ContentTypeID); err != nil {
				return err
			}
		}
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
	if parentProvided {
		if parentID == "" {
			if err := qtx.ClearLayoutTemplateParent(ctx, db.ClearLayoutTemplateParentParams{UpdatedAt: now, ID: templateID}); err != nil {
				return err
			}
		} else {
			if err := qtx.UpdateLayoutTemplateParent(ctx, db.UpdateLayoutTemplateParentParams{ParentTemplateID: sql.NullString{String: parentID, Valid: true}, UpdatedAt: now, ID: templateID}); err != nil {
				return err
			}
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
	encoded, _ := json.Marshal(doc)
	if err := qtx.CreateLayoutTemplateRevision(ctx, db.CreateLayoutTemplateRevisionParams{ID: revID, TemplateID: templateID, RevisionNumber: nextRev, DocumentJson: string(encoded), CreatedBy: createdBy, CreatedAt: now}); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Service) Publish(ctx context.Context, templateID, name, docJSON, authorID string) error {
	return s.PublishWithParent(ctx, templateID, name, docJSON, "", authorID, false)
}

func (s *Service) PublishWithParent(ctx context.Context, templateID, name, docJSON, parentID string, authorID string, parentProvided bool) error {
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
	if parentProvided && parentID != "" {
		if err := s.validateParent(ctx, templateID, parentID, tmpl.ContentTypeID); err != nil {
			return err
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
	if parentProvided {
		if parentID == "" {
			if err := qtx.ClearLayoutTemplateParent(ctx, db.ClearLayoutTemplateParentParams{UpdatedAt: now, ID: templateID}); err != nil {
				return err
			}
		} else {
			if err := qtx.UpdateLayoutTemplateParent(ctx, db.UpdateLayoutTemplateParentParams{ParentTemplateID: sql.NullString{String: parentID, Valid: true}, UpdatedAt: now, ID: templateID}); err != nil {
				return err
			}
		}
	}
	if err := qtx.SetLayoutTemplatePublishedRevision(ctx, db.SetLayoutTemplatePublishedRevisionParams{PublishedRevisionID: sql.NullString{String: revID, Valid: true}, UpdatedAt: now, ID: templateID}); err != nil {
		return err
	}
	_ = doc
	return tx.Commit()
}

func (s *Service) SetParent(ctx context.Context, templateID, parentID string) error {
	tmpl, err := s.queries.GetLayoutTemplate(ctx, templateID)
	if err != nil {
		return err
	}
	if parentID == "" {
		if s.db == nil {
			return errors.New("database is not configured")
		}
		return s.queries.UpdateLayoutTemplateParent(ctx, db.UpdateLayoutTemplateParentParams{ParentTemplateID: sql.NullString{}, UpdatedAt: time.Now().Unix(), ID: templateID})
	}
	if err := s.validateParent(ctx, templateID, parentID, tmpl.ContentTypeID); err != nil {
		return err
	}
	now := time.Now().Unix()
	if s.db != nil {
		return s.queries.UpdateLayoutTemplateParent(ctx, db.UpdateLayoutTemplateParentParams{ParentTemplateID: sql.NullString{String: parentID, Valid: true}, UpdatedAt: now, ID: templateID})
	}
	return s.queries.UpdateLayoutTemplateParent(ctx, db.UpdateLayoutTemplateParentParams{ParentTemplateID: sql.NullString{String: parentID, Valid: true}, UpdatedAt: now, ID: templateID})
}

func (s *Service) validateParent(ctx context.Context, templateID, parentID, contentTypeID string) error {
	if parentID == templateID {
		return errors.New("template cannot inherit from itself")
	}
	parent, err := s.queries.GetLayoutTemplate(ctx, parentID)
	if err != nil {
		return errors.New("parent template not found")
	}
	if parent.ContentTypeID != contentTypeID {
		return errors.New("parent template must belong to the same content type")
	}
	if !parent.PublishedRevisionID.Valid {
		return errors.New("parent template must be published")
	}
	// Cycle detection and depth limit
	visited := map[string]bool{templateID: true}
	depth := 1
	current := parentID
	for current != "" {
		if visited[current] {
			return errors.New("template nesting cycle detected")
		}
		visited[current] = true
		depth++
		if depth > maxNestingDepth {
			return fmt.Errorf("template nesting depth exceeds the %d level limit", maxNestingDepth)
		}
		t, err := s.queries.GetLayoutTemplate(ctx, current)
		if err != nil {
			break
		}
		if !t.ParentTemplateID.Valid || t.ParentTemplateID.String == "" {
			break
		}
		current = t.ParentTemplateID.String
	}
	return nil
}

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

// Ensure strings import is used
var _ = strings.TrimSpace
