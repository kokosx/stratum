package preview

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	db "github.com/kokosx/stratum/internal/storage/sqlc"
)

const (
	// TokenBytes is 32 bytes = 256 bits.
	TokenBytes = 32
	// MaxActivePerEntry limits active links per entry.
	MaxActivePerEntry = 10
)

type PreviewLink struct {
	ID         string
	TokenHash  string
	EntryID    string
	RevisionID string
	CreatedBy  sql.NullString
	ExpiresAt  int64
	RevokedAt  sql.NullInt64
	CreatedAt  int64
}

type PreviewLinkView struct {
	ID         string `json:"id"`
	RevisionID string `json:"revisionId"`
	CreatedAt  int64  `json:"createdAt"`
	ExpiresAt  int64  `json:"expiresAt"`
	CreatedBy  string `json:"createdBy,omitempty"`
}

// Service handles preview link lifecycle.
type Service struct {
	db      *sql.DB
	queries *db.Queries
}

func NewService(database *sql.DB, queries *db.Queries) *Service {
	return &Service{db: database, queries: queries}
}

// Create generates a new preview link for a specific revision. Returns plaintext token and the link.
func (s *Service) Create(ctx context.Context, entryID, revisionID, createdBy string, expiresIn time.Duration) (string, *PreviewLink, error) {
	if strings.TrimSpace(entryID) == "" || strings.TrimSpace(revisionID) == "" {
		return "", nil, fmt.Errorf("entry and revision required")
	}
	if expiresIn <= 0 {
		return "", nil, fmt.Errorf("expiry must be positive")
	}
	// Validate expiry bounded: allow 1h, 24h, 7d, 30d. Reject never or overly long.
	if expiresIn > 30*24*time.Hour {
		return "", nil, fmt.Errorf("expiry too long")
	}
	// Verify entry and revision exist and match
	entry, err := s.queries.GetEntry(ctx, entryID)
	if err != nil {
		return "", nil, fmt.Errorf("entry not found")
	}
	rev, err := s.queries.GetEntryRevision(ctx, revisionID)
	if err != nil {
		return "", nil, fmt.Errorf("revision not found")
	}
	if rev.EntryID != entry.ID {
		return "", nil, fmt.Errorf("revision does not belong to entry")
	}
	// Enforce limit per entry
	var activeCount int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM preview_links WHERE entry_id = ? AND revoked_at IS NULL AND expires_at > ?`, entryID, time.Now().Unix()).Scan(&activeCount); err == nil {
		if activeCount >= MaxActivePerEntry {
			return "", nil, fmt.Errorf("too many active preview links for this entry (max %d)", MaxActivePerEntry)
		}
	}

	// Generate token
	tokenBytes := make([]byte, TokenBytes)
	if _, err := rand.Read(tokenBytes); err != nil {
		return "", nil, fmt.Errorf("generate token: %w", err)
	}
	token := base64.RawURLEncoding.EncodeToString(tokenBytes)
	hash := hashToken(token)

	// Generate ID
	idBytes := make([]byte, 16)
	if _, err := rand.Read(idBytes); err != nil {
		return "", nil, fmt.Errorf("generate id: %w", err)
	}
	id := hex.EncodeToString(idBytes)

	now := time.Now().Unix()
	expiresAt := now + int64(expiresIn.Seconds())

	var createdByVal sql.NullString
	if strings.TrimSpace(createdBy) != "" {
		createdByVal = sql.NullString{String: createdBy, Valid: true}
	}

	_, err = s.db.ExecContext(ctx, `INSERT INTO preview_links (id, token_hash, entry_id, revision_id, created_by, expires_at, revoked_at, created_at) VALUES (?, ?, ?, ?, ?, ?, NULL, ?)`,
		id, hash, entryID, revisionID, createdByVal, expiresAt, now)
	if err != nil {
		return "", nil, fmt.Errorf("create preview link: %w", err)
	}

	link := &PreviewLink{
		ID:         id,
		TokenHash:  hash,
		EntryID:    entryID,
		RevisionID: revisionID,
		CreatedBy:  createdByVal,
		ExpiresAt:  expiresAt,
		CreatedAt:  now,
	}
	return token, link, nil
}

// GetByToken validates token and returns the link if valid (not expired, not revoked, entry/revision still exist). Returns 404-style error if invalid.
func (s *Service) GetByToken(ctx context.Context, token string) (*PreviewLink, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, sql.ErrNoRows
	}
	hash := hashToken(token)
	var link PreviewLink
	var revoked sql.NullInt64
	err := s.db.QueryRowContext(ctx, `SELECT id, token_hash, entry_id, revision_id, created_by, expires_at, revoked_at, created_at FROM preview_links WHERE token_hash = ?`, hash).Scan(
		&link.ID, &link.TokenHash, &link.EntryID, &link.RevisionID, &link.CreatedBy, &link.ExpiresAt, &revoked, &link.CreatedAt)
	if err != nil {
		return nil, err
	}
	link.RevokedAt = revoked
	now := time.Now().Unix()
	if link.RevokedAt.Valid {
		return nil, sql.ErrNoRows
	}
	if link.ExpiresAt <= now {
		return nil, sql.ErrNoRows
	}
	// Verify entry and revision still exist
	if _, err := s.queries.GetEntry(ctx, link.EntryID); err != nil {
		return nil, sql.ErrNoRows
	}
	if _, err := s.queries.GetEntryRevision(ctx, link.RevisionID); err != nil {
		return nil, sql.ErrNoRows
	}
	return &link, nil
}

// ListActiveByEntry returns active (not revoked, not expired) links for an entry.
func (s *Service) ListActiveByEntry(ctx context.Context, entryID string) ([]PreviewLink, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, token_hash, entry_id, revision_id, created_by, expires_at, revoked_at, created_at FROM preview_links WHERE entry_id = ? AND revoked_at IS NULL AND expires_at > ? ORDER BY created_at DESC`, entryID, time.Now().Unix())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PreviewLink
	for rows.Next() {
		var l PreviewLink
		var revoked sql.NullInt64
		if err := rows.Scan(&l.ID, &l.TokenHash, &l.EntryID, &l.RevisionID, &l.CreatedBy, &l.ExpiresAt, &revoked, &l.CreatedAt); err != nil {
			continue
		}
		l.RevokedAt = revoked
		out = append(out, l)
	}
	return out, rows.Err()
}

