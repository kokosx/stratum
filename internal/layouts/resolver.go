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
// Otherwise it loads the published layout template revision, verifies content type,
// decodes and composes.
func ResolveEffectiveDocument(ctx context.Context, queries *db.Queries, entryDoc *document.Document, contentTypeID string, layoutTemplateID sql.NullString) (*document.Document, error) {
	if !layoutTemplateID.Valid || layoutTemplateID.String == "" {
		return entryDoc, nil
	}

	// Prefer single query that joins template + published revision.
	row, err := queries.GetLayoutTemplateWithPublishedRevision(ctx, layoutTemplateID.String)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("layout template %q not found or has no published revision", layoutTemplateID.String)
		}
		return nil, fmt.Errorf("load layout template %q: %w", layoutTemplateID.String, err)
	}
	if row.ContentTypeID != contentTypeID {
		return nil, fmt.Errorf("This template belongs to %s and cannot be used by a %s", row.ContentTypeID, contentTypeID)
	}
	// PublishedRevisionID is guaranteed non-null because join succeeded, but check.
	if !row.PublishedRevisionID.Valid || row.PublishedRevisionID.String == "" {
		return nil, fmt.Errorf("The selected layout template has not been published yet")
	}
	layoutDoc, err := document.Decode([]byte(row.DocumentJson))
	if err != nil {
		return nil, fmt.Errorf("decode layout template document: %w", err)
	}
	composed, err := Compose(layoutDoc, entryDoc)
	if err != nil {
		return nil, fmt.Errorf("compose layout template: %w", err)
	}
	return composed, nil
}

// ResolveEffectiveDocumentWithID also returns the published layout revision ID for caching.
func ResolveEffectiveDocumentWithID(ctx context.Context, queries *db.Queries, entryDoc *document.Document, contentTypeID string, layoutTemplateID sql.NullString) (*document.Document, string, error) {
	if !layoutTemplateID.Valid || layoutTemplateID.String == "" {
		return entryDoc, "", nil
	}
	row, err := queries.GetLayoutTemplateWithPublishedRevision(ctx, layoutTemplateID.String)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, "", fmt.Errorf("layout template %q not found or has no published revision", layoutTemplateID.String)
		}
		return nil, "", fmt.Errorf("load layout template %q: %w", layoutTemplateID.String, err)
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
		return nil, "", fmt.Errorf("compose layout template: %w", err)
	}
	return composed, row.RevisionID, nil
}
