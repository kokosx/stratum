package media

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync/atomic"

	"github.com/kokosx/stratum/internal/content"
	"github.com/kokosx/stratum/internal/document"
)

// UsageRef describes one place that references a media asset.
type UsageRef struct {
	SourceKind  string `json:"sourceKind"` // entry, template, sitePart, settings
	SourceID    string `json:"sourceId"`
	SourceLabel string `json:"sourceLabel"`
	Context     string `json:"context"` // e.g., "Image block", "Featured image", "Site Icon"
	Public      bool   `json:"public"`
	EditURL     string `json:"editUrl"`
}

// UsageIndex is a request-local snapshot of all media references.
// It maps mediaID -> usage refs. Built once per request then used for
// cheap lookups (IsUsed / Refs). No persistent storage.
type UsageIndex struct {
	refsByMediaID map[string][]UsageRef
}

// IsUsed reports whether mediaID has any current references.
func (idx *UsageIndex) IsUsed(mediaID string) bool {
	if idx == nil || idx.refsByMediaID == nil {
		return false
	}
	refs, ok := idx.refsByMediaID[mediaID]
	return ok && len(refs) > 0
}

// Refs returns all current usage refs for mediaID. The slice is sorted
// deterministically (published first, then kind/label).
func (idx *UsageIndex) Refs(mediaID string) []UsageRef {
	if idx == nil || idx.refsByMediaID == nil {
		return nil
	}
	refs := idx.refsByMediaID[mediaID]
	// Return a copy to prevent caller mutation of internal map.
	out := make([]UsageRef, len(refs))
	copy(out, refs)
	return out
}

// BuildUsageIndex scans the entire site once and produces a UsageIndex.
// It inspects only published + latest working revisions (deduplicated) for
// entries, templates and site parts. Historical revisions are ignored.
// On any scanning error it returns an error (fail-safe: caller must not delete).
func (s *Service) BuildUsageIndex(ctx context.Context) (*UsageIndex, error) {
	idx := &UsageIndex{refsByMediaID: make(map[string][]UsageRef)}
	atomic.AddInt64(&s.usageIndexBuilds, 1)

	if s.db == nil {
		return idx, nil
	}

	// 1. Site settings (icon, logo, social) – always public
	if err := s.scanSiteSettings(ctx, idx); err != nil {
		return nil, fmt.Errorf("scan site settings: %w", err)
	}

	// 2. Load content type definitions for typed field filtering
	defs, err := s.loadContentDefinitions(ctx)
	if err != nil {
		return nil, err
	}

	// 3. Entries (published + latest)
	if err := s.scanEntries(ctx, idx, defs); err != nil {
		return nil, err
	}

	// 4. Layout templates (published + latest)
	if err := s.scanTemplates(ctx, idx); err != nil {
		return nil, err
	}

	// 5. Site parts (published + latest)
	if err := s.scanSiteParts(ctx, idx); err != nil {
		return nil, err
	}

	// Deterministic ordering per mediaID
	for mediaID, refs := range idx.refsByMediaID {
		sort.Slice(refs, func(i, j int) bool {
			if refs[i].Public != refs[j].Public {
				return refs[i].Public && !refs[j].Public
			}
			if refs[i].SourceKind != refs[j].SourceKind {
				return refs[i].SourceKind < refs[j].SourceKind
			}
			if refs[i].SourceLabel != refs[j].SourceLabel {
				return refs[i].SourceLabel < refs[j].SourceLabel
			}
			return refs[i].SourceID < refs[j].SourceID
		})
		idx.refsByMediaID[mediaID] = refs
	}

	return idx, nil
}

// UsageBuildCount returns how many times BuildUsageIndex has been called.
// Used in tests to verify Unused filter scans once per request.
func (s *Service) UsageBuildCount() int64 {
	return atomic.LoadInt64(&s.usageIndexBuilds)
}

