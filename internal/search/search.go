// Package search maintains the rebuildable SQLite FTS5 projection for public
// published entries.
package search

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"unicode"

	"github.com/kokosx/stratum/internal/blocks"
	"github.com/kokosx/stratum/internal/content"
	"github.com/kokosx/stratum/internal/document"
)

const (
	pageSize      = 10
	maxPage       = 1000
	maxQueryRunes = 256
	maxTerms      = 12
)

type Service struct {
	db     *sql.DB
	blocks *blocks.Registry
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
	EntryID string
	Title   string
	Excerpt string
	Path    string
}

func New(database *sql.DB, registry *blocks.Registry) *Service {
	return &Service{db: database, blocks: registry}
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
SELECT e.id, e.content_type_id, r.title, r.excerpt, r.document_json, r.fields_json,
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
	row.Fields = textFields(row.ContentTypeID, row.fieldsJSON)
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

// Rebuild clears and recreates the entire derived projection.
func (s *Service) Rebuild(ctx context.Context) (int, error) {
	if _, err := s.db.ExecContext(ctx, `DELETE FROM search_documents_fts; DELETE FROM search_documents`); err != nil {
		return 0, err
	}
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
	for i, id := range ids {
		if err := s.RefreshEntry(ctx, id); err != nil {
			return i, err
		}
	}
	return len(ids), nil
}

// Query accepts user words, not FTS expression syntax. Every normalized word is
// quoted so punctuation, operators, and quotes cannot alter query semantics.
func (s *Service) Query(ctx context.Context, input string, page int) ([]Result, int, error) {
	match := safeMatch(input)
	if match == "" {
		return nil, 0, nil
	}
	if page < 1 {
		page = 1
	}
	if page > maxPage {
		page = maxPage
	}
	var total int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM search_documents_fts WHERE search_documents_fts MATCH ?`, match).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT d.entry_id, d.title, d.excerpt, d.path FROM search_documents_fts JOIN search_documents d ON d.entry_id = search_documents_fts.entry_id WHERE search_documents_fts MATCH ? ORDER BY bm25(search_documents_fts, 10.0, 4.0, 1.0, 2.0), d.first_published_at DESC, d.entry_id ASC LIMIT ? OFFSET ?`, match, pageSize, (page-1)*pageSize)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var result []Result
	for rows.Next() {
		var item Result
		if err := rows.Scan(&item.EntryID, &item.Title, &item.Excerpt, &item.Path); err != nil {
			return nil, 0, err
		}
		result = append(result, item)
	}
	return result, total, rows.Err()
}

func safeMatch(input string) string {
	words := strings.FieldsFunc(input, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsNumber(r) })
	var terms []string
	for _, word := range words {
		if utf8RuneLen(strings.Join(terms, ""))+utf8RuneLen(word) > maxQueryRunes || len(terms) == maxTerms {
			break
		}
		terms = append(terms, `"`+strings.ReplaceAll(word, `"`, `""`)+`"`)
	}
	return strings.Join(terms, " AND ")
}

func utf8RuneLen(s string) int { return len([]rune(s)) }

func textFields(contentTypeID, raw string) string {
	values, err := content.DecodeFieldSnapshot(raw)
	if err != nil {
		return ""
	}
	allowed := map[content.FieldType]bool{content.FieldText: true, content.FieldTextarea: true, content.FieldEmail: true, content.FieldURL: true, content.FieldSelect: true}
	var out []string
	for _, field := range content.DefinitionFor(contentTypeID).Fields {
		if !allowed[field.Type] {
			continue
		}
		if value, ok := values[field.Key].(string); ok {
			out = append(out, value)
		}
	}
	return strings.Join(out, "\n")
}
