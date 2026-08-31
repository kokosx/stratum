package agents

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/kokosx/stratum/internal/authz"
	db "github.com/kokosx/stratum/internal/storage/sqlc"
)

const tokenPrefixStr = "stratum_agent_"

var (
	ErrAgentNotFound  = errors.New("agent not found")
	ErrTokenNotFound  = errors.New("agent token not found")
	ErrInvalidName    = errors.New("agent name is required")
	ErrInvalidToken   = errors.New("invalid agent token")
	ErrAgentDisabled  = errors.New("agent is disabled")
	ErrAgentTokenRevoked = errors.New("agent token revoked")
	ErrAgentTokenExpired = errors.New("agent token expired")
)

// Service manages agent identities and machine credentials.
type Service struct {
	db      *sql.DB
	queries *db.Queries
}

func New(database *sql.DB, queries *db.Queries) *Service {
	return &Service{db: database, queries: queries}
}

type Agent struct {
	ID               string
	Name             string
	Status           string
	DefaultAuthorID  sql.NullString
	CreatedByUserID  sql.NullString
	CreatedAt        int64
	UpdatedAt        int64
}

// Create creates a new agent with optional default author.
func (s *Service) Create(ctx context.Context, name, defaultAuthorID, createdByUserID string) (*Agent, error) {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 120 {
		return nil, ErrInvalidName
	}
	if s.db == nil || s.queries == nil {
		return nil, errors.New("database not configured")
	}
	var defaultAuthor sql.NullString
	if strings.TrimSpace(defaultAuthorID) != "" {
		// Validate author exists and is active
		u, err := s.queries.GetUserByID(ctx, strings.TrimSpace(defaultAuthorID))
		if err != nil {
			return nil, fmt.Errorf("default author not found: %w", err)
		}
		if u.Status != "active" {
			return nil, errors.New("default author is not active")
		}
		defaultAuthor = sql.NullString{String: strings.TrimSpace(defaultAuthorID), Valid: true}
	}
	id, err := randomID()
	if err != nil {
		return nil, err
	}
	now := time.Now().Unix()
	var createdBy sql.NullString
	if strings.TrimSpace(createdByUserID) != "" {
		createdBy = sql.NullString{String: strings.TrimSpace(createdByUserID), Valid: true}
	}
	if err := s.queries.CreateAgent(ctx, db.CreateAgentParams{
		ID: id, Name: name, Status: "active",
		DefaultAuthorID: defaultAuthor, CreatedByUserID: createdBy,
		CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		return nil, err
	}
	return &Agent{ID: id, Name: name, Status: "active", DefaultAuthorID: defaultAuthor, CreatedByUserID: createdBy, CreatedAt: now, UpdatedAt: now}, nil
}

func (s *Service) Get(ctx context.Context, id string) (*Agent, error) {
	row, err := s.queries.GetAgent(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrAgentNotFound
		}
		return nil, err
	}
	return dbAgentToDomain(row), nil
}

func (s *Service) List(ctx context.Context) ([]Agent, error) {
	rows, err := s.queries.ListAgents(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Agent, 0, len(rows))
	for _, r := range rows {
		out = append(out, *dbAgentToDomain(r))
	}
	return out, nil
}

func (s *Service) Update(ctx context.Context, id, name, defaultAuthorID string) error {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 120 {
		return ErrInvalidName
	}
	var defaultAuthor sql.NullString
	if strings.TrimSpace(defaultAuthorID) != "" {
		u, err := s.queries.GetUserByID(ctx, strings.TrimSpace(defaultAuthorID))
		if err != nil {
			return fmt.Errorf("default author not found: %w", err)
		}
		if u.Status != "active" {
			return errors.New("default author is not active")
		}
		defaultAuthor = sql.NullString{String: strings.TrimSpace(defaultAuthorID), Valid: true}
	}
	if _, err := s.queries.GetAgent(ctx, id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrAgentNotFound
		}
		return err
	}
	now := time.Now().Unix()
	return s.queries.UpdateAgent(ctx, db.UpdateAgentParams{
		Name: name, DefaultAuthorID: defaultAuthor, UpdatedAt: now, ID: id,
	})
}

