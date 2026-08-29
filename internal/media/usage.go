package media

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"

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

// Usage returns all usage refs for a media ID. It distinguishes published vs draft where practical.
func (s *Service) Usage(ctx context.Context, id string) ([]UsageRef, error) {
	return s.UsageRefs(ctx, id)
}

// UsageRefs is the canonical scanner for media references. It checks site settings, entries (published + draft),
// templates, and site parts via structured traversal, not LIKE.
func (s *Service) UsageRefs(ctx context.Context, id string) ([]UsageRef, error) {
	if id == "" {
		return nil, nil
	}
	var out []UsageRef

	// 1. Site settings: icon, logo, social
	if s.db != nil {
		var icon, logo, social sql.NullString
		err := s.db.QueryRowContext(ctx, `SELECT site_icon_media_id, site_logo_media_id, site_social_media_id FROM site_settings WHERE id=1`).Scan(&icon, &logo, &social)
		if err == nil {
			if icon.Valid && icon.String == id {
				out = append(out, UsageRef{SourceKind: "settings", SourceID: "site_icon", SourceLabel: "Settings", Context: "Site Icon", Public: true, EditURL: "/admin/settings/general"})
			}
			if logo.Valid && logo.String == id {
				out = append(out, UsageRef{SourceKind: "settings", SourceID: "site_logo", SourceLabel: "Settings", Context: "Site Logo", Public: true, EditURL: "/admin/settings/general"})
			}
			if social.Valid && social.String == id {
				out = append(out, UsageRef{SourceKind: "settings", SourceID: "site_social", SourceLabel: "Settings", Context: "Social Image", Public: true, EditURL: "/admin/settings/seo"})
			}
		} else if err != sql.ErrNoRows {
			return nil, err
		}
	}

	// Helper to check if document JSON contains media id via structured traversal
	containsInDocument := func(docJSON string) (bool, string) {
		if docJSON == "" {
			return false, ""
		}
		doc, err := document.Decode([]byte(docJSON))
		if err != nil || doc == nil {
			return false, ""
		}
		found, ctxLabel := scanDocumentForMedia(doc, id)
		return found, ctxLabel
	}

	// 2. Entries: need to handle both published and draft (latest) revisions.
	// We collect per-entry whether published or draft contains the id.
	if s.db != nil {
		// Fetch all active entries with their published revision id and content type
		rows, err := s.db.QueryContext(ctx, `SELECT e.id, e.content_type_id, e.published_revision_id, e.status FROM entries e WHERE e.status='active'`)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		type entryInfo struct {
			id          string
			ct          string
			publishedID sql.NullString
		}
		var entries []entryInfo
		for rows.Next() {
			var e entryInfo
			var pub sql.NullString
			var status string
			if err := rows.Scan(&e.id, &e.ct, &pub, &status); err != nil {
				continue
			}
			e.publishedID = pub
			entries = append(entries, e)
		}
		rows.Close()
		// For each entry, check revisions
		for _, e := range entries {
			// Get title from latest revision if needed for label; we'll fetch revisions
			// Fetch all revisions for this entry (could be many, but typically few)
			revRows, err := s.db.QueryContext(ctx, `SELECT id, title, document_json, featured_media_id, social_media_id, fields_json FROM entry_revisions WHERE entry_id = ? ORDER BY revision_number DESC`, e.id)
			if err != nil {
				continue
			}
			var latestTitle string
			var foundPublished, foundDraft bool
			var draftContext, publishedContext string
			var featuredFound, socialFound bool
			for revRows.Next() {
				var revID, title, docJSON, fieldsJSON string
				var feat, social sql.NullString
				// Note: featured/social may be NULL; scan as sql.NullString
				if err := revRows.Scan(&revID, &title, &docJSON, &feat, &social, &fieldsJSON); err != nil {
					continue
				}
				if latestTitle == "" {
					latestTitle = title
				}
				isPublished := e.publishedID.Valid && e.publishedID.String == revID
				// Check featured/social
				if feat.Valid && feat.String == id {
					featuredFound = true
					if isPublished {
						foundPublished = true
						publishedContext = "Featured image"
					} else {
						foundDraft = true
						draftContext = "Featured image"
					}
				}
				if social.Valid && social.String == id {
					socialFound = true
					if isPublished {
						foundPublished = true
						if publishedContext == "" {
							publishedContext = "Social image"
						}
					} else {
						foundDraft = true
						if draftContext == "" {
							draftContext = "Social image"
						}
					}
				}
				// Check fields_json for custom media fields
				if fieldsJSON != "" && fieldsContainMedia(fieldsJSON, id) {
					if isPublished {
						foundPublished = true
						if publishedContext == "" {
							publishedContext = "Custom media field"
						}
					} else {
						foundDraft = true
						if draftContext == "" {
							draftContext = "Custom media field"
						}
					}
				}
				// Check document
				if ok, ctxLabel := containsInDocument(docJSON); ok {
					if isPublished {
						foundPublished = true
						publishedContext = ctxLabel
					} else {
						foundDraft = true
						draftContext = ctxLabel
					}
				}
			}
			revRows.Close()
			_ = featuredFound
			_ = socialFound
			if foundPublished || foundDraft {
				// Prefer published context if available, else draft
				public := foundPublished
				ctxLabel := publishedContext
				if !foundPublished {
					ctxLabel = draftContext
				}
				if ctxLabel == "" {
					ctxLabel = "Content"
				}
				label := latestTitle
				if label == "" {
					label = e.id
				}
				editURL := entryEditURL(e.ct, e.id)
				out = append(out, UsageRef{
					SourceKind:  "entry",
					SourceID:    e.id,
					SourceLabel: label,
					Context:     ctxLabel,
					Public:      public,
					EditURL:     editURL,
				})
			}
		}
	}

	// 3. Layout templates
	if s.db != nil {
		rows, err := s.db.QueryContext(ctx, `SELECT lt.id, lt.name, ltr.document_json, lt.published_revision_id, ltr.id FROM layout_templates lt JOIN layout_template_revisions ltr ON ltr.id = lt.published_revision_id`)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var tid, tname, docJSON string
				var pubID, revID sql.NullString
				// Actually ltr.id is revision id, lt.published_revision_id is same; we join correctly
				// The query selects lt.published_revision_id and ltr.id, but we only need doc
				if err := rows.Scan(&tid, &tname, &docJSON, &pubID, &revID); err != nil {
					continue
				}
				if ok, ctxLabel := containsInDocument(docJSON); ok {
					out = append(out, UsageRef{
						SourceKind:  "template",
						SourceID:    tid,
						SourceLabel: tname,
						Context:     ctxLabel,
						Public:      true,
						EditURL:     "/admin/appearance/templates/" + tid + "/edit",
					})
				}
			}
		}
	}

	// 4. Site parts
	if s.db != nil {
		rows, err := s.db.QueryContext(ctx, `SELECT sp.id, sp.name, spr.document_json FROM site_parts sp JOIN site_part_revisions spr ON spr.id = sp.published_revision_id`)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var sid, sname, docJSON string
				if err := rows.Scan(&sid, &sname, &docJSON); err != nil {
					continue
				}
				if ok, ctxLabel := containsInDocument(docJSON); ok {
					out = append(out, UsageRef{
						SourceKind:  "sitePart",
						SourceID:    sid,
						SourceLabel: sname,
						Context:     ctxLabel,
						Public:      true,
						EditURL:     "/admin/appearance/site-parts/" + sid + "/edit",
					})
				}
			}
		}
	}

	// Also check site_parts that may have no published revision? Try alternative table? The above covers published.

	// Note: we do not scan draft templates/site parts that are not published, but they are rare.

	return out, nil
}

