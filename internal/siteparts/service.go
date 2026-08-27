package siteparts

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

func (s *Service) Create(ctx context.Context, name string) (string, error) {
	return s.CreateForLocation(ctx, name, "")
}

func (s *Service) CreateForLocation(ctx context.Context, name, location string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", errors.New("name is required")
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
	docJSON := `{"version":1,"nodes":[{"id":"` + id + `-root","block":"core/section","version":1,"props":{},"settings":{},"children":[{"id":"` + id + `-text","block":"core/text","version":1,"props":{"text":"New site part"},"settings":{}}]}]}`
	if location == "header" || location == "footer" {
		menuLocation := "primary"
		if location == "footer" {
			menuLocation = "footer"
		}
		docJSON = `{"version":1,"nodes":[{"id":"` + id + `-root","block":"core/section","version":1,"props":{},"settings":{},"children":[{"id":"` + id + `-stack","block":"core/stack","version":1,"props":{},"settings":{},"children":[{"id":"` + id + `-logo","block":"core/site-logo","version":1,"props":{},"settings":{}},{"id":"` + id + `-nav","block":"core/navigation","version":1,"props":{},"settings":{"location":"` + menuLocation + `"}}]}]}]}`
	}
	if d, err := document.Decode([]byte(docJSON)); err == nil {
		if err := ValidateSitePartDocument(s.blocks, d); err != nil {
			docJSON = `{"version":1,"nodes":[]}`
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
	if err := qtx.CreateSitePart(ctx, db.CreateSitePartParams{ID: id, Name: name, PublishedRevisionID: sql.NullString{}, CreatedAt: now, UpdatedAt: now}); err != nil {
		return "", err
	}
	if err := qtx.CreateSitePartRevision(ctx, db.CreateSitePartRevisionParams{ID: revID, SitePartID: id, RevisionNumber: 1, DocumentJson: docJSON, CreatedAt: now}); err != nil {
		return "", err
	}
	if err := tx.Commit(); err != nil {
		return "", err
	}
	return id, nil
}

func (s *Service) SaveDraft(ctx context.Context, partID, name, docJSON, authorID string) error {
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
	if err := ValidateSitePartDocument(s.blocks, doc); err != nil {
		return err
	}
	if err := s.validateNoCycles(ctx, partID, doc, false); err != nil {
		return err
	}
	if _, err := s.queries.GetSitePart(ctx, partID); err != nil {
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
	latest, err := qtx.GetLatestSitePartRevision(ctx, partID)
	if err != nil {
		return err
	}
	nextRev := latest.RevisionNumber + 1
	if err := qtx.UpdateSitePart(ctx, db.UpdateSitePartParams{Name: name, UpdatedAt: now, ID: partID}); err != nil {
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
	if err := qtx.CreateSitePartRevision(ctx, db.CreateSitePartRevisionParams{ID: revID, SitePartID: partID, RevisionNumber: nextRev, DocumentJson: string(encoded), CreatedBy: createdBy, CreatedAt: now}); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Service) Publish(ctx context.Context, partID, name, docJSON, authorID string) error {
	part, err := s.queries.GetSitePart(ctx, partID)
	if err != nil {
		return err
	}
	if name == "" {
		name = part.Name
	}
	var docString string
	var doc *document.Document
	if docJSON != "" {
		d, err := document.Decode([]byte(docJSON))
		if err != nil {
			return fmt.Errorf("invalid document: %w", err)
		}
		if err := ValidateSitePartDocument(s.blocks, d); err != nil {
			return err
		}
		if err := s.validateNoCycles(ctx, partID, d, true); err != nil {
			return err
		}
		enc, err := json.Marshal(d)
		if err != nil {
			return fmt.Errorf("marshal document: %w", err)
		}
		docString = string(enc)
		doc = d
	} else {
		latest, err := s.queries.GetLatestSitePartRevision(ctx, partID)
		if err != nil {
			return err
		}
		docString = latest.DocumentJson
		if d, err := document.Decode([]byte(docString)); err == nil {
			doc = d
		}
		if err := ValidateSitePartDocument(s.blocks, doc); err != nil {
			return err
		}
		if err := s.validateNoCycles(ctx, partID, doc, true); err != nil {
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
	latest, err := qtx.GetLatestSitePartRevision(ctx, partID)
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
		if err := qtx.CreateSitePartRevision(ctx, db.CreateSitePartRevisionParams{ID: revID, SitePartID: partID, RevisionNumber: nextRev, DocumentJson: docString, CreatedBy: createdBy, CreatedAt: now}); err != nil {
			return err
		}
		if err := qtx.UpdateSitePart(ctx, db.UpdateSitePartParams{Name: name, UpdatedAt: now, ID: partID}); err != nil {
			return err
		}
	} else if part.Name != name {
		if err := qtx.UpdateSitePart(ctx, db.UpdateSitePartParams{Name: name, UpdatedAt: now, ID: partID}); err != nil {
			return err
		}
	}
	if err := qtx.SetSitePartPublishedRevision(ctx, db.SetSitePartPublishedRevisionParams{PublishedRevisionID: sql.NullString{String: revID, Valid: true}, UpdatedAt: now, ID: partID}); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Service) SetLocation(ctx context.Context, location, sitePartID string) error {
	if location != "header" && location != "footer" {
		return errors.New("invalid location")
	}
	if sitePartID != "" {
		part, err := s.queries.GetSitePart(ctx, sitePartID)
		if err != nil {
			return errors.New("site part not found")
		}
		if !part.PublishedRevisionID.Valid {
			return errors.New("site part must be published to be assigned")
		}
	}
	now := time.Now().Unix()
	var sid sql.NullString
	if sitePartID != "" {
		sid = sql.NullString{String: sitePartID, Valid: true}
	}
	return s.setLocationInternal(ctx, location, sid, now)
}

func (s *Service) setLocationInternal(ctx context.Context, location string, sid sql.NullString, now int64) error {
	if sid.Valid {
		return s.queries.SetSitePartLocation(ctx, db.SetSitePartLocationParams{Location: location, SitePartID: sid, UpdatedAt: now})
	}
	return s.queries.ClearSitePartLocation(ctx, location)
}

func (s *Service) GetLocation(ctx context.Context, location string) (db.SitePartLocation, error) {
	return s.queries.GetSitePartLocation(ctx, location)
}

func (s *Service) RestoreRevision(ctx context.Context, partID, revisionID, authorID string) (string, error) {
	revision, err := s.queries.GetSitePartRevision(ctx, revisionID)
	if err != nil {
		return "", err
	}
	if revision.SitePartID != partID {
		return "", errors.New("revision does not belong to this Site Part")
	}
	latest, err := s.queries.GetLatestSitePartRevision(ctx, partID)
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
	err = s.queries.CreateSitePartRevision(ctx, db.CreateSitePartRevisionParams{ID: id, SitePartID: partID, RevisionNumber: latest.RevisionNumber + 1, DocumentJson: revision.DocumentJson, CreatedBy: createdBy, CreatedAt: time.Now().Unix()})
	return id, err
}

func (s *Service) ListLocations(ctx context.Context) ([]db.SitePartLocation, error) {
	return s.queries.ListSitePartLocations(ctx)
}

func (s *Service) IsUsedAsHeaderOrFooter(ctx context.Context, partID string) (bool, string) {
	locs, err := s.queries.ListSitePartLocations(ctx)
	if err != nil {
		return false, ""
	}
	for _, l := range locs {
		if l.SitePartID.Valid && l.SitePartID.String == partID {
			return true, l.Location
		}
	}
	return false, ""
}

func (s *Service) IsReferenced(ctx context.Context, partID string) (bool, int, error) {
	if s.db == nil {
		return false, 0, errors.New("database is not configured")
	}
	owners := make(map[string]struct{})
	queries := []struct {
		prefix string
		sql    string
	}{
		{"site-part:", `SELECT site_part_id, document_json FROM site_part_revisions WHERE (site_part_id, revision_number) IN (SELECT site_part_id, MAX(revision_number) FROM site_part_revisions GROUP BY site_part_id)`},
		{"site-part:", `SELECT p.id, r.document_json FROM site_parts p JOIN site_part_revisions r ON r.id = p.published_revision_id`},
		{"template:", `SELECT template_id, document_json FROM layout_template_revisions WHERE (template_id, revision_number) IN (SELECT template_id, MAX(revision_number) FROM layout_template_revisions GROUP BY template_id)`},
		{"template:", `SELECT t.id, r.document_json FROM layout_templates t JOIN layout_template_revisions r ON r.id = t.published_revision_id`},
		{"entry:", `SELECT entry_id, document_json FROM entry_revisions WHERE (entry_id, revision_number) IN (SELECT entry_id, MAX(revision_number) FROM entry_revisions GROUP BY entry_id)`},
		{"entry:", `SELECT e.id, r.document_json FROM entries e JOIN entry_revisions r ON r.id = e.published_revision_id`},
	}
	for _, query := range queries {
		rows, err := s.db.QueryContext(ctx, query.sql)
		if err != nil {
			return false, 0, err
		}
		for rows.Next() {
			var ownerID, raw string
			if err := rows.Scan(&ownerID, &raw); err != nil {
				continue
			}
			doc, err := document.Decode([]byte(raw))
			if err == nil && ReferencesSitePart(doc, partID) {
				owners[query.prefix+ownerID] = struct{}{}
			}
		}
		if err := rows.Err(); err != nil {
			_ = rows.Close()
			return false, 0, err
		}
		_ = rows.Close()
	}
	return len(owners) > 0, len(owners), nil
}

func (s *Service) validateNoCycles(ctx context.Context, partID string, doc *document.Document, publishedGraph bool) error {
	refs := CollectSitePartRefs(doc)
	if len(refs) == 0 {
		return nil
	}
	visited := map[string]bool{partID: true}
	return s.checkCycleDFS(ctx, refs, visited, 1, publishedGraph)
}

func (s *Service) checkCycleDFS(ctx context.Context, refs []string, visited map[string]bool, depth int, publishedGraph bool) error {
	if depth > 16 {
		return errors.New("site part reference depth exceeds limit (possible cycle)")
	}
	for _, ref := range refs {
		if visited[ref] {
			return fmt.Errorf("cyclic site part reference detected: %s", ref)
		}
		part, err := s.queries.GetSitePart(ctx, ref)
		if err != nil {
			if publishedGraph {
				return fmt.Errorf("referenced Site Part %q does not exist", ref)
			}
			continue
		}
		var rev db.SitePartRevision
		if publishedGraph {
			if !part.PublishedRevisionID.Valid {
				return fmt.Errorf("Referenced Site Part %q is not published.", part.Name)
			}
			rev, err = s.queries.GetPublishedSitePartRevision(ctx, ref)
		} else {
			rev, err = s.queries.GetLatestSitePartRevision(ctx, ref)
		}
		if err != nil {
			return fmt.Errorf("load referenced Site Part %q: %w", part.Name, err)
		}
		doc, err := document.Decode([]byte(rev.DocumentJson))
		if err != nil {
			continue
		}
		nested := CollectSitePartRefs(doc)
		if len(nested) == 0 {
			continue
		}
		visited[ref] = true
		if err := s.checkCycleDFS(ctx, nested, visited, depth+1, publishedGraph); err != nil {
			return err
		}
		delete(visited, ref)
	}
	return nil
}

// CollectSitePartRefs returns exact, de-duplicated core/site-part references
// discovered by traversing SDT nodes. Arbitrary JSON text is never searched.
func CollectSitePartRefs(doc *document.Document) []string {
	if doc == nil {
		return nil
	}
	var out []string
	seen := make(map[string]struct{})
	var walk func([]document.Node)
	walk = func(nodes []document.Node) {
		for _, n := range nodes {
			if n.Block == "core/site-part" {
				var settings map[string]any
				if len(n.Settings) > 0 {
					_ = json.Unmarshal(n.Settings, &settings)
					if id, ok := settings["sitePartId"].(string); ok && id != "" {
						if _, exists := seen[id]; !exists {
							seen[id] = struct{}{}
							out = append(out, id)
						}
					}
				}
			}
			if len(n.Children) > 0 {
				walk(n.Children)
			}
		}
	}
	walk(doc.Nodes)
	return out
}

func ReferencesSitePart(doc *document.Document, partID string) bool {
	for _, id := range CollectSitePartRefs(doc) {
		if id == partID {
			return true
		}
	}
	return false
}
