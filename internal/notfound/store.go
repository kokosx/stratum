package notfound

import (
	"context"
	"database/sql"
	"strings"
	"sync/atomic"
	"time"

	"github.com/kokosx/stratum/internal/routing"
)

const (
	MaxPaths      = 10000
	RetentionDays = 30
	MaxLength     = 2048
)

var ignoreExact = map[string]bool{
	"/.env":         true,
	"/wp-login.php": true,
	"/xmlrpc.php":   true,
	"/.git/HEAD":    true,
}

var ignorePrefix = []string{
	"/.git/",
	"/.env",
	"/wp-",
}

func shouldIgnore(path string) bool {
	if ignoreExact[path] {
		return true
	}
	for _, p := range ignorePrefix {
		if strings.HasPrefix(path, p) {
			return true
		}
	}
	// Could add more heuristic but keep small
	return false
}

// Store aggregates 404 hits by normalized path.
type Store struct {
	db      *sql.DB
	counter atomic.Uint64 // for opportunistic cleanup
}

// nowFunc is overridable in tests for deterministic retention checks.
var nowFunc = time.Now

func New(db *sql.DB) *Store { return &Store{db: db} }

// Record increments count for path. Path must be already normalized via routing.NormalizePath and URL.Path only.
// Caller must ensure path is public 404 not admin/api/media/redirect.
func (s *Store) Record(ctx context.Context, rawPath string) error {
	if rawPath == "" {
		return nil
	}
	if len(rawPath) > MaxLength {
		rawPath = rawPath[:MaxLength]
	}
	norm := routing.NormalizePath(rawPath)
	// Privacy: don't store query, use path only
	// Already normalized; also treat path as case-sensitive
	if shouldIgnore(norm) {
		// Still optionally store but spec says suppress from default view; we choose to not store noise
		return nil
	}
	now := nowFunc().Unix()
	// Upsert
	// Use INSERT ... ON CONFLICT to aggregate
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO not_found_paths (path, hit_count, first_seen_at, last_seen_at)
		VALUES (?, 1, ?, ?)
		ON CONFLICT(path) DO UPDATE SET hit_count = hit_count + 1, last_seen_at = excluded.last_seen_at
	`, norm, now, now)
	if err != nil {
		return err
	}
	if s.counter.Add(1)%256 == 0 {
		// opportunistic cleanup, less aggressive to avoid DELETE bursts on 404 floods
		_ = s.Cleanup(ctx)
	}
	return nil
}

type Record struct {
	Path        string
	HitCount    int64
	FirstSeenAt int64
	LastSeenAt  int64
}

func (s *Store) List(ctx context.Context, limit int) ([]Record, error) {
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx, `SELECT path, hit_count, first_seen_at, last_seen_at FROM not_found_paths ORDER BY hit_count DESC, last_seen_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Record
	for rows.Next() {
		var r Record
		if err := rows.Scan(&r.Path, &r.HitCount, &r.FirstSeenAt, &r.LastSeenAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) Get(ctx context.Context, path string) (Record, error) {
	norm := routing.NormalizePath(path)
	var r Record
	err := s.db.QueryRowContext(ctx, `SELECT path, hit_count, first_seen_at, last_seen_at FROM not_found_paths WHERE path = ?`, norm).Scan(&r.Path, &r.HitCount, &r.FirstSeenAt, &r.LastSeenAt)
	return r, err
}

func (s *Store) Delete(ctx context.Context, path string) error {
	norm := routing.NormalizePath(path)
	_, err := s.db.ExecContext(ctx, `DELETE FROM not_found_paths WHERE path = ?`, norm)
	return err
}

func (s *Store) ClearAll(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM not_found_paths`)
	return err
}

// Cleanup applies retention and max size bounds.
func (s *Store) Cleanup(ctx context.Context) error {
	cutoff := nowFunc().AddDate(0, 0, -RetentionDays).Unix()
	// Delete old
	if _, err := s.db.ExecContext(ctx, `DELETE FROM not_found_paths WHERE last_seen_at < ?`, cutoff); err != nil {
		return err
	}
	// Trim excess oldest
	var count int64
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM not_found_paths`).Scan(&count); err != nil {
		return err
	}
	if count > MaxPaths {
		excess := count - MaxPaths
		// Delete oldest by last_seen_at
		_, err := s.db.ExecContext(ctx, `DELETE FROM not_found_paths WHERE path IN (SELECT path FROM not_found_paths ORDER BY last_seen_at ASC LIMIT ?)`, excess)
		return err
	}
	return nil
}

// Count returns total unique paths
func (s *Store) Count(ctx context.Context) (int64, error) {
	var c int64
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM not_found_paths`).Scan(&c)
	return c, err
}
