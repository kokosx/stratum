package siteparts

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

func (s *Service) Create(ctx context.Context, name string) (string, error) {
	if stringsTrim(name) == "" {
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
	if err := s.validateNoCycles(ctx, partID, doc); err != nil {
		return err
	}
	part, err := s.queries.GetSitePart(ctx, partID)
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
	latest, err := qtx.GetLatestSitePartRevision(ctx, partID)
	if err != nil {
		return err
	}
	nextRev := latest.RevisionNumber + 1
	if part.Name != name {
		if err := qtx.UpdateSitePart(ctx, db.UpdateSitePartParams{Name: name, UpdatedAt: now, ID: partID}); err != nil {
			return err
		}
	} else {
		if err := qtx.UpdateSitePart(ctx, db.UpdateSitePartParams{Name: name, UpdatedAt: now, ID: partID}); err != nil {
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
		if err := s.validateNoCycles(ctx, partID, d); err != nil {
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
		if err := s.validateNoCycles(ctx, partID, doc); err != nil {
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

func (s *Service) IsReferenced(ctx context.Context, partID string) (bool, int) {
	count := 0
	if s.db == nil {
		return false, 0
	}
	rows, err := s.db.QueryContext(ctx, `SELECT document_json FROM site_part_revisions WHERE id IN (SELECT id FROM site_part_revisions WHERE (site_part_id, revision_number) IN (SELECT site_part_id, MAX(revision_number) FROM site_part_revisions GROUP BY site_part_id))`)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var js string
			if err := rows.Scan(&js); err == nil {
				if containsSitePartRef(js, partID) {
					count++
				}
			}
		}
	}
	rows2, err := s.db.QueryContext(ctx, `SELECT document_json FROM layout_template_revisions WHERE id IN (SELECT id FROM layout_template_revisions WHERE (template_id, revision_number) IN (SELECT template_id, MAX(revision_number) FROM layout_template_revisions GROUP BY template_id))`)
	if err == nil {
		defer rows2.Close()
		for rows2.Next() {
			var js string
			if err := rows2.Scan(&js); err == nil {
				if containsSitePartRef(js, partID) {
					count++
				}
			}
		}
	}
	rows3, err := s.db.QueryContext(ctx, `SELECT document_json FROM entry_revisions WHERE id IN (SELECT published_revision_id FROM entries WHERE published_revision_id IS NOT NULL)`)
	if err == nil {
		defer rows3.Close()
		for rows3.Next() {
			var js string
			if err := rows3.Scan(&js); err == nil {
				if containsSitePartRef(js, partID) {
					count++
				}
			}
		}
	}
	rows4, err := s.db.QueryContext(ctx, `SELECT document_json FROM entry_revisions WHERE (entry_id, revision_number) IN (SELECT entry_id, MAX(revision_number) FROM entry_revisions GROUP BY entry_id)`)
	if err == nil {
		defer rows4.Close()
		for rows4.Next() {
			var js string
			if err := rows4.Scan(&js); err == nil {
				if containsSitePartRef(js, partID) {
					count++
				}
			}
		}
	}
	return count > 0, count
}

func containsSitePartRef(jsonStr, targetID string) bool {
	return contains(jsonStr, targetID)
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && indexOf(s, substr) >= 0
}

func indexOf(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
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

func (s *Service) validateNoCycles(ctx context.Context, partID string, doc *document.Document) error {
	refs := collectSitePartRefs(doc)
	if len(refs) == 0 {
		return nil
	}
	visited := map[string]bool{partID: true}
	return s.checkCycleDFS(ctx, refs, visited, 1)
}

func (s *Service) checkCycleDFS(ctx context.Context, refs []string, visited map[string]bool, depth int) error {
	if depth > 16 {
		return errors.New("site part reference depth exceeds limit (possible cycle)")
	}
	for _, ref := range refs {
		if visited[ref] {
			return fmt.Errorf("cyclic site part reference detected: %s", ref)
		}
		rev, err := s.queries.GetLatestSitePartRevision(ctx, ref)
		if err != nil {
			continue
		}
		doc, err := document.Decode([]byte(rev.DocumentJson))
		if err != nil {
			continue
		}
		nested := collectSitePartRefs(doc)
		if len(nested) == 0 {
			continue
		}
		visited[ref] = true
		if err := s.checkCycleDFS(ctx, nested, visited, depth+1); err != nil {
			return err
		}
		delete(visited, ref)
	}
	return nil
}

func collectSitePartRefs(doc *document.Document) []string {
	if doc == nil {
		return nil
	}
	var out []string
	var walk func([]document.Node)
	walk = func(nodes []document.Node) {
		for _, n := range nodes {
			if n.Block == "core/site-part" {
				var settings map[string]any
				if len(n.Settings) > 0 {
					_ = json.Unmarshal(n.Settings, &settings)
					if id, ok := settings["sitePartId"].(string); ok && id != "" {
						out = append(out, id)
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
