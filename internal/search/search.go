// Package search maintains the rebuildable SQLite FTS5 projection for public
// published entries.
package search

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"html/template"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/kokosx/stratum/internal/blocks"
	"github.com/kokosx/stratum/internal/content"
	"github.com/kokosx/stratum/internal/document"
	db "github.com/kokosx/stratum/internal/storage/sqlc"
)

const (
	pageSize      = 10
	maxPage       = 1000
	maxQueryRunes = 256
	maxTerms      = 12
)

type ContentTypeReader interface {
	GetDefinition(ctx context.Context, id string) (content.ContentTypeDefinition, error)
	ListDefinitions(ctx context.Context) ([]content.ContentTypeDefinition, error)
}

type Service struct {
	db      *sql.DB
	blocks  *blocks.Registry
	queries *db.Queries
	catalog ContentTypeReader
	// rebuildHook is a test seam to force BuildDocument failure for a specific entry.
	// If non-nil, Rebuild uses it instead of BuildDocument.
	rebuildHook func(ctx context.Context, entryID string) (Document, error)
}

type Document struct {
	EntryID          string
	ContentTypeID    string
	Title            string
	Excerpt          string
	Body             string
	Fields           string
	Path             string
	FirstPublishedAt sql.NullInt64
}

type Result struct {
	EntryID          string
	Title            string
	Excerpt          string
	Snippet          string // safe HTML containing <mark> tags with escaped fragments
	Path             string
	ContentTypeID    string
	ContentTypeLabel string
}

func New(database *sql.DB, registry *blocks.Registry) *Service {
	var queries *db.Queries
	if database != nil {
		queries = db.New(database)
	}
	return &Service{db: database, blocks: registry, queries: queries}
}

// NewWithCatalog allows tests to inject a custom catalog without a real DB.
func NewWithCatalog(database *sql.DB, registry *blocks.Registry, catalog ContentTypeReader) *Service {
	s := New(database, registry)
	s.catalog = catalog
	return s
}

func (s *Service) SetCatalog(catalog ContentTypeReader) { s.catalog = catalog }

// SetRebuildHook installs a test-only hook for Rebuild to force BuildDocument failures.
// Passing nil clears the hook.
func (s *Service) SetRebuildHook(fn func(ctx context.Context, entryID string) (Document, error)) {
	s.rebuildHook = fn
}

// BuildDocument reads precisely the current public published revision. Drafts,
// private/password revisions, trashed entries, and historical revisions cannot
// enter the projection.
func (s *Service) BuildDocument(ctx context.Context, entryID string) (Document, error) {
	var row struct {
		Document
		documentJSON string
		fieldsJSON   string
	}
	err := s.db.QueryRowContext(ctx, `
SELECT e.id, e.content_type_id, r.title, COALESCE(r.excerpt, ''), COALESCE(r.document_json, '{"version":1,"nodes":[]}'), COALESCE(r.fields_json, '{}'),
       routes.path, e.first_published_at
FROM entries e
JOIN entry_revisions r ON r.id = e.published_revision_id
JOIN routes ON routes.entry_id = e.id AND routes.route_type = 'entry'
WHERE e.id = ? AND e.status = 'active' AND r.visibility = 'public'`, entryID).Scan(
		&row.EntryID, &row.ContentTypeID, &row.Title, &row.Excerpt, &row.documentJSON,
		&row.fieldsJSON, &row.Path, &row.FirstPublishedAt,
	)
	if err != nil {
		return Document{}, err
	}
	doc, err := document.Decode([]byte(row.documentJSON))
	if err != nil {
		return Document{}, fmt.Errorf("decode published document: %w", err)
	}
	if s.blocks != nil {
		row.Body = strings.Join(s.blocks.SearchText(doc), "\n")
	}
	row.Fields = s.extractFields(ctx, row.ContentTypeID, row.fieldsJSON)
	return row.Document, nil
}