func (s *Service) loadContentDefinitions(ctx context.Context) (map[string]content.ContentTypeDefinition, error) {
	if s.queries == nil {
		return map[string]content.ContentTypeDefinition{}, nil
	}
	cat := content.NewCatalog(s.queries)
	defs, err := cat.ListDefinitions(ctx)
	if err != nil {
		return nil, fmt.Errorf("load content types: %w", err)
	}
	m := make(map[string]content.ContentTypeDefinition, len(defs))
	for _, d := range defs {
		m[string(d.ID)] = d
	}
	return m, nil
}

func (s *Service) scanSiteSettings(ctx context.Context, idx *UsageIndex) error {
	var icon, logo, social sql.NullString
	err := s.db.QueryRowContext(ctx, `SELECT site_icon_media_id, site_logo_media_id, site_social_media_id FROM site_settings WHERE id=1`).Scan(&icon, &logo, &social)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil
		}
		return err
	}
	if icon.Valid && icon.String != "" {
		idx.refsByMediaID[icon.String] = append(idx.refsByMediaID[icon.String], UsageRef{
			SourceKind: "settings", SourceID: "site_icon", SourceLabel: "Settings", Context: "Site Icon", Public: true, EditURL: "/admin/settings/general",
		})
	}
	if logo.Valid && logo.String != "" {
		idx.refsByMediaID[logo.String] = append(idx.refsByMediaID[logo.String], UsageRef{
			SourceKind: "settings", SourceID: "site_logo", SourceLabel: "Settings", Context: "Site Logo", Public: true, EditURL: "/admin/settings/general",
		})
	}
	if social.Valid && social.String != "" {
		idx.refsByMediaID[social.String] = append(idx.refsByMediaID[social.String], UsageRef{
			SourceKind: "settings", SourceID: "site_social", SourceLabel: "Settings", Context: "Social Image", Public: true, EditURL: "/admin/settings/seo",
		})
	}
	return nil
}

// perMediaState tracks whether a media appears in published and/or draft for a single source.
type perMediaState struct {
	foundPublished bool
	foundDraft     bool
	ctxPublished   string
	ctxDraft       string
}

type entryRev struct {
	ID              string
	EntryID         string
	Title           string
	DocumentJSON    string
	FeaturedMediaID sql.NullString
	SocialMediaID   sql.NullString
	FieldsJSON      string
}

