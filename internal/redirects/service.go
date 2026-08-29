package redirects

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"net/url"
	"strings"

	"github.com/kokosx/stratum/internal/routing"
	db "github.com/kokosx/stratum/internal/storage/sqlc"
)

const MaxPathLength = 2048

var (
	ErrInvalidSource  = errors.New("source must be a local absolute path")
	ErrInvalidTarget  = errors.New("target must be an internal path or http(s) URL")
	ErrSelfRedirect   = errors.New("source and target must be different")
	ErrLoop           = errors.New("this redirect would create a loop")
	ErrSourceConflict = errors.New("source path is already in use")
	ErrReservedSource = errors.New("source path is reserved")
)

func randomID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// NormalizeSource validates and normalizes a source path. Returns normalized path or error.
func NormalizeSource(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", ErrInvalidSource
	}
	if len(raw) > MaxPathLength {
		return "", ErrInvalidSource
	}
	if strings.Contains(raw, "\\") {
		return "", ErrInvalidSource
	}
	if strings.Contains(raw, "\n") || strings.Contains(raw, "\r") || strings.Contains(raw, "\x00") {
		return "", ErrInvalidSource
	}
	// Must be local absolute path
	if !strings.HasPrefix(raw, "/") {
		return "", ErrInvalidSource
	}
	if strings.HasPrefix(raw, "//") {
		return "", ErrInvalidSource
	}
	if strings.HasPrefix(strings.ToLower(raw), "javascript:") || strings.HasPrefix(strings.ToLower(raw), "data:") || strings.HasPrefix(strings.ToLower(raw), "vbscript:") {
		return "", ErrInvalidSource
	}
	// Use routing policy normalization (trailing slash)
	norm := routing.NormalizePath(raw)
	// Additional checks after normalization
	if norm != raw && raw != "/" {
		// Allow normalization; caller will store normalized
	}
	// Reject control chars
	for _, ch := range norm {
		if ch < 0x20 {
			return "", ErrInvalidSource
		}
	}
	// Reject reserved prefixes
	for _, prefix := range []string{"/admin", "/_stratum", "/media"} {
		if norm == prefix || strings.HasPrefix(norm, prefix+"/") {
			return "", ErrReservedSource
		}
	}
	for _, exact := range []string{"/sitemap.xml", "/robots.txt", "/feed.xml", "/favicon.ico"} {
		if norm == exact {
			return "", ErrReservedSource
		}
	}
	return norm, nil
}

// NormalizeTarget validates target; may be internal path or external http(s) URL. Returns normalized target or error.
func NormalizeTarget(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", ErrInvalidTarget
	}
	if len(raw) > MaxPathLength+1024 {
		return "", ErrInvalidTarget
	}
	if strings.Contains(raw, "\n") || strings.Contains(raw, "\r") || strings.Contains(raw, "\x00") {
		return "", ErrInvalidTarget
	}
	lower := strings.ToLower(raw)
	if strings.HasPrefix(lower, "javascript:") || strings.HasPrefix(lower, "data:") || strings.HasPrefix(lower, "vbscript:") {
		return "", ErrInvalidTarget
	}
	if strings.HasPrefix(raw, "//") {
		return "", ErrInvalidTarget
	}
	if strings.HasPrefix(raw, "/") {
		// internal path
		if strings.HasPrefix(raw, "//") {
			return "", ErrInvalidTarget
		}
		norm := routing.NormalizePath(raw)
		return norm, nil
	}
	// external URL
	u, err := url.Parse(raw)
	if err != nil {
		return "", ErrInvalidTarget
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", ErrInvalidTarget
	}
	if u.Host == "" {
		return "", ErrInvalidTarget
	}
	// Reject CRLF in URL
	if strings.Contains(raw, "\n") || strings.Contains(raw, "\r") {
		return "", ErrInvalidTarget
	}
	return raw, nil
}

func statusCodeValid(code int) bool { return code == 301 || code == 302 }

// Service handles manual redirect CRUD using the routes table.
type Service struct {
	db      *sql.DB
	queries *db.Queries
}

func New(database *sql.DB, queries *db.Queries) *Service {
	return &Service{db: database, queries: queries}
}

// List returns all redirect routes ordered by path.
func (s *Service) List(ctx context.Context) ([]db.Route, error) {
	rows, err := s.queries.ListRoutes(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]db.Route, 0)
	for _, r := range rows {
		if r.RouteType == routing.RouteTypeRedirect {
			out = append(out, r)
		}
	}
	return out, nil
}