func (s *Service) SetStatus(ctx context.Context, id, status string) error {
	if status != "active" && status != "disabled" {
		return errors.New("invalid agent status")
	}
	if _, err := s.queries.GetAgent(ctx, id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrAgentNotFound
		}
		return err
	}
	return s.queries.UpdateAgentStatus(ctx, db.UpdateAgentStatusParams{
		Status: status, UpdatedAt: time.Now().Unix(), ID: id,
	})
}

func (s *Service) Delete(ctx context.Context, id string) error {
	return s.queries.DeleteAgent(ctx, id)
}

// Grants

func (s *Service) ListGrants(ctx context.Context, agentID string) ([]authz.AgentGrant, error) {
	if _, err := s.queries.GetAgent(ctx, agentID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrAgentNotFound
		}
		return nil, err
	}
	rows, err := s.queries.ListAgentGrants(ctx, agentID)
	if err != nil {
		return nil, err
	}
	out := make([]authz.AgentGrant, 0, len(rows))
	for _, r := range rows {
		out = append(out, authz.AgentGrant{Permission: r.Permission, Scope: r.Scope})
	}
	return out, nil
}

func (s *Service) ReplaceGrants(ctx context.Context, agentID string, grants []authz.AgentGrant) error {
	if _, err := s.queries.GetAgent(ctx, agentID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrAgentNotFound
		}
		return err
	}
	// Validate
	for _, g := range grants {
		if !authz.IsValidAgentPermission(authz.Permission(g.Permission)) {
			return fmt.Errorf("invalid permission %q", g.Permission)
		}
		if g.Scope != authz.ScopeAll && !strings.HasPrefix(g.Scope, "content_type:") {
			return fmt.Errorf("invalid scope %q", g.Scope)
		}
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	qtx := s.queries.WithTx(tx)
	if err := qtx.DeleteAgentGrants(ctx, agentID); err != nil {
		return err
	}
	for _, g := range grants {
		if err := qtx.AddAgentGrant(ctx, db.AddAgentGrantParams{AgentID: agentID, Permission: g.Permission, Scope: g.Scope}); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// Tokens

type IssuedToken struct {
	TokenID     string
	AgentID     string
	Raw         string // returned exactly once
	Prefix      string
	Label       string
	CreatedAt   int64
	ExpiresAt   sql.NullInt64
}

// IssueToken generates a new high-entropy token for the agent.
// Raw token is returned once and never persisted.
func (s *Service) IssueToken(ctx context.Context, agentID, label string, expiresAt *int64) (*IssuedToken, error) {
	if _, err := s.queries.GetAgent(ctx, agentID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrAgentNotFound
		}
		return nil, err
	}
	raw, err := generateRawToken()
	if err != nil {
		return nil, err
	}
	hash := tokenHash(raw)
	prefix := tokenDisplayPrefix(raw)
	id, err := randomID()
	if err != nil {
		return nil, err
	}
	now := time.Now().Unix()
	var exp sql.NullInt64
	if expiresAt != nil && *expiresAt > 0 {
		exp = sql.NullInt64{Int64: *expiresAt, Valid: true}
	}
	label = strings.TrimSpace(label)
	if len(label) > 80 {
		label = label[:80]
	}
	if err := s.queries.CreateAgentToken(ctx, db.CreateAgentTokenParams{
		ID: id, AgentID: agentID, TokenHash: hash, TokenPrefix: prefix,
		Label: label, CreatedAt: now, ExpiresAt: exp,
	}); err != nil {
		return nil, err
	}
	return &IssuedToken{
		TokenID: id, AgentID: agentID, Raw: raw, Prefix: prefix, Label: label,
		CreatedAt: now, ExpiresAt: exp,
	}, nil
}

func (s *Service) ListTokens(ctx context.Context, agentID string) ([]db.AgentToken, error) {
	return s.queries.ListAgentTokens(ctx, agentID)
}

func (s *Service) RevokeToken(ctx context.Context, tokenID string) error {
	if _, err := s.queries.GetAgentToken(ctx, tokenID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrTokenNotFound
		}
		return err
	}
	return s.queries.RevokeAgentToken(ctx, db.RevokeAgentTokenParams{
		RevokedAt: sql.NullInt64{Int64: time.Now().Unix(), Valid: true},
		ID: tokenID,
	})
}

