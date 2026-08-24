package content

import (
	"fmt"
	"strings"
)

// EntryQuery is the declarative, plugin-safe model for fetching published entries.
// It replaces ad-hoc `ListPublishedEntriesByContentType` calls that previously
// lived in the public handler. Only a small, bounded set of fields is supported
// so the host can enforce stable prepared query shapes.
//
// The query is intentionally NOT a SQL builder. The implementation maps this
// struct to one of a few prepared statements (by content type, limit, offset,
// stable sort). A future plugin or Collection block will construct this struct,
// never raw SQL.
type EntryQuery struct {
	ContentType ContentTypeID
	Limit       int
	Offset      int
	Order       string // "published_desc" (default) or "published_asc"
	ExcludeIDs  []string
	// Cursor pagination support (optional, for future).
	Cursor string
	TermID string // optional filter for taxonomy archive
}

// Normalized returns a copy with defaults applied, limits clamped, and
// deterministic ordering so the memo key is stable.
func (q EntryQuery) Normalized() EntryQuery {
	if q.Limit <= 0 {
		q.Limit = 10
	}
	if q.Limit > 100 {
		q.Limit = 100
	}
	if q.Offset < 0 {
		q.Offset = 0
	}
	if q.Order == "" {
		q.Order = "published_desc"
	}
	q.Order = strings.ToLower(strings.TrimSpace(q.Order))
	if q.Order != "published_asc" && q.Order != "published_desc" {
		q.Order = "published_desc"
	}
	q.TermID = strings.TrimSpace(q.TermID)
	// Deduplicate and sort ExcludeIDs for stable key (sorting via map would add dep).
	uniq := make(map[string]struct{}, len(q.ExcludeIDs))
	for _, id := range q.ExcludeIDs {
		if id != "" {
			uniq[id] = struct{}{}
		}
	}
	q.ExcludeIDs = q.ExcludeIDs[:0]
	for id := range uniq {
		q.ExcludeIDs = append(q.ExcludeIDs, id)
	}
	// Deterministic by sorting.
	for i := 0; i < len(q.ExcludeIDs); i++ {
		for j := i + 1; j < len(q.ExcludeIDs); j++ {
			if q.ExcludeIDs[j] < q.ExcludeIDs[i] {
				q.ExcludeIDs[i], q.ExcludeIDs[j] = q.ExcludeIDs[j], q.ExcludeIDs[i]
			}
		}
	}
	return q
}

// CacheKey returns a stable string that can be used as a request-memo key.
// It must be deterministic for Normalized queries with the same semantics.
func (q EntryQuery) CacheKey() string {
	q = q.Normalized()
	exc := strings.Join(q.ExcludeIDs, ",")
	return fmt.Sprintf("ct=%s|lim=%d|off=%d|ord=%s|exc=%s|cur=%s|term=%s", q.ContentType, q.Limit, q.Offset, q.Order, exc, q.Cursor, q.TermID)
}

// Validate returns an error if the query is structurally invalid.
func (q EntryQuery) Validate() error {
	if strings.TrimSpace(string(q.ContentType)) == "" {
		return fmt.Errorf("content type is required")
	}
	if q.Limit < 0 || q.Limit > 100 {
		return fmt.Errorf("limit must be between 0 and 100")
	}
	if q.Offset < 0 {
		return fmt.Errorf("offset must be >= 0")
	}
	return nil
}