// RefreshEntry atomically replaces one derived document. A missing public
// revision means removal, which makes lifecycle callers naturally idempotent.
func (s *Service) RefreshEntry(ctx context.Context, entryID string) error {
	doc, err := s.BuildDocument(ctx, entryID)
	if err == sql.ErrNoRows {
		return s.RemoveEntry(ctx, entryID)
	}
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM search_documents_fts WHERE entry_id = ?`, entryID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM search_documents WHERE entry_id = ?`, entryID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO search_documents(entry_id, content_type_id, title, excerpt, body, fields, path, first_published_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, doc.EntryID, doc.ContentTypeID, doc.Title, doc.Excerpt, doc.Body, doc.Fields, doc.Path, doc.FirstPublishedAt); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO search_documents_fts(entry_id, title, excerpt, body, fields) VALUES (?, ?, ?, ?, ?)`, doc.EntryID, doc.Title, doc.Excerpt, doc.Body, doc.Fields); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Service) RemoveEntry(ctx context.Context, entryID string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM search_documents_fts WHERE entry_id = ?`, entryID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM search_documents WHERE entry_id = ?`, entryID); err != nil {
		return err
	}
	return tx.Commit()
}

// Rebuild clears and recreates the entire derived projection. It first
// discovers and builds all valid documents in memory, then replaces the
// projection transactionally so a failure does not leave the index empty.
func (s *Service) Rebuild(ctx context.Context) (int, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT e.id FROM entries e JOIN entry_revisions r ON r.id = e.published_revision_id JOIN routes ON routes.entry_id = e.id AND routes.route_type = 'entry' WHERE e.status = 'active' AND r.visibility = 'public'`)
	if err != nil {
		return 0, err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			_ = rows.Close()
			return 0, err
		}
		ids = append(ids, id)
	}
	if err := rows.Close(); err != nil {
		return 0, err
	}
	// Build all valid documents before touching the projection.
	// Any unexpected BuildDocument error aborts the rebuild to preserve the previous
	// valid projection. Only sql.ErrNoRows (entry no longer qualifies) is safely skipped.
	var docs []Document
	for _, id := range ids {
		var doc Document
		var err error
		if s.rebuildHook != nil {
			doc, err = s.rebuildHook(ctx, id)
		} else {
			doc, err = s.BuildDocument(ctx, id)
		}
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				continue
			}
			return 0, fmt.Errorf("build search document %s: %w", id, err)
		}
		docs = append(docs, doc)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM search_documents_fts`); err != nil {
		return 0, err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM search_documents`); err != nil {
		return 0, err
	}
	for _, doc := range docs {
		if _, err := tx.ExecContext(ctx, `INSERT INTO search_documents(entry_id, content_type_id, title, excerpt, body, fields, path, first_published_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, doc.EntryID, doc.ContentTypeID, doc.Title, doc.Excerpt, doc.Body, doc.Fields, doc.Path, doc.FirstPublishedAt); err != nil {
			return 0, err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO search_documents_fts(entry_id, title, excerpt, body, fields) VALUES (?, ?, ?, ?, ?)`, doc.EntryID, doc.Title, doc.Excerpt, doc.Body, doc.Fields); err != nil {
			return 0, err
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return len(docs), nil
}

// Query is the backward-compatible entrypoint (no type filter).
func (s *Service) Query(ctx context.Context, input string, page int) ([]Result, int, error) {
	results, total, _, err := s.QueryFiltered(ctx, input, "", page)
	return results, total, err
}

// QueryFiltered returns filtered results, total (for the filter if applied),
// and grouped counts per content type for the same query (unfiltered) for
// filter navigation. It validates typeFilter against known public types.
func (s *Service) QueryFiltered(ctx context.Context, input string, typeFilter string, page int) ([]Result, int, map[string]int, error) {
	match := safeMatch(input)
	if match == "" {
		return nil, 0, nil, nil
	}
	if page < 1 {
		page = 1
	}
	if page > maxPage {
		page = maxPage
	}
	filterID := s.validatedFilter(ctx, strings.TrimSpace(typeFilter))
	terms := parseTerms(input)

	var total int
	countSQL := `SELECT COUNT(*) FROM search_documents_fts JOIN search_documents d ON d.entry_id = search_documents_fts.entry_id WHERE search_documents_fts MATCH ?`
	countArgs := []any{match}
	if filterID != "" {
		countSQL += ` AND d.content_type_id = ?`
		countArgs = append(countArgs, filterID)
	}
	if err := s.db.QueryRowContext(ctx, countSQL, countArgs...).Scan(&total); err != nil {
		return nil, 0, nil, err
	}

	// Grouped counts for filter UI (one query, not N).
	counts := make(map[string]int)
	groupSQL := `SELECT d.content_type_id, COUNT(*) FROM search_documents_fts JOIN search_documents d ON d.entry_id = search_documents_fts.entry_id WHERE search_documents_fts MATCH ? GROUP BY d.content_type_id`
	if rows, err := s.db.QueryContext(ctx, groupSQL, match); err == nil {
		defer rows.Close()
		for rows.Next() {
			var ctype string
			var cnt int
			if err := rows.Scan(&ctype, &cnt); err == nil {
				counts[ctype] = cnt
			}
		}
		_ = rows.Err()
	}

	// Early exit if total is zero: no need to fetch rows.
	if total == 0 {
		return nil, 0, counts, nil
	}

	selectSQL := `SELECT d.entry_id, d.title, d.excerpt, d.body, d.fields, d.path, d.content_type_id, d.first_published_at FROM search_documents_fts JOIN search_documents d ON d.entry_id = search_documents_fts.entry_id WHERE search_documents_fts MATCH ?`
	selectArgs := []any{match}
	if filterID != "" {
		selectSQL += ` AND d.content_type_id = ?`
		selectArgs = append(selectArgs, filterID)
	}
	// Relevance: title 10, excerpt 5, body 1, fields 3. Lower bm25 ranks better.
	// entry_id is UNINDEXED, weight 0 is placeholder.
	selectSQL += ` ORDER BY bm25(search_documents_fts, 0.0, 10.0, 5.0, 1.0, 3.0), d.first_published_at DESC, d.entry_id ASC LIMIT ? OFFSET ?`
	selectArgs = append(selectArgs, pageSize, (page-1)*pageSize)

	rows, err := s.db.QueryContext(ctx, selectSQL, selectArgs...)
	if err != nil {
		return nil, 0, nil, err
	}
	defer rows.Close()

	labels := s.contentTypeLabels(ctx)
	var results []Result
	for rows.Next() {
		var entryID, title, excerpt, body, fields, path, ctype string
		var firstPub sql.NullInt64
		if err := rows.Scan(&entryID, &title, &excerpt, &body, &fields, &path, &ctype, &firstPub); err != nil {
			return nil, 0, nil, err
		}
		label := labels[ctype]
		if label == "" {
			label = content.DefinitionFor(ctype).Label()
		}
		snippet := buildSnippet(excerpt, body, fields, terms)
		results = append(results, Result{
			EntryID:          entryID,
			Title:            title,
			Excerpt:          excerpt,
			Snippet:          snippet,
			Path:             path,
			ContentTypeID:    ctype,
			ContentTypeLabel: label,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, 0, nil, err
	}
	return results, total, counts, nil
}

// CountDocuments returns the number of rows in the search projection.
func (s *Service) CountDocuments(ctx context.Context) (int, error) {
	var n int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM search_documents`).Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}

// CountExpectedPublicDocuments returns the number of entries that should be indexed
// (active, public, has canonical Single route). Used for health diagnostics.
func (s *Service) CountExpectedPublicDocuments(ctx context.Context) (int, error) {
	var n int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM entries e JOIN entry_revisions r ON r.id = e.published_revision_id JOIN routes ON routes.entry_id = e.id AND routes.route_type = 'entry' WHERE e.status = 'active' AND r.visibility = 'public'`).Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}

func safeMatch(input string) string {
	words := parseWords(input)
	if len(words) == 0 {
		return ""
	}
	var terms []string
	var runesUsed int
	for i, word := range words {
		wRunes := utf8.RuneCountInString(word)
		if runesUsed+wRunes > maxQueryRunes || len(terms) >= maxTerms {
			break
		}
		escaped := strings.ReplaceAll(word, `"`, `""`)
		isLast := i == len(words)-1
		// Prefix matching: safe prefix on final term if >=3 runes.
		if isLast && wRunes >= 3 && len(terms) < maxTerms {
			terms = append(terms, `"`+escaped+`"*`)
		} else {
			terms = append(terms, `"`+escaped+`"`)
		}
		runesUsed += wRunes
	}
	// If we truncated words, ensure prefix only on new last if applicable.
	// When prefix was on truncated-away term, move it to actual last.
	if len(terms) > 0 && len(words) > len(terms) {
		// Need to check if last term currently has prefix star incorrectly when original not last
		// Re-derive prefix flag based on actual last kept word.
		lastIdx := len(terms) - 1
		lastWord := words[lastIdx]
		if utf8.RuneCountInString(lastWord) >= 3 {
			esc := strings.ReplaceAll(lastWord, `"`, `""`)
			terms[lastIdx] = `"` + esc + `"*`
		}
	}
	return strings.Join(terms, " AND ")
}

func parseWords(input string) []string {
	// Split on any non-letter non-number rune.
	words := strings.FieldsFunc(input, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsNumber(r) })
	// Filter empty and apply limits similar to safeMatch but without quoting.
	var out []string
	var runesUsed int
	for _, w := range words {
		if w == "" {
			continue
		}
		wRunes := utf8.RuneCountInString(w)
		if runesUsed+wRunes > maxQueryRunes || len(out) >= maxTerms {
			break
		}
		out = append(out, w)
		runesUsed += wRunes
	}
	return out
}

func parseTerms(input string) []string { return parseWords(input) }

func utf8RuneLen(s string) int { return len([]rune(s)) }

func (s *Service) extractFields(ctx context.Context, contentTypeID, raw string) string {
	values, err := content.DecodeFieldSnapshot(raw)
	if err != nil {
		return ""
	}
	def, err := s.definitionFor(ctx, contentTypeID)
	if err != nil {
		def = content.DefinitionFor(contentTypeID)
	}
	allowed := map[content.FieldType]bool{
		content.FieldText:     true,
		content.FieldTextarea: true,
		content.FieldEmail:    true,
		content.FieldURL:      true,
		content.FieldSelect:   true,
		content.FieldNumber:   true,
	}
	var out []string
	for _, field := range def.Fields {
		if !allowed[field.Type] {
			continue
		}
		rawVal, ok := values[field.Key]
		if !ok {
			continue
		}
		var str string
		switch v := rawVal.(type) {
		case string:
			str = v
		case float64:
			// Number field may be stored as float64; treat as textual for SKU search.
			// Use minimal formatting without trailing zeros where possible.
			if v == float64(int64(v)) {
				str = fmt.Sprintf("%d", int64(v))
			} else {
				str = fmt.Sprintf("%v", v)
			}
		case int, int64, float32:
			str = fmt.Sprintf("%v", v)
		default:
			continue
		}
		if strings.TrimSpace(str) != "" {
			out = append(out, str)
		}
	}
	return strings.Join(out, "\n")
}

func (s *Service) definitionFor(ctx context.Context, id string) (content.ContentTypeDefinition, error) {
	if s.catalog != nil {
		return s.catalog.GetDefinition(ctx, id)
	}
	if s.queries != nil {
		cat := content.NewCatalog(s.queries)
		if def, err := cat.GetDefinition(ctx, id); err == nil {
			return def, nil
		}
	}
	return content.DefinitionFor(id), fmt.Errorf("definition not found, using fallback")
}

func (s *Service) contentTypeLabels(ctx context.Context) map[string]string {
	m := make(map[string]string)
	if s.catalog != nil {
		if defs, err := s.catalog.ListDefinitions(ctx); err == nil {
			for _, d := range defs {
				m[string(d.ID)] = d.Label()
			}
		}
	} else if s.queries != nil {
		cat := content.NewCatalog(s.queries)
		if defs, err := cat.ListDefinitions(ctx); err == nil {
			for _, d := range defs {
				m[string(d.ID)] = d.Label()
			}
		}
	}
	// Ensure builtins
	for _, id := range []string{"page", "post"} {
		if _, ok := m[id]; !ok {
			m[id] = content.DefinitionFor(id).Label()
		}
	}
	return m
}

func (s *Service) validatedFilter(ctx context.Context, filter string) string {
	if filter == "" {
		return ""
	}
	// Load definitions and check if filter corresponds to a public Single routable type.
	var defs []content.ContentTypeDefinition
	if s.catalog != nil {
		defs, _ = s.catalog.ListDefinitions(ctx)
	} else if s.queries != nil {
		cat := content.NewCatalog(s.queries)
		defs, _ = cat.ListDefinitions(ctx)
	}
	for _, d := range defs {
		if string(d.ID) == filter {
			if d.Routing.Single {
				return filter
			}
			return "" // data-only or non-routable -> ignore
		}
	}
	// Fallback for builtins not in DB list
	if filter == "page" || filter == "post" {
		def := content.DefinitionFor(filter)
		if def.Routing.Single {
			return filter
		}
	}
	return ""
}

// ValidateFilter exposes the internal filter validation for handlers.
func (s *Service) ValidateFilter(ctx context.Context, filter string) string {
	return s.validatedFilter(ctx, filter)
}

// buildSnippet builds a 120-220 char snippet prioritizing excerpt, then body, then fields.
func buildSnippet(excerpt, body, fields string, terms []string) string {
	source := ""
	if containsTerm(excerpt, terms) {
		source = excerpt
	} else if containsTerm(body, terms) {
		source = body
	} else if containsTerm(fields, terms) {
		source = fields
	} else if strings.TrimSpace(excerpt) != "" {
		source = excerpt
	} else if strings.TrimSpace(body) != "" {
		source = body
	} else if strings.TrimSpace(fields) != "" {
		source = fields
	} else {
		return ""
	}
	window := snippetWindow(source, terms)
	return highlightTerms(window, terms)
}

func containsTerm(text string, terms []string) bool {
	if strings.TrimSpace(text) == "" || len(terms) == 0 {
		return false
	}
	lower := strings.ToLower(text)
	for _, term := range terms {
		if term == "" {
			continue
		}
		if strings.Contains(lower, strings.ToLower(term)) {
			return true
		}
		// Also consider prefix match for last term >=3
	}
	// Check prefix of last term if applicable
	if len(terms) > 0 {
		last := terms[len(terms)-1]
		if utf8.RuneCountInString(last) >= 3 {
			lastLower := strings.ToLower(last)
			// If text contains last as prefix of any word
			words := strings.FieldsFunc(strings.ToLower(text), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsNumber(r) })
			for _, w := range words {
				if strings.HasPrefix(w, lastLower) {
					return true
				}
			}
		}
	}
	return false
}