func (s *Service) scanEntries(ctx context.Context, idx *UsageIndex, defs map[string]content.ContentTypeDefinition) error {
	// Load all active entries
	rows, err := s.db.QueryContext(ctx, `SELECT id, content_type_id, published_revision_id FROM entries WHERE status='active'`)
	if err != nil {
		return fmt.Errorf("load entries: %w", err)
	}
	defer rows.Close()
	type eInfo struct {
		id    string
		ct    string
		pubID sql.NullString
	}
	var entries []eInfo
	for rows.Next() {
		var id, ct string
		var pub sql.NullString
		if err := rows.Scan(&id, &ct, &pub); err != nil {
			return fmt.Errorf("scan entry: %w", err)
		}
		entries = append(entries, eInfo{id: id, ct: ct, pubID: pub})
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("entries rows: %w", err)
	}
	if len(entries) == 0 {
		return nil
	}

	// Fetch published revisions
	pubMap := make(map[string]*entryRev) // entryID -> rev
	rowsPub, err := s.db.QueryContext(ctx, `SELECT r.id, r.entry_id, r.title, r.document_json, r.featured_media_id, r.social_media_id, r.fields_json FROM entry_revisions r WHERE r.id IN (SELECT published_revision_id FROM entries WHERE status='active' AND published_revision_id IS NOT NULL)`)
	if err != nil {
		return fmt.Errorf("load published revisions: %w", err)
	}
	for rowsPub.Next() {
		var rev entryRev
		var title, docJSON, fieldsJSON string
		var feat, social sql.NullString
		var id, entryID string
		if err := rowsPub.Scan(&id, &entryID, &title, &docJSON, &feat, &social, &fieldsJSON); err != nil {
			rowsPub.Close()
			return fmt.Errorf("scan published rev: %w", err)
		}
		rev = entryRev{ID: id, EntryID: entryID, Title: title, DocumentJSON: docJSON, FeaturedMediaID: feat, SocialMediaID: social, FieldsJSON: fieldsJSON}
		// copy to heap
		c := rev
		pubMap[entryID] = &c
	}
	rowsPub.Close()
	if err := rowsPub.Err(); err != nil {
		return fmt.Errorf("published rows: %w", err)
	}

	// Fetch latest revisions (one per active entry)
	latestMap := make(map[string]*entryRev)
	rowsLatest, err := s.db.QueryContext(ctx, `SELECT r.id, r.entry_id, r.title, r.document_json, r.featured_media_id, r.social_media_id, r.fields_json FROM entry_revisions r INNER JOIN (SELECT entry_id, MAX(revision_number) AS max_num FROM entry_revisions GROUP BY entry_id) m ON m.entry_id = r.entry_id AND m.max_num = r.revision_number WHERE r.entry_id IN (SELECT id FROM entries WHERE status='active')`)
	if err != nil {
		return fmt.Errorf("load latest revisions: %w", err)
	}
	for rowsLatest.Next() {
		var rev entryRev
		var title, docJSON, fieldsJSON string
		var feat, social sql.NullString
		var id, entryID string
		if err := rowsLatest.Scan(&id, &entryID, &title, &docJSON, &feat, &social, &fieldsJSON); err != nil {
			rowsLatest.Close()
			return fmt.Errorf("scan latest rev: %w", err)
		}
		rev = entryRev{ID: id, EntryID: entryID, Title: title, DocumentJSON: docJSON, FeaturedMediaID: feat, SocialMediaID: social, FieldsJSON: fieldsJSON}
		c := rev
		latestMap[entryID] = &c
	}
	rowsLatest.Close()
	if err := rowsLatest.Err(); err != nil {
		return fmt.Errorf("latest rows: %w", err)
	}

	// Process each entry
	for _, e := range entries {
		pubRev := pubMap[e.id]
		latestRev := latestMap[e.id]

		// Deduplicate same revision
		isSame := pubRev != nil && latestRev != nil && pubRev.ID == latestRev.ID

		perMedia := make(map[string]*perMediaState)

		// Helper to process a revision as published or draft
		process := func(rev *entryRev, isPublished bool) error {
			if rev == nil {
				return nil
			}
			// Featured
			if rev.FeaturedMediaID.Valid && rev.FeaturedMediaID.String != "" {
				mid := rev.FeaturedMediaID.String
				st := perMedia[mid]
				if st == nil {
					st = &perMediaState{}
					perMedia[mid] = st
				}
				if isPublished {
					st.foundPublished = true
					if st.ctxPublished == "" {
						st.ctxPublished = "Featured image"
					}
				} else {
					st.foundDraft = true
					if st.ctxDraft == "" {
						st.ctxDraft = "Featured image"
					}
				}
			}
			if rev.SocialMediaID.Valid && rev.SocialMediaID.String != "" {
				mid := rev.SocialMediaID.String
				st := perMedia[mid]
				if st == nil {
					st = &perMediaState{}
					perMedia[mid] = st
				}
				if isPublished {
					st.foundPublished = true
					if st.ctxPublished == "" {
						st.ctxPublished = "Social image"
					}
				} else {
					st.foundDraft = true
					if st.ctxDraft == "" {
						st.ctxDraft = "Social image"
					}
				}
			}
			// Fields
			if rev.FieldsJSON != "" && rev.FieldsJSON != "{}" {
				m, err := mediaIDsFromFields(rev.FieldsJSON, e.ct, defs)
				if err != nil {
					return fmt.Errorf("fields %s: %w", e.id, err)
				}
				for mid, ctxLabel := range m {
					st := perMedia[mid]
					if st == nil {
						st = &perMediaState{}
						perMedia[mid] = st
					}
					if isPublished {
						st.foundPublished = true
						if st.ctxPublished == "" {
							st.ctxPublished = ctxLabel
						}
					} else {
						st.foundDraft = true
						if st.ctxDraft == "" {
							st.ctxDraft = ctxLabel
						}
					}
				}
			}
			// Document
			if rev.DocumentJSON != "" {
				m, err := mediaIDsFromDocument(rev.DocumentJSON)
				if err != nil {
					return fmt.Errorf("document %s: %w", e.id, err)
				}
				for mid, ctxLabel := range m {
					st := perMedia[mid]
					if st == nil {
						st = &perMediaState{}
						perMedia[mid] = st
					}
					if isPublished {
						st.foundPublished = true
						st.ctxPublished = ctxLabel
					} else {
						st.foundDraft = true
						st.ctxDraft = ctxLabel
					}
				}
			}
			return nil
		}

		if pubRev != nil {
			if err := process(pubRev, true); err != nil {
				return err
			}
		}
		if latestRev != nil && !isSame {
			if err := process(latestRev, false); err != nil {
				return err
			}
		}
		// If draft-only entry (no published), latest already processed as draft.
		// If entry has published == latest, we already processed as published only.

		// Emit one UsageRef per media per entry
		for mid, st := range perMedia {
			label := ""
			if latestRev != nil && latestRev.Title != "" {
				label = latestRev.Title
			} else if pubRev != nil && pubRev.Title != "" {
				label = pubRev.Title
			} else {
				label = e.id
			}
			ctxLabel := st.ctxPublished
			if !st.foundPublished {
				ctxLabel = st.ctxDraft
			}
			if ctxLabel == "" {
				ctxLabel = "Content"
			}
			ref := UsageRef{
				SourceKind:  "entry",
				SourceID:    e.id,
				SourceLabel: label,
				Context:     ctxLabel,
				Public:      st.foundPublished,
				EditURL:     entryEditURL(e.ct, e.id),
			}
			idx.refsByMediaID[mid] = append(idx.refsByMediaID[mid], ref)
		}
	}

	return nil
}

