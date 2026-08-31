package mcpserver

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kokosx/stratum/internal/authz"
	db "github.com/kokosx/stratum/internal/storage/sqlc"
)

func (s *Server) handleBlocksList(ctx context.Context, actor authz.Actor, grants []authz.AgentGrant) (*mcp.CallToolResult, any, error) {
	if !checkPerm(actor, authz.PermBlocksRead, grants, "") {
		return mcpError("forbidden: missing blocks.read")
	}
	rows, err := s.queries.ListBlockDefinitions(ctx)
	if err != nil {
		return mcpError("failed to list blocks: " + err.Error())
	}
	out := make([]map[string]any, 0, len(rows))
	for _, r := range rows {
		if r.Enabled == 0 {
			continue
		}
		out = append(out, map[string]any{
			"type":         r.Namespace + "/" + r.Name,
			"version":      r.Version,
			"display_name": r.DisplayName,
			"description":  r.Description.String,
		})
	}
	return mcpJSONResult(map[string]any{"blocks": out})
}

func (s *Server) handleBlockGet(ctx context.Context, actor authz.Actor, grants []authz.AgentGrant, blockType string, version int64) (*mcp.CallToolResult, any, error) {
	if !checkPerm(actor, authz.PermBlocksRead, grants, "") {
		return mcpError("forbidden: missing blocks.read")
	}
	if blockType == "" {
		return mcpError("validation failed: type is required")
	}
	ns, name := parseBlockType(blockType)
	if version == 0 {
		// Find latest version for this block
		rows, err := s.queries.ListBlockDefinitions(ctx)
		if err != nil {
			return mcpError("failed to get block: " + err.Error())
		}
		var latest int64 = -1
		var latestRow *struct {
			Version int64
			Schema  string
		}
		for _, r := range rows {
			if r.Namespace == ns && r.Name == name && r.Version > latest {
				latest = r.Version
				// keep reference? We'll fetch via query
			}
		}
		if latest == -1 {
			return mcpError("block not found: " + blockType)
		}
		version = latest
		_ = latestRow
	}
	row, err := s.queries.GetBlockDefinition(ctx, db.GetBlockDefinitionParams{Namespace: ns, Name: name, Version: version})
	if err != nil {
		return mcpError("block not found: " + blockType)
	}
	var schema map[string]any
	_ = json.Unmarshal([]byte(row.SchemaJson), &schema)
	result := map[string]any{
		"type":         row.Namespace + "/" + row.Name,
		"version":      row.Version,
		"display_name": row.DisplayName,
		"schema":       schema,
		"template":     row.Template.String,
		"styles":       row.Styles.String,
	}
	return mcpJSONResult(result)
}

func parseBlockType(t string) (string, string) {
	if strings.Contains(t, "/") {
		parts := strings.SplitN(t, "/", 2)
		return parts[0], parts[1]
	}
	return "core", t
}
