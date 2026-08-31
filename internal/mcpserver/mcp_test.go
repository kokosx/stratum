package mcpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kokosx/stratum/internal/audit"
	"github.com/kokosx/stratum/internal/agents"
	"github.com/kokosx/stratum/internal/auth"
	"github.com/kokosx/stratum/internal/authz"
	"github.com/kokosx/stratum/internal/blocks"
	"github.com/kokosx/stratum/internal/entryops"
	"github.com/kokosx/stratum/internal/publishing"
	"github.com/kokosx/stratum/internal/runtimehub"
	"github.com/kokosx/stratum/internal/storage"
	db "github.com/kokosx/stratum/internal/storage/sqlc"
	"github.com/kokosx/stratum/internal/themes"
)

func newTestMCP(t *testing.T) (*Server, *agents.Service, *auth.Service) {
	t.Helper()
	ctx := context.Background()
	dir := t.TempDir()
	database, err := storage.Open(filepath.Join(dir, "stratum.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if err := database.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	queries := db.New(database.DB)
	authSvc, err := auth.NewService(database.DB, queries, false)
	if err != nil {
		t.Fatal(err)
	}
	_, err = authSvc.Setup(ctx, authSvc.SetupCode(), "Test", "admin@example.com", "correct-horse-battery-staple")
	if err != nil {
		t.Fatal(err)
	}
	// Get admin user for default author
	var adminID string
	users, _ := queries.ListUsers(ctx)
	for _, u := range users {
		if u.Email == "admin@example.com" {
			adminID = u.ID
		}
	}
	registry, err := blocks.NewRegistry(ctx, queries)
	if err != nil {
		t.Fatal(err)
	}
	themeRt, err := themes.NewRuntime(ctx, queries)
	if err != nil {
		t.Fatal(err)
	}
	hub, err := runtimehub.New(queries, registry, themeRt, nil)
	if err != nil {
		t.Fatal(err)
	}
	publisher := publishing.New(database.DB, queries)
	auditSvc := audit.New(database.DB, queries)
	agentsSvc := agents.New(database.DB, queries)
	entryOpsSvc := entryops.New(database.DB, queries, registry, publisher, auditSvc, hub)

	srv := New(database.DB, queries, agentsSvc, entryOpsSvc, registry)
	// Create test agent with default author
	ag, err := agentsSvc.Create(ctx, "Test Agent", adminID, adminID)
	if err != nil {
		t.Fatal(err)
	}
	// Need to set grants elsewhere
	_ = ag
	return srv, agentsSvc, authSvc
}

func doMCP(t *testing.T, srv *Server, token string, method string, params any) (int, string) {
	t.Helper()
	body := map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  method,
		"params":  params,
	}
	if method == "tools/call" {
		// params already contains name and arguments
	} else if method == "initialize" {
		// initialize expects params with protocolVersion
	}
	b, _ := json.Marshal(body)
	req := httptest.NewRequest("POST", "/stratum/mcp", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	resp := rec.Result()
	data, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(data)
}

func TestMCPAuth(t *testing.T) {
	srv, agentsSvc, _ := newTestMCP(t)
	ctx := context.Background()
	users, _ := srv.queries.ListUsers(ctx)
	var adminID string
	for _, u := range users {
		if u.Email == "admin@example.com" {
			adminID = u.ID
		}
	}
	ag, _ := agentsSvc.Create(ctx, "Auth Agent", adminID, adminID)
	_ = agentsSvc.ReplaceGrants(ctx, ag.ID, []authz.AgentGrant{{Permission: "site.read", Scope: "*"}})
	issued, _ := agentsSvc.IssueToken(ctx, ag.ID, "tok", nil)

	// Missing token -> 401
	code, _ := doMCP(t, srv, "", "tools/call", map[string]any{"name": "site_get", "arguments": map[string]any{}})
	if code != 401 {
		t.Fatalf("expected 401 missing token, got %d", code)
	}
	// Invalid token -> 401
	code, _ = doMCP(t, srv, "stratum_agent_invalid", "tools/call", map[string]any{"name": "site_get", "arguments": map[string]any{}})
	if code != 401 {
		t.Fatalf("expected 401 invalid token, got %d", code)
	}
	// Valid token -> success
	code, body := doMCP(t, srv, issued.Raw, "tools/call", map[string]any{"name": "site_get", "arguments": map[string]any{}})
	if code != 200 {
		t.Fatalf("expected 200 valid token, got %d body %s", code, body)
	}
	// Wrong grant -> forbidden but still 200 with IsError
	// Try entries_list without grant? But this agent has only site.read, so should be forbidden
	code, body = doMCP(t, srv, issued.Raw, "tools/call", map[string]any{"name": "entries_list", "arguments": map[string]any{"content_type": "post"}})
	if code != 200 {
		t.Fatalf("expected 200 for forbidden tool call (error inside), got %d", code)
	}
	if !contains(body, "forbidden") {
		t.Fatalf("expected forbidden error, got %s", body)
	}
	// Disabled agent -> 401
	_ = agentsSvc.SetStatus(ctx, ag.ID, "disabled")
	code, _ = doMCP(t, srv, issued.Raw, "tools/call", map[string]any{"name": "site_get", "arguments": map[string]any{}})
	if code != 401 {
		t.Fatalf("expected 401 disabled agent, got %d", code)
	}
	// Revoked token -> 401
	// Create new agent for revoke test
	ag2, _ := agentsSvc.Create(ctx, "Revoke Agent", adminID, adminID)
	_ = agentsSvc.ReplaceGrants(ctx, ag2.ID, []authz.AgentGrant{{Permission: "site.read", Scope: "*"}})
	iss2, _ := agentsSvc.IssueToken(ctx, ag2.ID, "tok2", nil)
	iss3, _ := agentsSvc.IssueToken(ctx, ag2.ID, "tok3", nil)
	_ = agentsSvc.RevokeToken(ctx, iss2.TokenID)
	code, _ = doMCP(t, srv, iss2.Raw, "tools/call", map[string]any{"name": "site_get", "arguments": map[string]any{}})
	if code != 401 {
		t.Fatalf("expected 401 revoked token, got %d", code)
	}
	// Other token still works
	code, body = doMCP(t, srv, iss3.Raw, "tools/call", map[string]any{"name": "site_get", "arguments": map[string]any{}})
	if code != 200 {
		t.Fatalf("expected 200 for other token, got %d body %s", code, body)
	}
}

func contains(s, sub string) bool { return strings.Contains(s, sub) }