func (s *Service) scanTemplates(ctx context.Context, idx *UsageIndex) error {
	// Load templates
	rows, err := s.db.QueryContext(ctx, `SELECT id, name, published_revision_id FROM layout_templates`)
	if err != nil {
		// If table missing, ignore?
		return fmt.Errorf("load templates: %w", err)
	}
	defer rows.Close()
	type tInfo struct {
		id    string
		name  string
		pubID sql.NullString
	}
	var templates []tInfo
	for rows.Next() {
		var id, name string
		var pub sql.NullString
		if err := rows.Scan(&id, &name, &pub); err != nil {
			return fmt.Errorf("scan template: %w", err)
		}
		templates = append(templates, tInfo{id: id, name: name, pubID: pub})
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("templates rows: %w", err)
	}
	if len(templates) == 0 {
		return nil
	}

	// Published revisions
	pubMap := make(map[string]string) // templateID -> docJSON
	rowsPub, err := s.db.QueryContext(ctx, `SELECT r.template_id, r.document_json FROM layout_template_revisions r WHERE r.id IN (SELECT published_revision_id FROM layout_templates WHERE published_revision_id IS NOT NULL)`)
	if err != nil {
		return fmt.Errorf("load published template revs: %w", err)
	}
	for rowsPub.Next() {
		var tid, doc string
		if err := rowsPub.Scan(&tid, &doc); err != nil {
			rowsPub.Close()
			return fmt.Errorf("scan published template rev: %w", err)
		}
		pubMap[tid] = doc
	}
	rowsPub.Close()
	if err := rowsPub.Err(); err != nil {
		return fmt.Errorf("published template rows: %w", err)
	}

	// Latest revisions
	latestMap := make(map[string]string)   // templateID -> docJSON
	latestIDMap := make(map[string]string) // templateID -> id
	rowsLatest, err := s.db.QueryContext(ctx, `SELECT r.template_id, r.document_json, r.id FROM layout_template_revisions r INNER JOIN (SELECT template_id, MAX(revision_number) AS max_num FROM layout_template_revisions GROUP BY template_id) m ON m.template_id = r.template_id AND m.max_num = r.revision_number`)
	if err != nil {
		return fmt.Errorf("load latest template revs: %w", err)
	}
	for rowsLatest.Next() {
		var tid, doc, id string
		if err := rowsLatest.Scan(&tid, &doc, &id); err != nil {
			rowsLatest.Close()
			return fmt.Errorf("scan latest template rev: %w", err)
		}
		latestMap[tid] = doc
		latestIDMap[tid] = id
	}
	rowsLatest.Close()
	if err := rowsLatest.Err(); err != nil {
		return fmt.Errorf("latest template rows: %w", err)
	}
	// Need published ID map for dedup
	pubIDMap := make(map[string]string)
	for _, t := range templates {
		if t.pubID.Valid {
			pubIDMap[t.id] = t.pubID.String
		}
	}

	for _, t := range templates {
		pubDoc, hasPub := pubMap[t.id]
		latestDoc, hasLatest := latestMap[t.id]
		pubID := pubIDMap[t.id]
		latestID := latestIDMap[t.id]
		isSame := hasPub && hasLatest && pubID != "" && latestID != "" && pubID == latestID

		perMedia := make(map[string]*perMediaState)

		processDoc := func(docJSON string, isPublished bool) error {
			if docJSON == "" {
				return nil
			}
			m, err := mediaIDsFromDocument(docJSON)
			if err != nil {
				return fmt.Errorf("template %s document: %w", t.id, err)
			}
			for mid, ctxLabel := range m {
				st := perMedia[mid]
				if st == nil {
					st = &perMediaState{}
					perMedia[mid] = st
				}
				if isPublished {
					st.foundPublished = true
					st.ctxPublished = ctxLabel
				} else {
					st.foundDraft = true
					st.ctxDraft = ctxLabel
				}
			}
			return nil
		}

		if hasPub {
			if err := processDoc(pubDoc, true); err != nil {
				return err
			}
		}
		if hasLatest && !isSame {
			if err := processDoc(latestDoc, false); err != nil {
				return err
			}
		}

		for mid, st := range perMedia {
			ctxLabel := st.ctxPublished
			if !st.foundPublished {
				ctxLabel = st.ctxDraft
			}
			if ctxLabel == "" {
				ctxLabel = "Content"
			}
			ref := UsageRef{
				SourceKind:  "template",
				SourceID:    t.id,
				SourceLabel: t.name,
				Context:     ctxLabel,
				Public:      st.foundPublished,
				EditURL:     "/admin/appearance/templates/" + t.id + "/edit",
			}
			idx.refsByMediaID[mid] = append(idx.refsByMediaID[mid], ref)
		}
	}
	return nil
}

