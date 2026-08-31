package mcpserver

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kokosx/stratum/internal/agents"
	"github.com/kokosx/stratum/internal/authz"
	"github.com/kokosx/stratum/internal/blocks"
	"github.com/kokosx/stratum/internal/content"
	"github.com/kokosx/stratum/internal/entryops"
	db "github.com/kokosx/stratum/internal/storage/sqlc"
	"github.com/kokosx/stratum/internal/version"
)

// Server is the MCP adapter. It authenticates agent Bearer tokens and exposes
// content tools that delegate to the shared application services.
type Server struct {
	agents   *agents.Service
	entryOps *entryops.Service
	queries  *db.Queries
	blocks   *blocks.Registry
	db       *sql.DB
}

// New creates a new MCP server adapter.
func New(database *sql.DB, queries *db.Queries, agentsSvc *agents.Service, entryOpsSvc *entryops.Service, blockRegistry *blocks.Registry) *Server {
	return &Server{
		db: database, queries: queries, agents: agentsSvc, entryOps: entryOpsSvc, blocks: blockRegistry,
	}
}

// Handler returns the HTTP handler mounted at /stratum/mcp.
// It enforces Bearer token authentication before delegating to the MCP streamable handler.
func (s *Server) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Hardening: body limit, no-store, CORS/origin etc
		r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
		w.Header().Set("Cache-Control", "no-store")
		// Origin validation: allow same-origin or no Origin for now; don't block MCP clients that set Origin
		// For DNS rebinding protection, reject non-local Origin that doesn't match Host if needed?
		// Simplified: if Origin is present and host is localhost/127.0.0.1, validate it matches Host
		if origin := r.Header.Get("Origin"); origin != "" {
			// Basic check: if request is from browser, Origin should be same Host or trusted
			// For MCP, many clients won't send Origin. We allow missing.
			// If Origin contains different host and Host is localhost, reject
			// This is minimal; production should enforce stricter.
			if strings.Contains(r.Host, "localhost") || strings.Contains(r.Host, "127.0.0.1") {
				if !strings.Contains(origin, r.Host) && !strings.Contains(origin, "localhost") && !strings.Contains(origin, "127.0.0.1") {
					http.Error(w, "Forbidden origin", http.StatusForbidden)
					return
				}
			}
		}
		auth := r.Header.Get("Authorization")
		if !strings.HasPrefix(auth, "Bearer ") {
			http.Error(w, `{"jsonrpc":"2.0","error":{"code":-32001,"message":"Unauthorized: missing bearer token"}}`, http.StatusUnauthorized)
			return
		}
		raw := strings.TrimSpace(strings.TrimPrefix(auth, "Bearer "))
		if raw == "" || !strings.HasPrefix(raw, "stratum_agent_") {
			http.Error(w, `{"jsonrpc":"2.0","error":{"code":-32001,"message":"Unauthorized: invalid token"}}`, http.StatusUnauthorized)
			return
		}
		actor, grants, err := s.agents.Authenticate(r.Context(), raw)
		if err != nil {
			http.Error(w, `{"jsonrpc":"2.0","error":{"code":-32001,"message":"Unauthorized"}}`, http.StatusUnauthorized)
			return
		}
		// Build per-request MCP server with actor-bound tools
		mcpServer := s.newMCPServer(actor, grants)
		handler := mcp.NewStreamableHTTPHandler(func(_ *http.Request) *mcp.Server {
			return mcpServer
		}, &mcp.StreamableHTTPOptions{
			Stateless:           true,
			JSONResponse:        true,
			MaxRequestBodyBytes: 1 << 20,
		})
		// Preserve actor in request context for tool handlers that may need it via context (if SDK propagates)
		ctx := context.WithValue(r.Context(), actorContextKey{}, actor)
		ctx = context.WithValue(ctx, grantsContextKey{}, grants)
		r = r.WithContext(ctx)
		handler.ServeHTTP(w, r)
	})
}

type actorContextKey struct{}
type grantsContextKey struct{}

func actorFromContext(ctx context.Context) (authz.Actor, bool) {
	a, ok := ctx.Value(actorContextKey{}).(authz.Actor)
	return a, ok
}