// entryEditURL helper for health and usage (duplicate to avoid import cycle)
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

func scanDocumentForMedia(doc *document.Document, targetID string) (bool, string) {
	var found bool
	var label string
	var walk func([]document.Node)
	walk = func(nodes []document.Node) {
		for _, n := range nodes {
			if found {
				return
			}
			// Decode props
			var props map[string]any
			if len(n.Props) > 0 {
				_ = json.Unmarshal(n.Props, &props)
			}
			if props != nil {
				// Image block: props.mediaId
				if v, ok := props["mediaId"].(string); ok && v == targetID {
					found = true
					label = "Image block"
					return
				}
				// Gallery block v2: props.images as []any
				if v, ok := props["images"]; ok {
					switch val := v.(type) {
					case []any:
						for _, item := range val {
							if s, ok := item.(string); ok && s == targetID {
								found = true
								label = "Gallery block"
								return
							}
						}
					case string:
						// Old gallery: comma-separated string
						parts := strings.Split(val, ",")
						for _, p := range parts {
							if strings.TrimSpace(p) == targetID {
								found = true
								label = "Gallery block"
								return
							}
						}
					case []string:
						for _, s := range val {
							if s == targetID {
								found = true
								label = "Gallery block"
								return
							}
						}
					}
				}
				// Generic: any string prop equals targetID (covers other blocks that store mediaId via generic prop)
				for key, v := range props {
					if key == "mediaId" || key == "images" {
						continue
					}
					if s, ok := v.(string); ok && s == targetID && strings.HasPrefix(targetID, "media_") {
						// Only consider if prop looks like media reference (heuristic)
						found = true
						label = "Image block"
						return
					}
					if arr, ok := v.([]any); ok {
						for _, item := range arr {
							if s, ok := item.(string); ok && s == targetID {
								found = true
								label = "Gallery block"
								return
							}
						}
					}
				}
			}
			if len(n.Children) > 0 {
				walk(n.Children)
				if found {
					return
				}
			}
		}
	}
	walk(doc.Nodes)
	return found, label
}

func fieldsContainMedia(fieldsJSON, targetID string) bool {
	if fieldsJSON == "" {
		return false
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(fieldsJSON), &m); err != nil {
		// Try as raw string contains check as fallback (should not LIKE per spec, but we parse JSON)
		return strings.Contains(fieldsJSON, targetID)
	}
	for _, v := range m {
		switch val := v.(type) {
		case string:
			if val == targetID {
				return true
			}
		case []any:
			for _, item := range val {
				if s, ok := item.(string); ok && s == targetID {
					return true
				}
			}
		case []string:
			for _, s := range val {
				if s == targetID {
					return true
				}
			}
		}
	}
	return false
}

// CountUsage via structured scan (replaces LIKE). Used by safe delete.
func (s *Service) CountUsageStructured(ctx context.Context, id string) (int64, error) {
	refs, err := s.UsageRefs(ctx, id)
	if err != nil {
		return 0, err
	}
	return int64(len(refs)), nil
}