func (s *Service) scanSiteParts(ctx context.Context, idx *UsageIndex) error {
	rows, err := s.db.QueryContext(ctx, `SELECT id, name, published_revision_id FROM site_parts`)
	if err != nil {
		return fmt.Errorf("load site parts: %w", err)
	}
	defer rows.Close()
	type pInfo struct {
		id    string
		name  string
		pubID sql.NullString
	}
	var parts []pInfo
	for rows.Next() {
		var id, name string
		var pub sql.NullString
		if err := rows.Scan(&id, &name, &pub); err != nil {
			return fmt.Errorf("scan site part: %w", err)
		}
		parts = append(parts, pInfo{id: id, name: name, pubID: pub})
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("site parts rows: %w", err)
	}
	if len(parts) == 0 {
		return nil
	}

	pubMap := make(map[string]string)
	rowsPub, err := s.db.QueryContext(ctx, `SELECT r.site_part_id, r.document_json FROM site_part_revisions r WHERE r.id IN (SELECT published_revision_id FROM site_parts WHERE published_revision_id IS NOT NULL)`)
	if err != nil {
		return fmt.Errorf("load published site part revs: %w", err)
	}
	for rowsPub.Next() {
		var sid, doc string
		if err := rowsPub.Scan(&sid, &doc); err != nil {
			rowsPub.Close()
			return fmt.Errorf("scan published site part rev: %w", err)
		}
		pubMap[sid] = doc
	}
	rowsPub.Close()
	if err := rowsPub.Err(); err != nil {
		return fmt.Errorf("published site part rows: %w", err)
	}

	latestMap := make(map[string]string)
	latestIDMap := make(map[string]string)
	rowsLatest, err := s.db.QueryContext(ctx, `SELECT r.site_part_id, r.document_json, r.id FROM site_part_revisions r INNER JOIN (SELECT site_part_id, MAX(revision_number) AS max_num FROM site_part_revisions GROUP BY site_part_id) m ON m.site_part_id = r.site_part_id AND m.max_num = r.revision_number`)
	if err != nil {
		return fmt.Errorf("load latest site part revs: %w", err)
	}
	for rowsLatest.Next() {
		var sid, doc, id string
		if err := rowsLatest.Scan(&sid, &doc, &id); err != nil {
			rowsLatest.Close()
			return fmt.Errorf("scan latest site part rev: %w", err)
		}
		latestMap[sid] = doc
		latestIDMap[sid] = id
	}
	rowsLatest.Close()
	if err := rowsLatest.Err(); err != nil {
		return fmt.Errorf("latest site part rows: %w", err)
	}

	pubIDMap := make(map[string]string)
	for _, p := range parts {
		if p.pubID.Valid {
			pubIDMap[p.id] = p.pubID.String
		}
	}

	for _, p := range parts {
		pubDoc, hasPub := pubMap[p.id]
		latestDoc, hasLatest := latestMap[p.id]
		pubID := pubIDMap[p.id]
		latestID := latestIDMap[p.id]
		isSame := hasPub && hasLatest && pubID != "" && latestID != "" && pubID == latestID

		perMedia := make(map[string]*perMediaState)
		processDoc := func(docJSON string, isPublished bool) error {
			if docJSON == "" {
				return nil
			}
			m, err := mediaIDsFromDocument(docJSON)
			if err != nil {
				return fmt.Errorf("site part %s document: %w", p.id, err)
			}
			for mid, ctxLabel := range m {
				st := perMedia[mid]
				if st == nil {
					st = &perMediaState{}
					perMedia[mid] = st
				}
				if isPublished {
					st.foundPublished = true
					st.ctxPublished = ctxLabel
				} else {
					st.foundDraft = true
					st.ctxDraft = ctxLabel
				}
			}
			return nil
		}
		if hasPub {
			if err := processDoc(pubDoc, true); err != nil {
				return err
			}
		}
		if hasLatest && !isSame {
			if err := processDoc(latestDoc, false); err != nil {
				return err
			}
		}
		for mid, st := range perMedia {
			ctxLabel := st.ctxPublished
			if !st.foundPublished {
				ctxLabel = st.ctxDraft
			}
			if ctxLabel == "" {
				ctxLabel = "Content"
			}
			ref := UsageRef{
				SourceKind:  "sitePart",
				SourceID:    p.id,
				SourceLabel: p.name,
				Context:     ctxLabel,
				Public:      st.foundPublished,
				EditURL:     "/admin/appearance/site-parts/" + p.id + "/edit",
			}
			idx.refsByMediaID[mid] = append(idx.refsByMediaID[mid], ref)
		}
	}
	return nil
}