func (s *Server) newMCPServer(actor authz.Actor, grants []authz.AgentGrant) *mcp.Server {
	impl := &mcp.Implementation{Name: "StratumCMS", Version: version.Version}
	srv := mcp.NewServer(impl, nil)
	s.registerTools(srv, actor, grants)
	return srv
}

func (s *Server) registerTools(srv *mcp.Server, actor authz.Actor, grants []authz.AgentGrant) {
	mcp.AddTool(srv, &mcp.Tool{
		Name: "site_get", Description: "Get site metadata (title, URL, language, timezone)",
		InputSchema: map[string]any{"type": "object", "properties": map[string]any{}},
	}, func(ctx context.Context, req *mcp.CallToolRequest, _ map[string]any) (*mcp.CallToolResult, any, error) {
		return s.handleSiteGet(ctx, actor, grants)
	})
	mcp.AddTool(srv, &mcp.Tool{
		Name: "content_types_list", Description: "List available content types",
		InputSchema: map[string]any{"type": "object", "properties": map[string]any{}},
	}, func(ctx context.Context, req *mcp.CallToolRequest, _ map[string]any) (*mcp.CallToolResult, any, error) {
		return s.handleContentTypesList(ctx, actor, grants)
	})
	mcp.AddTool(srv, &mcp.Tool{
		Name: "content_type_get", Description: "Get content type definition including fields and capabilities",
		InputSchema: map[string]any{"type": "object", "properties": map[string]any{
			"id": map[string]any{"type": "string", "description": "Content type ID"},
		}, "required": []string{"id"}},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args map[string]any) (*mcp.CallToolResult, any, error) {
		id, _ := args["id"].(string)
		return s.handleContentTypeGet(ctx, actor, grants, id)
	})
	mcp.AddTool(srv, &mcp.Tool{
		Name: "blocks_list", Description: "List available blocks",
		InputSchema: map[string]any{"type": "object", "properties": map[string]any{}},
	}, func(ctx context.Context, req *mcp.CallToolRequest, _ map[string]any) (*mcp.CallToolResult, any, error) {
		return s.handleBlocksList(ctx, actor, grants)
	})
	mcp.AddTool(srv, &mcp.Tool{
		Name: "block_get", Description: "Get block schema for building SDT",
		InputSchema: map[string]any{"type": "object", "properties": map[string]any{
			"type": map[string]any{"type": "string", "description": "Block type e.g. core/heading"},
			"version": map[string]any{"type": "integer", "description": "Block version"},
		}, "required": []string{"type"}},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args map[string]any) (*mcp.CallToolResult, any, error) {
		btype, _ := args["type"].(string)
		var ver int64
		if v, ok := args["version"].(float64); ok {
			ver = int64(v)
		}
		return s.handleBlockGet(ctx, actor, grants, btype, ver)
	})
	mcp.AddTool(srv, &mcp.Tool{
		Name: "entries_list", Description: "List entries with filtering",
		InputSchema: map[string]any{"type": "object", "properties": map[string]any{
			"content_type": map[string]any{"type": "string"},
			"status": map[string]any{"type": "string", "description": "published, draft, etc or empty for all"},
			"search": map[string]any{"type": "string"},
			"limit": map[string]any{"type": "integer"},
			"offset": map[string]any{"type": "integer"},
		}},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args map[string]any) (*mcp.CallToolResult, any, error) {
		return s.handleEntriesList(ctx, actor, grants, args)
	})
	mcp.AddTool(srv, &mcp.Tool{
		Name: "entry_get", Description: "Get entry with full SDT document and revision IDs",
		InputSchema: map[string]any{"type": "object", "properties": map[string]any{
			"entry_id": map[string]any{"type": "string"},
		}, "required": []string{"entry_id"}},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args map[string]any) (*mcp.CallToolResult, any, error) {
		id, _ := args["entry_id"].(string)
		return s.handleEntryGet(ctx, actor, grants, id)
	})
	mcp.AddTool(srv, &mcp.Tool{
		Name: "media_list", Description: "List media assets",
		InputSchema: map[string]any{"type": "object", "properties": map[string]any{
			"limit":  map[string]any{"type": "integer"},
			"offset": map[string]any{"type": "integer"},
		}},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args map[string]any) (*mcp.CallToolResult, any, error) {
		return s.handleMediaList(ctx, actor, grants, args)
	})
	mcp.AddTool(srv, &mcp.Tool{
		Name: "media_get", Description: "Get media asset metadata",
		InputSchema: map[string]any{"type": "object", "properties": map[string]any{
			"media_id": map[string]any{"type": "string"},
		}, "required": []string{"media_id"}},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args map[string]any) (*mcp.CallToolResult, any, error) {
		id, _ := args["media_id"].(string)
		return s.handleMediaGet(ctx, actor, grants, id)
	})
	mcp.AddTool(srv, &mcp.Tool{
		Name: "entry_create", Description: "Create a new draft entry",
		InputSchema: map[string]any{"type": "object", "properties": map[string]any{
			"content_type": map[string]any{"type": "string"},
			"title": map[string]any{"type": "string"},
			"slug": map[string]any{"type": "string"},
			"excerpt": map[string]any{"type": "string"},
			"seo_title": map[string]any{"type": "string"},
			"seo_description": map[string]any{"type": "string"},
			"document": map[string]any{"type": "object", "description": "SDT document"},
			"fields": map[string]any{"type": "object"},
			"taxonomy": map[string]any{"type": "object", "description": "taxonomy assignment map"},
			"featured_media_id": map[string]any{"type": "string"},
			"parent_entry_id": map[string]any{"type": "string"},
			"visibility": map[string]any{"type": "string"},
			"password": map[string]any{"type": "string"},
		}, "required": []string{"content_type", "title"}},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args map[string]any) (*mcp.CallToolResult, any, error) {
		return s.handleEntryCreate(ctx, actor, grants, args)
	})
	mcp.AddTool(srv, &mcp.Tool{
		Name: "entry_update", Description: "Update entry via PATCH; requires expected_revision_id for optimistic concurrency",
		InputSchema: map[string]any{"type": "object", "properties": map[string]any{
			"entry_id": map[string]any{"type": "string"},
			"expected_revision_id": map[string]any{"type": "string"},
			"changes": map[string]any{"type": "object", "description": "Fields to change"},
		}, "required": []string{"entry_id", "expected_revision_id", "changes"}},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args map[string]any) (*mcp.CallToolResult, any, error) {
		return s.handleEntryUpdate(ctx, actor, grants, args)
	})
	mcp.AddTool(srv, &mcp.Tool{
		Name: "entry_publish", Description: "Publish an exact revision",
		InputSchema: map[string]any{"type": "object", "properties": map[string]any{
			"entry_id": map[string]any{"type": "string"},
			"revision_id": map[string]any{"type": "string"},
		}, "required": []string{"entry_id", "revision_id"}},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args map[string]any) (*mcp.CallToolResult, any, error) {
		return s.handleEntryPublish(ctx, actor, grants, args)
	})
	mcp.AddTool(srv, &mcp.Tool{
		Name: "entry_trash", Description: "Trash (soft-delete) an entry",
		InputSchema: map[string]any{"type": "object", "properties": map[string]any{
			"entry_id": map[string]any{"type": "string"},
		}, "required": []string{"entry_id"}},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args map[string]any) (*mcp.CallToolResult, any, error) {
		return s.handleEntryTrash(ctx, actor, grants, args)
	})
}

// helpers for error mapping
func mcpError(msg string) (*mcp.CallToolResult, any, error) {
	return &mcp.CallToolResult{
		IsError: true,
		Content: []mcp.Content{&mcp.TextContent{Text: msg}},
	}, nil, nil
}

func mcpJSONResult(v any) (*mcp.CallToolResult, any, error) {
	b, _ := json.Marshal(v)
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: string(b)}},
		StructuredContent: v,
	}, nil, nil
}

func checkPerm(actor authz.Actor, perm authz.Permission, grants []authz.AgentGrant, contentType string) bool {
	return authz.Allowed(actor, perm, authz.Resource{ContentTypeID: contentType}, grants)
}

var _ = fmt.Sprintf
var _ = content.DefinitionFor
