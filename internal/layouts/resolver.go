package layouts

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/kokosx/stratum/internal/document"
	db "github.com/kokosx/stratum/internal/storage/sqlc"
)

// ResolveEffectiveDocument returns the document to render for an entry.
// If layoutTemplateID is null/empty it returns entryDoc directly.
func ResolveEffectiveDocument(ctx context.Context, queries *db.Queries, entryDoc *document.Document, contentTypeID string, layoutTemplateID sql.NullString) (*document.Document, error) {
	doc, _, err := ResolveEffectiveDocumentWithID(ctx, queries, entryDoc, contentTypeID, layoutTemplateID)
	return doc, err
}

// ResolveEffectiveDocumentWithID also returns a deterministic fingerprint for caching.
// The fingerprint is the published revision ID of the layout template.
// Kept for backward compat – prefers ResolveSingle.
func ResolveEffectiveDocumentWithID(ctx context.Context, queries *db.Queries, entryDoc *document.Document, contentTypeID string, layoutTemplateID sql.NullString) (*document.Document, string, error) {
	return ResolveSingleWithID(ctx, queries, entryDoc, contentTypeID, layoutTemplateID)
}

// ResolvedTemplate is the result of template resolution.
type ResolvedTemplate struct {
	TemplateID string
	RevisionID string
	Document   *document.Document
}

// ResolveSingle resolves the single template for an entry, composing entry content if needed.
func ResolveSingle(ctx context.Context, queries *db.Queries, entryDoc *document.Document, contentTypeID string, layoutTemplateID sql.NullString) (*document.Document, error) {
	doc, _, err := ResolveSingleWithID(ctx, queries, entryDoc, contentTypeID, layoutTemplateID)
	return doc, err
}

// ResolveSingleWithID also returns fingerprint.
func ResolveSingleWithID(ctx context.Context, queries *db.Queries, entryDoc *document.Document, contentTypeID string, layoutTemplateID sql.NullString) (*document.Document, string, error) {
	if !layoutTemplateID.Valid || layoutTemplateID.String == "" {
		if contentTypeID != "" {
			if ct, err := queries.GetContentType(ctx, contentTypeID); err == nil && ct.DefaultLayoutTemplateID.Valid && ct.DefaultLayoutTemplateID.String != "" {
				layoutTemplateID = ct.DefaultLayoutTemplateID
			} else {
				return entryDoc, "", nil
			}
		} else {
			return entryDoc, "", nil
		}
		if !layoutTemplateID.Valid || layoutTemplateID.String == "" {
			return entryDoc, "", nil
		}
	}
	row, err := queries.GetLayoutTemplateWithPublishedRevision(ctx, layoutTemplateID.String)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, "", fmt.Errorf("layout template %q not found or has no published revision", layoutTemplateID.String)
		}
		return nil, "", fmt.Errorf("load layout template %q: %w", layoutTemplateID.String, err)
	}
	if row.Kind != "single" {
		return nil, "", fmt.Errorf("template %q is not a Single template", layoutTemplateID.String)
	}
	if row.ContentTypeID != contentTypeID {
		return nil, "", fmt.Errorf("This template belongs to %s and cannot be used by a %s", row.ContentTypeID, contentTypeID)
	}
	layoutDoc, err := document.Decode([]byte(row.DocumentJson))
	if err != nil {
		return nil, "", fmt.Errorf("decode layout template document: %w", err)
	}
	composed, err := Compose(layoutDoc, entryDoc)
	if err != nil {
		return nil, "", fmt.Errorf("compose layout template %q: %w", row.ID, err)
	}
	return composed, row.RevisionID, nil
}

// ResolveArchive resolves the archive template for a content type.
// Returns nil if no published archive template assigned (caller should fallback).
func ResolveArchive(ctx context.Context, queries *db.Queries, contentTypeID string) (*ResolvedTemplate, error) {
	if contentTypeID == "" {
		return nil, nil
	}
	ct, err := queries.GetContentType(ctx, contentTypeID)
	if err != nil {
		return nil, nil
	}
	if !ct.DefaultArchiveTemplateID.Valid || ct.DefaultArchiveTemplateID.String == "" {
		return nil, nil
	}
	row, err := queries.GetLayoutTemplateWithPublishedRevision(ctx, ct.DefaultArchiveTemplateID.String)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			// Template exists but not published – treat as no template (do not leak draft)
			return nil, nil
		}
		return nil, fmt.Errorf("load archive template %q: %w", ct.DefaultArchiveTemplateID.String, err)
	}
	if row.Kind != "archive" {
		return nil, fmt.Errorf("template %q is not an Archive template", ct.DefaultArchiveTemplateID.String)
	}
	if row.ContentTypeID != contentTypeID {
		return nil, fmt.Errorf("archive template %q belongs to %s, not %s", row.ID, row.ContentTypeID, contentTypeID)
	}
	doc, err := document.Decode([]byte(row.DocumentJson))
	if err != nil {
		return nil, fmt.Errorf("decode archive template: %w", err)
	}
	return &ResolvedTemplate{TemplateID: row.ID, RevisionID: row.RevisionID, Document: doc}, nil
}