// Usage returns all usage refs for a media ID. It distinguishes published vs draft where practical.
func (s *Service) Usage(ctx context.Context, id string) ([]UsageRef, error) {
	return s.UsageRefs(ctx, id)
}

// UsageRefs is the canonical scanner for media references. It builds a single
// UsageIndex snapshot and returns refs for one media ID.
// On scanner error it returns the error (fail-safe: caller must not assume unused).
func (s *Service) UsageRefs(ctx context.Context, id string) ([]UsageRef, error) {
	if id == "" {
		return nil, nil
	}
	idx, err := s.BuildUsageIndex(ctx)
	if err != nil {
		return nil, err
	}
	return idx.Refs(id), nil
}

// CountUsageStructured is a helper for domain checks (uses the canonical index).
func (s *Service) CountUsageStructured(ctx context.Context, id string) (int64, error) {
	refs, err := s.UsageRefs(ctx, id)
	if err != nil {
		return 0, err
	}
	return int64(len(refs)), nil
}

// entryEditURL helper
func entryEditURL(contentTypeID, entryID string) string {
	switch contentTypeID {
	case "page":
		return "/admin/pages/" + entryID + "/edit"
	case "post":
		return "/admin/posts/" + entryID + "/edit"
	case "":
		return "/admin/pages/" + entryID + "/edit"
	default:
		return "/admin/content/" + contentTypeID + "/" + entryID + "/edit"
	}
}

