package mcpserver

import (
	"context"
	"encoding/json"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kokosx/stratum/internal/authz"
	"github.com/kokosx/stratum/internal/content"
)

func (s *Server) handleContentTypesList(ctx context.Context, actor authz.Actor, grants []authz.AgentGrant) (*mcp.CallToolResult, any, error) {
	if !checkPerm(actor, authz.PermContentTypesRead, grants, "") {
		return mcpError("forbidden: missing content_types.read")
	}
	cat := content.NewCatalog(s.queries)
	defs, err := cat.ListDefinitions(ctx)
	if err != nil {
		return mcpError("failed to list content types: " + err.Error())
	}
	out := make([]map[string]any, 0, len(defs))
	for _, d := range defs {
		out = append(out, map[string]any{
			"id":         string(d.ID),
			"label":      d.PluralName,
			"item_label": d.Name,
			"capabilities": map[string]any{
				"has_content":      d.Capabilities.HasContent,
				"has_seo":          d.Capabilities.HasSEO,
				"has_featured":     d.Capabilities.HasFeatured,
				"has_excerpt":      d.Capabilities.HasExcerpt,
				"hierarchical":     d.Capabilities.Hierarchical,
				"supports_sticky":  d.Capabilities.SupportsSticky,
				"supports_comments": d.Capabilities.SupportsComments,
			},
			"routing": map[string]any{
				"single":  d.Routing.Single,
				"archive": d.Routing.Archive,
				"base_path": d.Routing.BasePath,
			},
		})
	}
	return mcpJSONResult(map[string]any{"content_types": out})
}

func (s *Server) handleContentTypeGet(ctx context.Context, actor authz.Actor, grants []authz.AgentGrant, id string) (*mcp.CallToolResult, any, error) {
	if !checkPerm(actor, authz.PermContentTypesRead, grants, "") {
		return mcpError("forbidden: missing content_types.read")
	}
	if id == "" {
		return mcpError("validation failed: id is required")
	}
	cat := content.NewCatalog(s.queries)
	def, err := cat.GetDefinition(ctx, id)
	if err != nil {
		return mcpError("content type not found: " + id)
	}
	// Build detailed response
	// Fields
	var fields []map[string]any
	for _, f := range def.Fields {
		b, _ := json.Marshal(f)
		var m map[string]any
		_ = json.Unmarshal(b, &m)
		fields = append(fields, m)
	}
	result := map[string]any{
		"id":             string(def.ID),
		"label":          def.PluralName,
		"item_label":     def.Name,
		"capabilities": map[string]any{
			"has_content": dCap(def.Capabilities.HasContent),
			"has_seo": dCap(def.Capabilities.HasSEO),
			"has_featured": dCap(def.Capabilities.HasFeatured),
			"has_excerpt": dCap(def.Capabilities.HasExcerpt),
			"hierarchical": dCap(def.Capabilities.Hierarchical),
		},
		"routing": map[string]any{
			"single": dCap(def.Routing.Single),
			"archive": dCap(def.Routing.Archive),
			"base_path": def.Routing.BasePath,
		},
		"fields": fields,
		"schema_version": def.SchemaVersion,
	}
	// Include taxonomies for this content type
	if taxRows, err := s.queries.ListTaxonomiesByContentType(ctx, id); err == nil {
		var taxs []map[string]any
		for _, t := range taxRows {
			taxs = append(taxs, map[string]any{
				"id": t.ID, "plural_name": t.PluralName, "singular_name": t.SingularName,
				"hierarchical": t.Hierarchical != 0,
			})
		}
		result["taxonomies"] = taxs
	}
	return mcpJSONResult(result)
}

func dCap(b bool) bool { return b }
