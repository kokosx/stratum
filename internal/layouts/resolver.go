package layouts

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

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
// The fingerprint is the colon-joined list of published revision IDs along the
// parent chain (root → leaf). Any change in any parent's published revision
// changes the fingerprint, so the page cache is correctly invalidated.
func ResolveEffectiveDocumentWithID(ctx context.Context, queries *db.Queries, entryDoc *document.Document, contentTypeID string, layoutTemplateID sql.NullString) (*document.Document, string, error) {
	if !layoutTemplateID.Valid || layoutTemplateID.String == "" {
		return entryDoc, "", nil
	}
	chain, err := resolveChain(ctx, queries, layoutTemplateID.String, contentTypeID)
	if err != nil {
		return nil, "", err
	}
	if len(chain) == 0 {
		return entryDoc, "", nil
	}
	// Compose bottom-up: entry into leaf, then leaf result into parent, etc.
	composed := entryDoc
	var revIDs []string
	for i := len(chain) - 1; i >= 0; i-- {
		item := chain[i]
		layoutDoc, err := document.Decode([]byte(item.DocumentJson))
		if err != nil {
			return nil, "", fmt.Errorf("decode layout template document: %w", err)
		}
		composed, err = Compose(layoutDoc, composed)
		if err != nil {
			return nil, "", fmt.Errorf("compose layout template %q: %w", item.ID, err)
		}
		revIDs = append([]string{item.RevisionID}, revIDs...)
	}
	fingerprint := strings.Join(revIDs, ":")
	return composed, fingerprint, nil
}

type chainItem struct {
	ID           string
	RevisionID   string
	DocumentJson string
}

func resolveChain(ctx context.Context, queries *db.Queries, leafID, contentTypeID string) ([]chainItem, error) {
	const maxDepth = 8
	visited := map[string]bool{}
	var chain []chainItem
	currentID := leafID
	depth := 0
	for currentID != "" {
		if visited[currentID] {
			return nil, errors.New("template nesting cycle detected")
		}
		visited[currentID] = true
		depth++
		if depth > maxDepth {
			return nil, fmt.Errorf("template nesting depth exceeds the %d level limit", maxDepth)
		}
		row, err := queries.GetLayoutTemplateWithPublishedRevision(ctx, currentID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return nil, fmt.Errorf("layout template %q not found or has no published revision", currentID)
			}
			return nil, fmt.Errorf("load layout template %q: %w", currentID, err)
		}
		if row.ContentTypeID != contentTypeID {
			return nil, fmt.Errorf("This template belongs to %s and cannot be used by a %s", row.ContentTypeID, contentTypeID)
		}
		chain = append(chain, chainItem{ID: row.ID, RevisionID: row.RevisionID, DocumentJson: row.DocumentJson})
		// Walk to parent
		tmpl, err := queries.GetLayoutTemplate(ctx, row.ID)
		if err != nil {
			break
		}
		if !tmpl.ParentTemplateID.Valid || tmpl.ParentTemplateID.String == "" {
			break
		}
		currentID = tmpl.ParentTemplateID.String
	}
	// Chain currently leaf → ... → root, but we want root → leaf for composition order.
	// Our loop appended leaf first, then its parent, etc., so chain[0] is leaf, chain[len-1] is root.
	// For fingerprint we want root first, so reverse.
	for i, j := 0, len(chain)-1; i < j; i, j = i+1, j-1 {
		chain[i], chain[j] = chain[j], chain[i]
	}
	return chain, nil
}
