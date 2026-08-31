package mcpserver

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kokosx/stratum/internal/authz"
	db "github.com/kokosx/stratum/internal/storage/sqlc"
)

func (s *Server) handleMediaList(ctx context.Context, actor authz.Actor, grants []authz.AgentGrant, args map[string]any) (*mcp.CallToolResult, any, error) {
	if !checkPerm(actor, authz.PermMediaRead, grants, "") {
		return mcpError("forbidden: missing media.read")
	}
	limitF, _ := args["limit"].(float64)
	offsetF, _ := args["offset"].(float64)
	limit := int64(limitF)
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	offset := int64(offsetF)
	rows, err := s.queries.ListMedia(ctx, db.ListMediaParams{Limit: limit, Offset: offset})
	if err != nil {
		return mcpError("failed to list media: " + err.Error())
	}
	out := make([]map[string]any, 0, len(rows))
	for _, m := range rows {
		out = append(out, map[string]any{
			"id": m.ID, "filename": m.OriginalFilename, "mime_type": m.MimeType, "size": m.FileSize, "created_at": m.CreatedAt,
		})
	}
	return mcpJSONResult(map[string]any{"media": out})
}

func (s *Server) handleMediaGet(ctx context.Context, actor authz.Actor, grants []authz.AgentGrant, mediaID string) (*mcp.CallToolResult, any, error) {
	if !checkPerm(actor, authz.PermMediaRead, grants, "") {
		return mcpError("forbidden: missing media.read")
	}
	if mediaID == "" {
		return mcpError("validation failed: media_id required")
	}
	m, err := s.queries.GetMedia(ctx, mediaID)
	if err != nil {
		return mcpError("not found: media " + mediaID)
	}
	variants, _ := s.queries.ListMediaVariants(ctx, mediaID)
	var vars []map[string]any
	for _, v := range variants {
		vars = append(vars, map[string]any{"kind": v.Kind, "storage_key": v.StorageKey})
	}
	result := map[string]any{
		"id": m.ID, "filename": m.OriginalFilename, "mime_type": m.MimeType, "size": m.FileSize,
		"alt_text": m.AltText, "caption": m.Caption, "created_at": m.CreatedAt,
		"variants": vars,
	}
	return mcpJSONResult(result)
}