func snippetWindow(text string, terms []string) string {
	textRunes := []rune(text)
	if len(textRunes) <= 180 {
		return text
	}
	lowerRunes := []rune(strings.ToLower(text))
	// Find earliest occurrence
	earliest := -1
	matchLen := 0
	for idx, term := range terms {
		termLower := []rune(strings.ToLower(term))
		if len(termLower) == 0 {
			continue
		}
		isPrefix := idx == len(terms)-1 && len(termLower) >= 3
		for pos := 0; pos <= len(lowerRunes)-len(termLower); pos++ {
			found := true
			for k := 0; k < len(termLower); k++ {
				if lowerRunes[pos+k] != termLower[k] {
					found = false
					break
				}
			}
			if found {
				end := pos + len(termLower)
				if isPrefix {
					// Extend to word boundary for prefix highlight length
					for end < len(lowerRunes) && (unicode.IsLetter(lowerRunes[end]) || unicode.IsNumber(lowerRunes[end])) {
						end++
					}
				}
				if earliest == -1 || pos < earliest {
					earliest = pos
					matchLen = end - pos
				}
				break
			}
		}
	}
	if earliest == -1 {
		// No term found, truncate to 200 runes
		truncated := string(textRunes[:180])
		if len(textRunes) > 180 {
			truncated += "…"
		}
		return truncated
	}
	// Build window 60 before, 140 after
	start := earliest - 60
	if start < 0 {
		start = 0
	}
	end := earliest + matchLen + 140
	if end > len(textRunes) {
		end = len(textRunes)
	}
	// Adjust to target 160-220 length where possible
	windowLen := end - start
	if windowLen < 120 && end < len(textRunes) {
		need := 120 - windowLen
		if end+need <= len(textRunes) {
			end += need
		} else {
			need2 := 120 - (end - start)
			if start-need2 >= 0 {
				start -= need2
			}
		}
	}
	if windowLen > 220 {
		end = start + 220
		if end > len(textRunes) {
			end = len(textRunes)
		}
	}
	windowRunes := textRunes[start:end]
	windowStr := string(windowRunes)
	if start > 0 {
		windowStr = "…" + strings.TrimLeft(windowStr, " \n\t")
	}
	if end < len(textRunes) {
		windowStr = strings.TrimRight(windowStr, " \n\t") + "…"
	}
	return windowStr
}