// --- Structured SDT scanning ---

func mediaIDsFromDocument(docJSON string) (map[string]string, error) {
	if strings.TrimSpace(docJSON) == "" {
		return map[string]string{}, nil
	}
	doc, err := document.Decode([]byte(docJSON))
	if err != nil {
		return nil, fmt.Errorf("decode document: %w", err)
	}
	if doc == nil {
		return map[string]string{}, nil
	}
	return mediaIDsFromDoc(doc), nil
}

func mediaIDsFromDoc(doc *document.Document) map[string]string {
	out := make(map[string]string)
	var walk func([]document.Node)
	walk = func(nodes []document.Node) {
		for _, n := range nodes {
			switch n.Block {
			case "core/image":
				var props map[string]any
				if len(n.Props) > 0 {
					_ = json.Unmarshal(n.Props, &props)
				}
				if v, ok := props["mediaId"].(string); ok && strings.TrimSpace(v) != "" {
					if _, exists := out[v]; !exists {
						out[v] = "Image block"
					}
				}
			case "core/gallery":
				var props map[string]any
				if len(n.Props) > 0 {
					_ = json.Unmarshal(n.Props, &props)
				}
				if v, ok := props["images"]; ok {
					switch val := v.(type) {
					case []any:
						for _, item := range val {
							if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
								if _, exists := out[s]; !exists {
									out[s] = "Gallery block"
								}
							}
						}
					case string:
						parts := strings.Split(val, ",")
						for _, p := range parts {
							p = strings.TrimSpace(p)
							if p != "" {
								if _, exists := out[p]; !exists {
									out[p] = "Gallery block"
								}
							}
						}
					case []string:
						for _, s := range val {
							if strings.TrimSpace(s) != "" {
								if _, exists := out[s]; !exists {
									out[s] = "Gallery block"
								}
							}
						}
					}
				}
			default:
				// No generic heuristic. Only known media blocks are considered.
				// Other blocks (e.g. core/site-logo) do not store explicit media IDs
				// in props; they reference site settings indirectly.
			}
			if len(n.Children) > 0 {
				walk(n.Children)
			}
		}
	}
	walk(doc.Nodes)
	return out
}

func mediaIDsFromFields(fieldsJSON, contentTypeID string, defs map[string]content.ContentTypeDefinition) (map[string]string, error) {
	if strings.TrimSpace(fieldsJSON) == "" || strings.TrimSpace(fieldsJSON) == "{}" {
		return map[string]string{}, nil
	}
	fields, err := content.DecodeFieldSnapshot(fieldsJSON)
	if err != nil {
		return nil, err
	}
	def, ok := defs[contentTypeID]
	if !ok {
		// No definition – cannot determine typed media fields. Return no media refs
		// rather than guessing via generic string equality.
		return map[string]string{}, nil
	}
	mediaFieldLabels := make(map[string]string)
	for _, f := range def.Fields {
		if f.Type == content.FieldMedia {
			mediaFieldLabels[f.Key] = f.Label
		}
	}
	if len(mediaFieldLabels) == 0 {
		return map[string]string{}, nil
	}
	out := make(map[string]string)
	for key, rawVal := range fields {
		label, isMedia := mediaFieldLabels[key]
		if !isMedia {
			continue
		}
		ctxLabel := "Custom media field"
		if strings.TrimSpace(label) != "" {
			ctxLabel = label
		}
		switch v := rawVal.(type) {
		case string:
			if strings.TrimSpace(v) != "" {
				out[v] = ctxLabel
			}
		case []any:
			for _, item := range v {
				if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
					out[s] = ctxLabel
				}
			}
		case []string:
			for _, s := range v {
				if strings.TrimSpace(s) != "" {
					out[s] = ctxLabel
				}
			}
		default:
			// Media field should be string; ignore other types
		}
	}
	return out, nil
}
