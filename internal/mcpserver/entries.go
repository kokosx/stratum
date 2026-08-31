package mcpserver

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kokosx/stratum/internal/authz"
	"github.com/kokosx/stratum/internal/content"
	"github.com/kokosx/stratum/internal/document"
	"github.com/kokosx/stratum/internal/entryops"
	db "github.com/kokosx/stratum/internal/storage/sqlc"
)

func (s *Server) handleEntriesList(ctx context.Context, actor authz.Actor, grants []authz.AgentGrant, args map[string]any) (*mcp.CallToolResult, any, error) {
	contentType, _ := args["content_type"].(string)
	status, _ := args["status"].(string)
	search, _ := args["search"].(string)
	limitF, _ := args["limit"].(float64)
	offsetF, _ := args["offset"].(float64)
	limit := int64(limitF)
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	offset := int64(offsetF)
	if offset < 0 {
		offset = 0
	}
	// If content_type specified, check permission for that type
	if contentType != "" {
		if !checkPerm(actor, authz.PermEntriesRead, grants, contentType) {
			return mcpError("forbidden: missing entries.read for content_type:" + contentType)
		}
		rows, err := s.queries.ListEntriesAdmin(ctx, db.ListEntriesAdminParams{
			ContentTypeID: contentType,
			StatusFilter:  status,
			Search:        search,
			Limit:         limit,
			Offset:        offset,
		})
		if err != nil {
			return mcpError("failed to list entries: " + err.Error())
		}
		out := make([]map[string]any, 0, len(rows))
		for _, r := range rows {
			out = append(out, map[string]any{
				"entry_id":           r.ID,
				"content_type":       contentType,
				"title":              r.Title.String,
				"slug":               r.Slug,
				"status":             r.Status,
				"latest_revision_id": r.LatestRevisionID.String,
				"published_revision_id": r.PublishedRevisionID.String,
				"updated_at":         r.UpdatedAt,
				"public_path":        r.PublicPath.String,
				"has_schedule":       r.HasSchedule != 0,
			})
		}
		return mcpJSONResult(map[string]any{"entries": out, "count": len(out)})
	}
	// No content_type filter: list across allowed types
	cat := content.NewCatalog(s.queries)
	defs, _ := cat.ListDefinitions(ctx)
	var allowed []string
	for _, d := range defs {
		if checkPerm(actor, authz.PermEntriesRead, grants, string(d.ID)) {
			allowed = append(allowed, string(d.ID))
		}
	}
	// For user with broad perms, allowed will be populated via role; for agent with "*" it will also be populated
	if len(allowed) == 0 {
		// No grants at all
		return mcpError("forbidden: missing entries.read")
	}
	var out []map[string]any
	for _, ct := range allowed {
		rows, err := s.queries.ListEntriesAdmin(ctx, db.ListEntriesAdminParams{
			ContentTypeID: ct,
			StatusFilter:  status,
			Search:        search,
			Limit:         limit,
			Offset:        offset,
		})
		if err != nil {
			continue
		}
		for _, r := range rows {
			out = append(out, map[string]any{
				"entry_id":           r.ID,
				"content_type":       ct,
				"title":              r.Title.String,
				"slug":               r.Slug,
				"status":             r.Status,
				"latest_revision_id": r.LatestRevisionID.String,
				"published_revision_id": r.PublishedRevisionID.String,
				"updated_at":         r.UpdatedAt,
				"public_path":        r.PublicPath.String,
			})
			if int64(len(out)) >= limit {
				break
			}
		}
		if int64(len(out)) >= limit {
			break
		}
	}
	return mcpJSONResult(map[string]any{"entries": out, "count": len(out)})
}