func highlightTerms(text string, terms []string) string {
	if len(terms) == 0 || strings.TrimSpace(text) == "" {
		return template.HTMLEscapeString(text)
	}
	textRunes := []rune(text)
	lowerRunes := []rune(strings.ToLower(text))
	type interval struct{ start, end int }
	var matches []interval
	for idx, term := range terms {
		termRunes := []rune(strings.ToLower(term))
		if len(termRunes) == 0 {
			continue
		}
		isPrefix := idx == len(terms)-1 && len(termRunes) >= 3
		for pos := 0; pos <= len(lowerRunes)-len(termRunes); {
			found := -1
			for j := pos; j <= len(lowerRunes)-len(termRunes); j++ {
				ok := true
				for k := 0; k < len(termRunes); k++ {
					if lowerRunes[j+k] != termRunes[k] {
						ok = false
						break
					}
				}
				if ok {
					found = j
					break
				}
			}
			if found == -1 {
				break
			}
			end := found + len(termRunes)
			if isPrefix {
				for end < len(lowerRunes) && (unicode.IsLetter(lowerRunes[end]) || unicode.IsNumber(lowerRunes[end])) {
					end++
				}
			}
			matches = append(matches, interval{found, end})
			// Avoid infinite loop on zero-length
			if end <= found {
				pos = found + 1
			} else {
				pos = end
			}
		}
	}
	if len(matches) == 0 {
		return template.HTMLEscapeString(text)
	}
	sort.Slice(matches, func(i, j int) bool {
		if matches[i].start == matches[j].start {
			return matches[i].end > matches[j].end
		}
		return matches[i].start < matches[j].start
	})
	// Merge overlapping intervals, prefer longer
	var merged []interval
	for _, m := range matches {
		if len(merged) == 0 {
			merged = append(merged, m)
			continue
		}
		last := &merged[len(merged)-1]
		if m.start < last.end {
			if m.end > last.end {
				last.end = m.end
			}
			continue
		}
		if m.start == last.start && m.end <= last.end {
			continue
		}
		if m.start < last.end {
			continue
		}
		merged = append(merged, m)
	}
	var b strings.Builder
	last := 0
	for _, m := range merged {
		if m.start < last {
			continue
		}
		if m.start > len(textRunes) || m.end > len(textRunes) {
			continue
		}
		b.WriteString(template.HTMLEscapeString(string(textRunes[last:m.start])))
		b.WriteString("<mark>")
		b.WriteString(template.HTMLEscapeString(string(textRunes[m.start:m.end])))
		b.WriteString("</mark>")
		last = m.end
	}
	b.WriteString(template.HTMLEscapeString(string(textRunes[last:])))
	return b.String()
}