// Get returns redirect by ID (must be redirect type).
func (s *Service) Get(ctx context.Context, id string) (db.Route, error) {
	// We need to fetch by path not id? Use List and filter or direct query. Use GetRouteByPath alternative not suitable.
	// Fallback: List and find.
	rows, err := s.queries.ListRoutes(ctx)
	if err != nil {
		return db.Route{}, err
	}
	for _, r := range rows {
		if r.ID == id && r.RouteType == routing.RouteTypeRedirect {
			return r, nil
		}
	}
	return db.Route{}, sql.ErrNoRows
}

func (s *Service) GetByPath(ctx context.Context, path string) (db.Route, error) {
	norm, err := NormalizeSource(path)
	if err != nil {
		return db.Route{}, err
	}
	route, err := s.queries.GetRouteByPath(ctx, norm)
	if err != nil {
		return db.Route{}, err
	}
	if route.RouteType != routing.RouteTypeRedirect {
		return db.Route{}, sql.ErrNoRows
	}
	return route, nil
}

// Create validates and inserts a redirect.
func (s *Service) Create(ctx context.Context, source, target string, status int, now int64) (db.Route, error) {
	source, err := NormalizeSource(source)
	if err != nil {
		return db.Route{}, err
	}
	target, err = NormalizeTarget(target)
	if err != nil {
		return db.Route{}, err
	}
	if !statusCodeValid(status) {
		status = 301
	}
	if source == target {
		return db.Route{}, ErrSelfRedirect
	}
	// Check live route conflict
	if live, err := s.queries.GetRouteByPath(ctx, source); err == nil {
		if live.RouteType == routing.RouteTypeEntry || live.RouteType == routing.RouteTypeArchive {
			return db.Route{}, errors.New("a published route already uses " + source)
		}
		if live.RouteType == routing.RouteTypeRedirect {
			return db.Route{}, errors.New("a redirect already starts at " + source)
		}
		// system etc. treat as conflict if not redirect
		return db.Route{}, ErrSourceConflict
	} else if !errors.Is(err, sql.ErrNoRows) {
		return db.Route{}, err
	}
	// Loop detection
	if err := s.checkLoop(ctx, source, target); err != nil {
		return db.Route{}, err
	}
	id, err := randomID()
	if err != nil {
		return db.Route{}, err
	}
	err = s.queries.CreateRoute(ctx, db.CreateRouteParams{
		ID: id, Path: source, RouteType: routing.RouteTypeRedirect,
		RedirectTo:     sql.NullString{String: target, Valid: true},
		RedirectStatus: sql.NullInt64{Int64: int64(status), Valid: true},
		CreatedAt:      now, UpdatedAt: now,
	})
	if err != nil {
		return db.Route{}, err
	}
	return s.queries.GetRouteByPath(ctx, source)
}

// Update edits an existing redirect (ID stable, source may change).
func (s *Service) Update(ctx context.Context, id, source, target string, status int, now int64) (db.Route, error) {
	source, err := NormalizeSource(source)
	if err != nil {
		return db.Route{}, err
	}
	target, err = NormalizeTarget(target)
	if err != nil {
		return db.Route{}, err
	}
	if !statusCodeValid(status) {
		status = 301
	}
	if source == target {
		return db.Route{}, ErrSelfRedirect
	}
	existing, err := s.Get(ctx, id)
	if err != nil {
		return db.Route{}, err
	}
	// If source changes, check new source not occupied
	if existing.Path != source {
		if live, err := s.queries.GetRouteByPath(ctx, source); err == nil {
			// If live is the same redirect we're updating (should not happen because path changed)
			if live.ID != id {
				if live.RouteType == routing.RouteTypeEntry || live.RouteType == routing.RouteTypeArchive {
					return db.Route{}, errors.New("a published route already uses " + source)
				}
				if live.RouteType == routing.RouteTypeRedirect {
					return db.Route{}, errors.New("a redirect already starts at " + source)
				}
				return db.Route{}, ErrSourceConflict
			}
		} else if !errors.Is(err, sql.ErrNoRows) {
			return db.Route{}, err
		}
	}
	// Loop detection with updated graph (temporarily exclude old edge if source changes)
	if err := s.checkLoopExcluding(ctx, source, target, id); err != nil {
		return db.Route{}, err
	}
	// If source changed, we need to delete old path and create new? Actually UpdateRoute changes path.
	// Use transaction
	err = s.queries.UpdateRoute(ctx, db.UpdateRouteParams{
		ID: existing.ID, Path: source, RouteType: routing.RouteTypeRedirect,
		RedirectTo:     sql.NullString{String: target, Valid: true},
		RedirectStatus: sql.NullInt64{Int64: int64(status), Valid: true},
		UpdatedAt:      now,
	})
	if err != nil {
		return db.Route{}, err
	}
	return s.queries.GetRouteByPath(ctx, source)
}