func (s *Server) handleEntryGet(ctx context.Context, actor authz.Actor, grants []authz.AgentGrant, entryID string) (*mcp.CallToolResult, any, error) {
	if entryID == "" {
		return mcpError("validation failed: entry_id required")
	}
	entry, err := s.queries.GetEntry(ctx, entryID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return mcpError("not found: entry " + entryID)
		}
		return mcpError("failed to get entry: " + err.Error())
	}
	if !checkPerm(actor, authz.PermEntriesRead, grants, entry.ContentTypeID) {
		return mcpError("forbidden: missing entries.read for content_type:" + entry.ContentTypeID)
	}
	latest, err := s.queries.GetLatestEntryRevision(ctx, entryID)
	if err != nil {
		return mcpError("failed to get revision: " + err.Error())
	}
	var published *db.EntryRevision
	if entry.PublishedRevisionID.Valid {
		if rev, err := s.queries.GetEntryRevision(ctx, entry.PublishedRevisionID.String); err == nil {
			published = &rev
		}
	}
	terms, _ := s.queries.ListTermsForRevision(ctx, latest.ID)
	var taxAssign []map[string]any
	for _, t := range terms {
		taxAssign = append(taxAssign, map[string]any{"id": t.ID, "name": t.Name, "slug": t.Slug, "taxonomy_id": t.TaxonomyID})
	}
	var doc any
	_ = json.Unmarshal([]byte(latest.DocumentJson), &doc)
	fields := map[string]any{}
	_ = json.Unmarshal([]byte(latest.FieldsJson), &fields)
	result := map[string]any{
		"entry_id":                entry.ID,
		"content_type":            entry.ContentTypeID,
		"author_id":               entry.AuthorID.String,
		"status":                  entry.Status,
		"latest_revision_id":      latest.ID,
		"latest_revision_number":  latest.RevisionNumber,
		"published_revision_id":   entry.PublishedRevisionID.String,
		"title":                   latest.Title,
		"slug":                    latest.Slug,
		"excerpt":                 latest.Excerpt.String,
		"seo_title":               latest.SeoTitle.String,
		"seo_description":         latest.SeoDescription.String,
		"canonical_url":           latest.CanonicalUrl.String,
		"document":                doc,
		"fields":                  fields,
		"taxonomy":                taxAssign,
		"visibility":              latest.Visibility,
		"sticky":                  latest.Sticky != 0,
		"comments_enabled":        latest.CommentsEnabled != 0,
		"layout_template_id":      latest.LayoutTemplateID.String,
		"parent_entry_id":         latest.ParentEntryID.String,
		"menu_order":              latest.MenuOrder,
		"featured_media_id":       latest.FeaturedMediaID.String,
		"social_media_id":         latest.SocialMediaID.String,
		"created_by":              latest.CreatedBy.String,
		"created_by_kind":         latest.CreatedByKind,
		"created_at":              latest.CreatedAt,
	}
	if published != nil {
		result["published_document"] = published.DocumentJson
	}
	return mcpJSONResult(result)
}

func (s *Server) handleEntryCreate(ctx context.Context, actor authz.Actor, grants []authz.AgentGrant, args map[string]any) (*mcp.CallToolResult, any, error) {
	contentType, _ := args["content_type"].(string)
	title, _ := args["title"].(string)
	if contentType == "" || strings.TrimSpace(title) == "" {
		return mcpError("validation failed: content_type and title required")
	}
	if !checkPerm(actor, authz.PermEntriesCreate, grants, contentType) {
		return mcpError("forbidden: missing entries.create for content_type:" + contentType)
	}
	patch := mcpArgsToPatch(args)
	entryID, revID, revNo, err := s.entryOps.CreateDraft(ctx, actor, contentType, "", patch)
	if err != nil {
		return mcpError(mapEntryOpsError(err))
	}
	return mcpJSONResult(map[string]any{
		"entry_id": entryID, "revision_id": revID, "revision_number": revNo,
	})
}

func (s *Server) handleEntryUpdate(ctx context.Context, actor authz.Actor, grants []authz.AgentGrant, args map[string]any) (*mcp.CallToolResult, any, error) {
	entryID, _ := args["entry_id"].(string)
	expected, _ := args["expected_revision_id"].(string)
	changesRaw, _ := args["changes"].(map[string]any)
	if entryID == "" || expected == "" || changesRaw == nil {
		return mcpError("validation failed: entry_id, expected_revision_id and changes required")
	}
	entry, err := s.queries.GetEntry(ctx, entryID)
	if err != nil {
		return mcpError("not found: entry " + entryID)
	}
	if !checkPerm(actor, authz.PermEntriesEdit, grants, entry.ContentTypeID) {
		return mcpError("forbidden: missing entries.edit for content_type:" + entry.ContentTypeID)
	}
	patch, err := changesToPatch(changesRaw)
	if err != nil {
		return mcpError("invalid changes: " + err.Error())
	}
	revID, revNo, changed, err := s.entryOps.UpdateDraft(ctx, actor, entryID, expected, *patch)
	if err != nil {
		return mcpError(mapEntryOpsError(err))
	}
	return mcpJSONResult(map[string]any{
		"entry_id": entryID, "previous_revision_id": expected, "revision_id": revID, "revision_number": revNo, "changed_fields": changed,
	})
}

func (s *Server) handleEntryPublish(ctx context.Context, actor authz.Actor, grants []authz.AgentGrant, args map[string]any) (*mcp.CallToolResult, any, error) {
	entryID, _ := args["entry_id"].(string)
	revID, _ := args["revision_id"].(string)
	if entryID == "" || revID == "" {
		return mcpError("validation failed: entry_id and revision_id required")
	}
	entry, err := s.queries.GetEntry(ctx, entryID)
	if err != nil {
		return mcpError("not found: entry " + entryID)
	}
	if !checkPerm(actor, authz.PermEntriesPublish, grants, entry.ContentTypeID) {
		return mcpError("forbidden: missing entries.publish for content_type:" + entry.ContentTypeID)
	}
	path, err := s.entryOps.Publish(ctx, actor, entryID, revID)
	if err != nil {
		return mcpError(mapEntryOpsError(err))
	}
	return mcpJSONResult(map[string]any{"entry_id": entryID, "published_revision_id": revID, "public_path": path})
}

