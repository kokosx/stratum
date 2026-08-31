package audit

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"time"

	"github.com/kokosx/stratum/internal/authz"
	db "github.com/kokosx/stratum/internal/storage/sqlc"
)

type Service struct {
	db      *sql.DB
	queries *db.Queries
}

func New(database *sql.DB, queries *db.Queries) *Service {
	return &Service{db: database, queries: queries}
}

type Event struct {
	Action       string
	ResourceType string
	ResourceID   string
	RevisionID   string
	Metadata     map[string]any
}

// Record writes an audit event. If txQueries is non-nil, it uses that transaction's queries;
// otherwise it uses the service's queries directly. Metadata is sanitized to never include
// sensitive fields.
func (s *Service) Record(ctx context.Context, q *db.Queries, actor authz.Actor, transport string, e Event) error {
	if q == nil {
		q = s.queries
	}
	if transport == "" {
		transport = "system"
	}
	meta := sanitizeMetadata(e.Metadata)
	metaJSON, err := json.Marshal(meta)
	if err != nil {
		metaJSON = []byte("{}")
	}
	id, err := randomID()
	if err != nil {
		return err
	}
	var actorID sql.NullString
	if actor.ID != "" {
		actorID = sql.NullString{String: actor.ID, Valid: true}
	} else if actor.AgentID != "" {
		actorID = sql.NullString{String: actor.AgentID, Valid: true}
	}
	// Map ActorKind to audit enum
	kind := string(actor.Kind)
	if kind == "" {
		kind = string(authz.ActorSystem)
	}
	var resourceID sql.NullString
	if e.ResourceID != "" {
		resourceID = sql.NullString{String: e.ResourceID, Valid: true}
	}
	var revisionID sql.NullString
	if e.RevisionID != "" {
		revisionID = sql.NullString{String: e.RevisionID, Valid: true}
	}
	return q.CreateAuditEvent(ctx, db.CreateAuditEventParams{
		ID:           id,
		ActorKind:    kind,
		ActorID:      actorID,
		Transport:    transport,
		Action:       e.Action,
		ResourceType: e.ResourceType,
		ResourceID:   resourceID,
		RevisionID:   revisionID,
		MetadataJson: string(metaJSON),
		CreatedAt:    time.Now().Unix(),
	})
}

// sanitizeMetadata removes sensitive keys and ensures no raw tokens leak.
func sanitizeMetadata(m map[string]any) map[string]any {
	if m == nil {
		return map[string]any{}
	}
	out := make(map[string]any, len(m))
	for k, v := range m {
		lk := k
		// Explicit denylist: never persist these
		switch lk {
		case "token", "raw_token", "authorization", "password", "password_hash", "secret":
			continue
		}
		out[k] = v
	}
	return out
}

func randomID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