// ListActiveViewByEntry returns safe view models without TokenHash.
func (s *Service) ListActiveViewByEntry(ctx context.Context, entryID string) ([]PreviewLinkView, error) {
	links, err := s.ListActiveByEntry(ctx, entryID)
	if err != nil {
		return nil, err
	}
	out := make([]PreviewLinkView, 0, len(links))
	for _, l := range links {
		createdBy := ""
		if l.CreatedBy.Valid {
			createdBy = l.CreatedBy.String
		}
		out = append(out, PreviewLinkView{
			ID:         l.ID,
			RevisionID: l.RevisionID,
			CreatedAt:  l.CreatedAt,
			ExpiresAt:  l.ExpiresAt,
			CreatedBy:  createdBy,
		})
	}
	return out, nil
}

// GetByID returns a preview link by ID (for revoke ownership check).
func (s *Service) GetByID(ctx context.Context, id string) (*PreviewLink, error) {
	var l PreviewLink
	var revoked sql.NullInt64
	err := s.db.QueryRowContext(ctx, `SELECT id, token_hash, entry_id, revision_id, created_by, expires_at, revoked_at, created_at FROM preview_links WHERE id = ?`, id).Scan(
		&l.ID, &l.TokenHash, &l.EntryID, &l.RevisionID, &l.CreatedBy, &l.ExpiresAt, &revoked, &l.CreatedAt)
	if err != nil {
		return nil, err
	}
	l.RevokedAt = revoked
	return &l, nil
}

// Revoke marks a preview link as revoked. Only owner or admin can revoke? For now any authenticated can revoke via entry check.
func (s *Service) Revoke(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `UPDATE preview_links SET revoked_at = ? WHERE id = ? AND revoked_at IS NULL`, time.Now().Unix(), id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// RevokeByEntryAndUser revokes if needed? Not used now.

// hashToken returns hex SHA-256 of token.
func hashToken(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}

// ParseExpiry converts string like "1h", "24h", "7d", "30d" to duration. Also accepts "1 hour" etc for UI.
func ParseExpiry(s string) (time.Duration, error) {
	s = strings.TrimSpace(strings.ToLower(s))
	switch s {
	case "1h", "1 hour", "1hour":
		return time.Hour, nil
	case "24h", "24 hours", "24hour", "1d", "1 day", "1day":
		return 24 * time.Hour, nil
	case "7d", "7 days", "7days", "168h":
		return 7 * 24 * time.Hour, nil
	case "30d", "30 days", "30days", "720h":
		return 30 * 24 * time.Hour, nil
	default:
		return 0, fmt.Errorf("invalid expiry %q: expected 1h, 24h, 7d or 30d", s)
	}
}