func (s *Server) handleEntryTrash(ctx context.Context, actor authz.Actor, grants []authz.AgentGrant, args map[string]any) (*mcp.CallToolResult, any, error) {
	entryID, _ := args["entry_id"].(string)
	if entryID == "" {
		return mcpError("validation failed: entry_id required")
	}
	entry, err := s.queries.GetEntry(ctx, entryID)
	if err != nil {
		return mcpError("not found: entry " + entryID)
	}
	if !checkPerm(actor, authz.PermEntriesTrash, grants, entry.ContentTypeID) {
		return mcpError("forbidden: missing entries.trash for content_type:" + entry.ContentTypeID)
	}
	if err := s.entryOps.Trash(ctx, actor, entryID); err != nil {
		return mcpError(mapEntryOpsError(err))
	}
	return mcpJSONResult(map[string]any{"entry_id": entryID, "status": "trash"})
}

func mcpArgsToPatch(args map[string]any) entryops.EntryPatch {
	var p entryops.EntryPatch
	if v, ok := args["title"].(string); ok {
		p.Title = &v
	}
	if v, ok := args["slug"].(string); ok {
		p.Slug = &v
	}
	if v, ok := args["excerpt"].(string); ok {
		p.Excerpt = &v
	}
	if v, ok := args["seo_title"].(string); ok {
		p.SEOTitle = &v
	}
	if v, ok := args["seo_description"].(string); ok {
		p.SEODescription = &v
	}
	if v, ok := args["canonical_url"].(string); ok {
		p.CanonicalURL = &v
	}
	if v, ok := args["featured_media_id"].(string); ok {
		p.FeaturedMediaID = &v
	}
	if v, ok := args["social_media_id"].(string); ok {
		p.SocialMediaID = &v
	}
	if v, ok := args["visibility"].(string); ok {
		p.Visibility = &v
	}
	if v, ok := args["password"].(string); ok {
		p.Password = &v
		p.PasswordSet = true
	}
	if v, ok := args["parent_entry_id"].(string); ok {
		p.ParentEntryID = &v
		p.ParentSet = true
	}
	if v, ok := args["layout_template_id"].(string); ok {
		p.LayoutTemplateID = &v
		p.LayoutSet = true
	}
	if doc, ok := args["document"]; ok {
		b, _ := json.Marshal(doc)
		var d document.Document
		if json.Unmarshal(b, &d) == nil {
			p.Document = &d
			p.DocumentSet = true
		}
	}
	if fields, ok := args["fields"].(map[string]any); ok {
		p.Fields = fields
		p.FieldsSet = true
	}
	if tax, ok := args["taxonomy"].(map[string]any); ok {
		m := map[string][]string{}
		for k, v := range tax {
			if arr, ok := v.([]any); ok {
				for _, e := range arr {
					if s, ok := e.(string); ok {
						m[k] = append(m[k], s)
					}
				}
			} else if s, ok := v.(string); ok {
				m[k] = []string{s}
			}
		}
		p.TaxonomyValues = m
		p.TaxonomySet = true
	}
	return p
}

func changesToPatch(changes map[string]any) (*entryops.EntryPatch, error) {
	b, _ := json.Marshal(changes)
	var p entryops.EntryPatch
	if err := json.Unmarshal(b, &p); err != nil {
		return nil, err
	}
	if _, ok := changes["document"]; ok {
		p.DocumentSet = true
		if p.Document == nil {
			// Try re-parse
			bdoc, _ := json.Marshal(changes["document"])
			var d document.Document
			if err := json.Unmarshal(bdoc, &d); err == nil {
				p.Document = &d
			}
		}
	}
	if _, ok := changes["fields"]; ok {
		p.FieldsSet = true
	}
	if _, ok := changes["layout_template_id"]; ok {
		p.LayoutSet = true
	}
	if _, ok := changes["parent_entry_id"]; ok {
		p.ParentSet = true
	}
	if _, ok := changes["password"]; ok {
		p.PasswordSet = true
	}
	if _, ok := changes["taxonomy"]; ok {
		p.TaxonomySet = true
	}
	if _, ok := changes["taxonomy_values"]; ok {
		p.TaxonomySet = true
	}
	return &p, nil
}

func mapEntryOpsError(err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	if strings.Contains(msg, "revision conflict") || strings.Contains(msg, "revision_conflict") {
		return "revision_conflict: " + msg
	}
	if strings.Contains(msg, "forbidden") {
		return "forbidden: " + msg
	}
	if strings.Contains(msg, "not found") {
		return "not_found: " + msg
	}
	if strings.Contains(msg, "validation") {
		return "validation_failed: " + msg
	}
	return msg
}

var _ = fmt.Sprintf
var _ = entryops.ErrNotFound
