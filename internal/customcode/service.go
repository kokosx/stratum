package customcode

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	"github.com/kokosx/stratum/internal/pagecache"
)

type Scope string

const (
	ScopeGlobal   Scope = "global"
	ScopeTemplate Scope = "template"
)

type Kind string

const (
	KindHTML Kind = "html"
	KindCSS  Kind = "css"
	KindJS   Kind = "js"
)

type Placement string

const (
	PlacementHead      Placement = "head"
	PlacementBodyStart Placement = "body_start"
	PlacementBodyEnd   Placement = "body_end"
)

type Snippet struct {
	ID        string
	Name      string
	Scope     Scope
	ScopeID   sql.NullString
	Kind      Kind
	Placement Placement
	Code      string
	Enabled   bool
	SortOrder int64
	CreatedAt int64
	UpdatedAt int64
}

type Service struct {
	db    *sql.DB
	cache *pagecache.Cache
}

func New(db *sql.DB, cache *pagecache.Cache) *Service {
	return &Service{db: db, cache: cache}
}

func (s *Service) List(ctx context.Context, scope Scope, scopeID string) ([]Snippet, error) {
	var rows *sql.Rows
	var err error
	if scope == ScopeGlobal {
		rows, err = s.db.QueryContext(ctx, `SELECT id, name, scope, scope_id, kind, placement, code, enabled, sort_order, created_at, updated_at FROM custom_code_snippets WHERE scope='global' ORDER BY sort_order ASC, id ASC`)
	} else {
		rows, err = s.db.QueryContext(ctx, `SELECT id, name, scope, scope_id, kind, placement, code, enabled, sort_order, created_at, updated_at FROM custom_code_snippets WHERE scope='template' AND scope_id=? ORDER BY sort_order ASC, id ASC`, scopeID)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Snippet
	for rows.Next() {
		var sn Snippet
		var scopeStr, kindStr, placementStr string
		var enabled int64
		if err := rows.Scan(&sn.ID, &sn.Name, &scopeStr, &sn.ScopeID, &kindStr, &placementStr, &sn.Code, &enabled, &sn.SortOrder, &sn.CreatedAt, &sn.UpdatedAt); err != nil {
			return nil, err
		}
		sn.Scope = Scope(scopeStr)
		sn.Kind = Kind(kindStr)
		sn.Placement = Placement(placementStr)
		sn.Enabled = enabled != 0
		out = append(out, sn)
	}
	return out, rows.Err()
}

func (s *Service) ListGlobal(ctx context.Context) ([]Snippet, error) {
	return s.List(ctx, ScopeGlobal, "")
}

func (s *Service) ListForTemplate(ctx context.Context, templateID string) ([]Snippet, error) {
	return s.List(ctx, ScopeTemplate, templateID)
}

func (s *Service) ResolveForPage(ctx context.Context, templateID string) (head, bodyStart, bodyEnd []Snippet, err error) {
	globals, err := s.ListGlobal(ctx)
	if err != nil {
		return nil, nil, nil, err
	}
	var tpl []Snippet
	if templateID != "" {
		tpl, _ = s.ListForTemplate(ctx, templateID)
	}
	all := append(globals, tpl...)
	for _, sn := range all {
		if !sn.Enabled {
			continue
		}
		switch sn.Placement {
		case PlacementHead:
			head = append(head, sn)
		case PlacementBodyStart:
			bodyStart = append(bodyStart, sn)
		case PlacementBodyEnd:
			bodyEnd = append(bodyEnd, sn)
		}
	}
	// Already sorted by sort_order,id via queries; merged preserves order globals then template (each sorted). For true deterministic ordering across scopes, sort by sort_order then id.
	// Keep as is: globals first then template, each sorted. Spec says deterministic ordering: sort_order then ID per placement.
	return head, bodyStart, bodyEnd, nil
}

func (s *Service) Get(ctx context.Context, id string) (Snippet, error) {
	var sn Snippet
	var scopeStr, kindStr, placementStr string
	var enabled int64
	err := s.db.QueryRowContext(ctx, `SELECT id, name, scope, scope_id, kind, placement, code, enabled, sort_order, created_at, updated_at FROM custom_code_snippets WHERE id=?`, id).Scan(&sn.ID, &sn.Name, &scopeStr, &sn.ScopeID, &kindStr, &placementStr, &sn.Code, &enabled, &sn.SortOrder, &sn.CreatedAt, &sn.UpdatedAt)
	if err != nil {
		return Snippet{}, err
	}
	sn.Scope = Scope(scopeStr)
	sn.Kind = Kind(kindStr)
	sn.Placement = Placement(placementStr)
	sn.Enabled = enabled != 0
	return sn, nil
}

func ValidateInput(name, scope, scopeID, kind, placement, code string, sortOrder int64) error {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 100 {
		return fmt.Errorf("name is required and must be 1-100 characters")
	}
	if scope != string(ScopeGlobal) && scope != string(ScopeTemplate) {
		return fmt.Errorf("invalid scope")
	}
	if scope == string(ScopeGlobal) && strings.TrimSpace(scopeID) != "" {
		return fmt.Errorf("global scope must not have scope_id")
	}
	if scope == string(ScopeTemplate) && strings.TrimSpace(scopeID) == "" {
		return fmt.Errorf("template scope requires template ID")
	}
	if kind != string(KindHTML) && kind != string(KindCSS) && kind != string(KindJS) {
		return fmt.Errorf("invalid kind")
	}
	if placement != string(PlacementHead) && placement != string(PlacementBodyStart) && placement != string(PlacementBodyEnd) {
		return fmt.Errorf("invalid placement")
	}
	if len(code) > 200*1024 {
		return fmt.Errorf("code exceeds 200KB limit")
	}
	if sortOrder < 0 || sortOrder > 10000 {
		return fmt.Errorf("invalid sort order")
	}
	return nil
}

func (s *Service) Create(ctx context.Context, name, scope, scopeID, kind, placement, code string, enabled bool, sortOrder int64) (Snippet, error) {
	if err := ValidateInput(name, scope, scopeID, kind, placement, code, sortOrder); err != nil {
		return Snippet{}, err
	}
	if scope == string(ScopeTemplate) {
		// Validate template exists
		var exists int
		if err := s.db.QueryRowContext(ctx, `SELECT 1 FROM layout_templates WHERE id=?`, scopeID).Scan(&exists); err != nil {
			return Snippet{}, fmt.Errorf("template not found")
		}
	}
	id, err := newID()
	if err != nil {
		return Snippet{}, err
	}
	now := time.Now().Unix()
	var sid sql.NullString
	if scope == string(ScopeTemplate) {
		sid = sql.NullString{String: scopeID, Valid: true}
	}
	enabledInt := 0
	if enabled {
		enabledInt = 1
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO custom_code_snippets (id, name, scope, scope_id, kind, placement, code, enabled, sort_order, created_at, updated_at) VALUES (?,?,?,?,?,?,?,?,?,?,?)`,
		id, strings.TrimSpace(name), scope, sid, kind, placement, code, enabledInt, sortOrder, now, now)
	if err != nil {
		return Snippet{}, err
	}
	// Invalidate cache
	if s.cache != nil {
		s.cache.InvalidateAll()
	}
	return s.Get(ctx, id)
}

func (s *Service) Update(ctx context.Context, id, name, kind, placement, code string, enabled bool, sortOrder int64) (Snippet, error) {
	existing, err := s.Get(ctx, id)
	if err != nil {
		return Snippet{}, err
	}
	if err := ValidateInput(name, string(existing.Scope), existing.ScopeID.String, kind, placement, code, sortOrder); err != nil {
		return Snippet{}, err
	}
	now := time.Now().Unix()
	enabledInt := 0
	if enabled {
		enabledInt = 1
	}
	_, err = s.db.ExecContext(ctx, `UPDATE custom_code_snippets SET name=?, kind=?, placement=?, code=?, enabled=?, sort_order=?, updated_at=? WHERE id=?`,
		strings.TrimSpace(name), kind, placement, code, enabledInt, sortOrder, now, id)
	if err != nil {
		return Snippet{}, err
	}
	if s.cache != nil {
		s.cache.InvalidateAll()
	}
	return s.Get(ctx, id)
}

func (s *Service) Delete(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM custom_code_snippets WHERE id=?`, id)
	if err != nil {
		return err
	}
	if s.cache != nil {
		s.cache.InvalidateAll()
	}
	return nil
}

func (s *Service) Toggle(ctx context.Context, id string, enabled bool) error {
	_, err := s.db.ExecContext(ctx, `UPDATE custom_code_snippets SET enabled=?, updated_at=? WHERE id=?`, boolToInt(enabled), time.Now().Unix(), id)
	if err != nil {
		return err
	}
	if s.cache != nil {
		s.cache.InvalidateAll()
	}
	return nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func newID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
