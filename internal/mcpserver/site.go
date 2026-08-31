package mcpserver

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kokosx/stratum/internal/authz"
	"github.com/kokosx/stratum/internal/version"
)

func (s *Server) handleSiteGet(ctx context.Context, actor authz.Actor, grants []authz.AgentGrant) (*mcp.CallToolResult, any, error) {
	if !checkPerm(actor, authz.PermSiteRead, grants, "") {
		return mcpError("forbidden: missing site.read")
	}
	settings, err := s.queries.GetSiteSettings(ctx)
	if err != nil {
		return mcpError("failed to load site settings: " + err.Error())
	}
	result := map[string]any{
		"site_title": settings.SiteTitle,
		"site_url":   settings.SiteUrl,
		"site_tagline": settings.SiteTagline,
		"language": settings.Language,
		"timezone": settings.Timezone,
		"version":  version.Version,
	}
	return mcpJSONResult(result)
}