// Authenticate resolves a Bearer token to an Actor.
// It verifies token hash, revocation, expiry, and agent status, then loads grants.
func (s *Service) Authenticate(ctx context.Context, raw string) (authz.Actor, []authz.AgentGrant, error) {
	if strings.TrimSpace(raw) == "" {
		return authz.Actor{}, nil, ErrInvalidToken
	}
	hash := tokenHash(strings.TrimSpace(raw))
	row, err := s.queries.LookupAgentByTokenHash(ctx, hash)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return authz.Actor{}, nil, ErrInvalidToken
		}
		return authz.Actor{}, nil, err
	}
	// Check revoked
	if row.RevokedAt.Valid && row.RevokedAt.Int64 != 0 {
		return authz.Actor{}, nil, ErrAgentTokenRevoked
	}
	// Check expiry
	if row.ExpiresAt.Valid && row.ExpiresAt.Int64 != 0 && row.ExpiresAt.Int64 <= time.Now().Unix() {
		return authz.Actor{}, nil, ErrAgentTokenExpired
	}
	// Check agent status
	if row.AgentStatus != "active" {
		return authz.Actor{}, nil, ErrAgentDisabled
	}
	grantsRows, err := s.queries.ListAgentGrants(ctx, row.AgentID)
	if err != nil {
		return authz.Actor{}, nil, err
	}
	grants := make([]authz.AgentGrant, 0, len(grantsRows))
	for _, g := range grantsRows {
		grants = append(grants, authz.AgentGrant{Permission: g.Permission, Scope: g.Scope})
	}
	// Bounded last_used_at update: only if >5 minutes old
	now := time.Now().Unix()
	shouldUpdate := true
	if row.LastUsedAt.Valid && now-row.LastUsedAt.Int64 < 300 {
		shouldUpdate = false
	}
	if shouldUpdate {
		// Best-effort; ignore error but do not block auth
		_ = s.queries.UpdateAgentTokenLastUsed(ctx, db.UpdateAgentTokenLastUsedParams{
			LastUsedAt:   sql.NullInt64{Int64: now, Valid: true},
			ID:           row.TokenID,
			LastUsedAt_2: sql.NullInt64{Int64: now, Valid: true},
		})
	}
	actor := authz.Actor{
		ID: row.AgentID, Kind: authz.ActorAgent,
		AgentID: row.AgentID, AgentName: row.AgentName,
		DisplayName: row.AgentName,
	}
	return actor, grants, nil
}

// Helpers

func dbAgentToDomain(row db.Agent) *Agent {
	return &Agent{
		ID: row.ID, Name: row.Name, Status: row.Status,
		DefaultAuthorID: row.DefaultAuthorID, CreatedByUserID: row.CreatedByUserID,
		CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	}
}

func generateRawToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return tokenPrefixStr + base64.RawURLEncoding.EncodeToString(b), nil
}

func tokenHash(raw string) string {
	h := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(h[:])
}

func tokenDisplayPrefix(raw string) string {
	// Show first 8 of suffix + last 4 for distinction, without revealing secret
	suffix := strings.TrimPrefix(raw, tokenPrefixStr)
	if len(suffix) < 8 {
		return raw
	}
	prefixPart := suffix[:8]
	lastPart := ""
	if len(suffix) >= 4 {
		lastPart = suffix[len(suffix)-4:]
	}
	return tokenPrefixStr + prefixPart + "…" + lastPart
}

func randomID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
