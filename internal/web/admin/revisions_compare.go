package admin

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/kokosx/stratum/internal/content"
	"github.com/kokosx/stratum/internal/document"
	"github.com/kokosx/stratum/internal/revisions"
	db "github.com/kokosx/stratum/internal/storage/sqlc"
)

type compareData struct {
	Heading    string
	BackURL    string
	EntityName string
	EntityKind string
	Revisions  []revisionHistoryRow
	Diff       *revisions.Diff
	CompareA   string
	CompareB   string
	CSRFToken  string
	Flash      string
	Warnings   []string
}

func (h *Handler) handleCompareRevisions(w http.ResponseWriter, r *http.Request, contentType, entryID string) {
	entry, err := h.queries.GetEntry(r.Context(), entryID)
	if err != nil || entry.ContentTypeID != contentType {
		http.NotFound(w, r)
		return
	}
	allRevs, err := h.queries.ListEntryRevisions(r.Context(), entryID)
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	rows := make([]revisionHistoryRow, 0, len(allRevs))
	for _, rev := range allRevs {
		status := ""
		if entry.PublishedRevisionID.Valid && entry.PublishedRevisionID.String == rev.ID {
			status = "Published"
		} else if len(allRevs) > 0 && rev.ID == allRevs[0].ID {
			status = "Current draft"
		}
		author := ""
		if rev.CreatedBy.Valid {
			author = rev.CreatedBy.String
		}
		rows = append(rows, revisionHistoryRow{
			ID:        rev.ID,
			Number:    rev.RevisionNumber,
			CreatedAt: formatRevisionTime(rev.CreatedAt),
			Author:    author,
			Status:    status,
		})
	}
	aID := strings.TrimSpace(r.URL.Query().Get("a"))
	bID := strings.TrimSpace(r.URL.Query().Get("b"))
	if aID == "" && bID == "" && len(allRevs) >= 2 {
		aID = allRevs[1].ID
		bID = allRevs[0].ID
	}
	var diff *revisions.Diff
	var warnings []string
	if aID != "" && bID != "" {
		revA, errA := h.queries.GetEntryRevision(r.Context(), aID)
		revB, errB := h.queries.GetEntryRevision(r.Context(), bID)
		if errA == nil && errB == nil && revA.EntryID == entryID && revB.EntryID == entryID {
			fieldSchemas := make(map[string]revisions.FieldSchema)
			if def, err := content.NewCatalog(h.queries).GetDefinition(r.Context(), entry.ContentTypeID); err == nil {
				for _, f := range def.Fields {
					fieldSchemas[f.Key] = revisions.FieldSchema{Label: f.Label, Type: string(f.Type)}
				}
			}
			opts := revisions.CompareOptions{
				ContentTypeID: entry.ContentTypeID,
				FieldSchemas:  fieldSchemas,
				BlockRegistry: h.blocks,
			}
			d, err := revisions.CompareRevisions(revA, revB, opts)
			if err != nil {
				log.Printf("compare revisions: %v", err)
			} else {
				diff = d
				// Check for missing media in either revision
				if miss := h.collectMissingMedia(r, revA, revB); len(miss) > 0 {
					warnings = append(warnings, fmt.Sprintf("This revision references %d media items that no longer exist.", len(miss)))
				}
			}
		}
	}
	token, _ := h.csrfToken(w, r)
	heading := "Compare revisions"
	entityName := entry.Slug
	if rev, err := h.queries.GetLatestEntryRevision(r.Context(), entryID); err == nil && rev.Title != "" {
		entityName = rev.Title
	}
	backURL := "/admin/" + contentTypeToAdminPath(contentType) + "/" + entryID + "/edit"
	kind := "Page"
	if contentType == "post" {
		kind = "Post"
	} else if contentType != "page" {
		kind = strings.Title(contentType)
	}
	data := compareData{
		Heading:    heading,
		BackURL:    backURL,
		EntityName: entityName,
		EntityKind: kind,
		Revisions:  rows,
		Diff:       diff,
		CompareA:   aID,
		CompareB:   bID,
		CSRFToken:  token,
		Flash:      h.consumeFlash(w, r),
		Warnings:   warnings,
	}
	w.Header().Set("X-Robots-Tag", "noindex, nofollow")
	w.Header().Set("Cache-Control", "no-store")
	if err := h.revisionsCompareTemplate.ExecuteTemplate(w, "layout.html", LayoutData{
		Title:         data.Heading + " — " + data.EntityName,
		ActiveMenu:    ResolveNav(r.URL.Path).ActiveSection,
		ActiveSection: ResolveNav(r.URL.Path).ActiveSection,
		ActiveItem:    ResolveNav(r.URL.Path).ActiveItem,
		Nav:           h.navForUser(r),
		CSRFToken:     token,
		Flash:         data.Flash,
		Content:       data,
	}); err != nil {
		log.Printf("render compare: %v", err)
	}
}

func (h *Handler) collectMissingMedia(r *http.Request, a, b db.EntryRevision) []string {
	mediaIDs := make(map[string]bool)
	for _, rev := range []db.EntryRevision{a, b} {
		doc, err := document.Decode([]byte(rev.DocumentJson))
		if err != nil || doc == nil {
			continue
		}
		var walk func(nodes []document.Node)
		walk = func(nodes []document.Node) {
			for _, n := range nodes {
				var props map[string]any
				if len(n.Props) > 0 {
					_ = json.Unmarshal(n.Props, &props)
				}
				switch n.Block {
				case "core/image":
					if props != nil {
						if mid, ok := props["mediaId"].(string); ok && mid != "" {
							mediaIDs[mid] = true
						}
					}
				case "core/gallery":
					if props != nil {
						if ids, ok := props["mediaIds"].([]any); ok {
							for _, v := range ids {
								if mid, ok := v.(string); ok && mid != "" {
									mediaIDs[mid] = true
								}
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
		if rev.FeaturedMediaID.Valid && rev.FeaturedMediaID.String != "" {
			mediaIDs[rev.FeaturedMediaID.String] = true
		}
		if rev.SocialMediaID.Valid && rev.SocialMediaID.String != "" {
			mediaIDs[rev.SocialMediaID.String] = true
		}
	}
	var missing []string
	for mid := range mediaIDs {
		if _, err := h.queries.GetMedia(r.Context(), mid); err != nil {
			missing = append(missing, mid)
		}
	}
	return missing
}