// Delete removes redirect by ID.
func (s *Service) Delete(ctx context.Context, id string) error {
	existing, err := s.Get(ctx, id)
	if err != nil {
		return err
	}
	return s.queries.DeleteRoute(ctx, existing.ID)
}

// checkLoop verifies source->target does not create a cycle. Uses in-memory graph traversal with max depth 32.
func (s *Service) checkLoop(ctx context.Context, source, target string) error {
	return s.checkLoopExcluding(ctx, source, target, "")
}

func (s *Service) checkLoopExcluding(ctx context.Context, source, target, excludeID string) error {
	// Only internal targets can form loops (external cannot loop back to internal source via redirects)
	if !strings.HasPrefix(target, "/") {
		return nil
	}
	// Build map from redirect
	rows, err := s.queries.ListRoutes(ctx)
	if err != nil {
		return err
	}
	graph := make(map[string]string)
	for _, r := range rows {
		if r.RouteType != routing.RouteTypeRedirect {
			continue
		}
		if r.ID == excludeID {
			continue
		}
		if r.RedirectTo.Valid {
			graph[r.Path] = r.RedirectTo.String
		}
	}
	// Add proposed edge
	graph[source] = target
	// Traverse from source following graph, detect cycle
	seen := map[string]bool{}
	cur := source
	for i := 0; i < 32; i++ {
		next, ok := graph[cur]
		if !ok {
			return nil
		}
		// Only follow internal paths
		if !strings.HasPrefix(next, "/") {
			return nil
		}
		if seen[next] {
			return ErrLoop
		}
		seen[next] = true
		if next == source {
			return ErrLoop
		}
		cur = next
		// Early exit if reaches a path that is not a redirect source
		if _, ok := graph[cur]; !ok {
			return nil
		}
	}
	return ErrLoop // max hops exceeded treated as loop
}

// DetectChains returns chains as slices of paths: e.g., /a -> /b -> /c
func (s *Service) DetectChains(ctx context.Context) ([][]string, error) {
	rows, err := s.queries.ListRoutes(ctx)
	if err != nil {
		return nil, err
	}
	graph := make(map[string]string)
	for _, r := range rows {
		if r.RouteType == routing.RouteTypeRedirect && r.RedirectTo.Valid && strings.HasPrefix(r.RedirectTo.String, "/") {
			graph[r.Path] = r.RedirectTo.String
		}
	}
	visited := map[string]bool{}
	var chains [][]string
	for start := range graph {
		if visited[start] {
			continue
		}
		// Only start chains where start is not a target of another redirect (head)
		isTarget := false
		for _, t := range graph {
			if t == start {
				isTarget = true
				break
			}
		}
		if isTarget {
			continue
		}
		chain := []string{start}
		cur := start
		seen := map[string]bool{start: true}
		for i := 0; i < 32; i++ {
			next, ok := graph[cur]
			if !ok {
				break
			}
			if seen[next] {
				break
			}
			chain = append(chain, next)
			seen[next] = true
			cur = next
			if _, hasOutgoing := graph[cur]; !hasOutgoing {
				break
			}
		}
		if len(chain) >= 3 {
			// Need at least 2 hops (3 nodes) to be a chain
			// Actually spec example /a -> /b -> /c is 3 nodes
			// We report chains with at least 2 edges
			chains = append(chains, chain)
			for _, p := range chain {
				visited[p] = true
			}
		} else if len(chain) == 2 {
			// Two nodes is single redirect, not chain; but we should still not report
			visited[start] = true
		}
	}
	return chains, nil
}

// DetectLoops returns loops as slices (for health)
func (s *Service) DetectLoops(ctx context.Context) ([][]string, error) {
	rows, err := s.queries.ListRoutes(ctx)
	if err != nil {
		return nil, err
	}
	graph := make(map[string]string)
	for _, r := range rows {
		if r.RouteType == routing.RouteTypeRedirect && r.RedirectTo.Valid && strings.HasPrefix(r.RedirectTo.String, "/") {
			graph[r.Path] = r.RedirectTo.String
		}
	}
	var loops [][]string
	visited := map[string]bool{}
	for node := range graph {
		if visited[node] {
			continue
		}
		// Floyd or DFS for each node
		path := []string{}
		seenIdx := map[string]int{}
		cur := node
		for i := 0; i < 32; i++ {
			if idx, ok := seenIdx[cur]; ok {
				// found loop
				loop := append([]string{}, path[idx:]...)
				loop = append(loop, cur)
				loops = append(loops, loop)
				break
			}
			seenIdx[cur] = len(path)
			path = append(path, cur)
			visited[cur] = true
			next, ok := graph[cur]
			if !ok || !strings.HasPrefix(next, "/") {
				break
			}
			cur = next
		}
	}
	return loops, nil
}
