package entryops

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/kokosx/stratum/internal/authz"
	"github.com/kokosx/stratum/internal/audit"
	"github.com/kokosx/stratum/internal/content"
	db "github.com/kokosx/stratum/internal/storage/sqlc"
)

// Trash soft-deletes an entry. Requires entries.trash scoped permission.
func (s *Service) Trash(ctx context.Context, actor authz.Actor, entryID string) error {
	entry, err := s.queries.GetEntry(ctx, entryID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("%w: entry not found", ErrNotFound)
		}
		return err
	}
	if entry.Status == "trash" {
		return fmt.Errorf("%w: entry already trashed", ErrValidation)
	}
	if !authz.Allowed(actor, authz.PermEntriesTrash, authz.Resource{ContentTypeID: entry.ContentTypeID, EntryID: entryID, OwnerID: stringValue(entry.AuthorID)}, loadGrantsForActor(ctx, s, actor)) {
		return &ForbiddenError{Permission: string(authz.PermEntriesTrash), Scope: "content_type:" + entry.ContentTypeID}
	}
	// Homepage / Posts page protection mirrors publishing protections
	if err := s.checkTrashProtection(ctx, entryID); err != nil {
		return err
	}
	now := time.Now().Unix()
	if err := s.queries.MoveEntryToTrash(ctx, db.MoveEntryToTrashParams{TrashedAt: sql.NullInt64{Int64: now, Valid: true}, UpdatedAt: now, ID: entryID}); err != nil {
		return err
	}
	if s.audit != nil {
		_ = s.audit.Record(ctx, nil, actor, transportForActor(actor), audit.Event{
			Action: "entry.trash", ResourceType: "entry", ResourceID: entryID,
			Metadata: map[string]any{"content_type": entry.ContentTypeID},
		})
	}
	if s.runtime != nil {
		s.runtime.InvalidateEntry(entryID, entry.ContentTypeID)
		s.runtime.InvalidateContent()
		_ = s.runtime.ReloadRoutes(ctx)
	}
	// Delete scheduled jobs if any
	_ = s.queries.DeletePublicationJobsForEntry(ctx, entryID)
	return nil
}

// Restore restores a trashed entry.
func (s *Service) Restore(ctx context.Context, actor authz.Actor, entryID string) error {
	entry, err := s.queries.GetEntry(ctx, entryID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("%w: entry not found", ErrNotFound)
		}
		return err
	}
	if entry.Status != "trash" {
		return fmt.Errorf("%w: entry is not trashed", ErrValidation)
	}
	// Reuse trash permission for restore (symmetric)
	if !authz.Allowed(actor, authz.PermEntriesTrash, authz.Resource{ContentTypeID: entry.ContentTypeID, EntryID: entryID, OwnerID: stringValue(entry.AuthorID)}, loadGrantsForActor(ctx, s, actor)) {
		return &ForbiddenError{Permission: string(authz.PermEntriesTrash), Scope: "content_type:" + entry.ContentTypeID}
	}
	now := time.Now().Unix()
	if err := s.queries.RestoreEntryFromTrash(ctx, db.RestoreEntryFromTrashParams{UpdatedAt: now, ID: entryID}); err != nil {
		return err
	}
	if s.audit != nil {
		_ = s.audit.Record(ctx, nil, actor, transportForActor(actor), audit.Event{
			Action: "entry.restore", ResourceType: "entry", ResourceID: entryID,
			Metadata: map[string]any{"content_type": entry.ContentTypeID},
		})
	}
	if s.runtime != nil {
		s.runtime.InvalidateEntry(entryID, entry.ContentTypeID)
		s.runtime.InvalidateContent()
		_ = s.runtime.ReloadRoutes(ctx)
	}
	return nil
}

func (s *Service) checkTrashProtection(ctx context.Context, entryID string) error {
	settings, err := s.queries.GetSiteSettings(ctx)
	if err != nil {
		return nil
	}
	if settings.HomepageEntryID.Valid && settings.HomepageEntryID.String == entryID {
		return fmt.Errorf("%w: cannot trash the homepage", ErrTrashProtected)
	}
	if settings.PostsPageEntryID.Valid && settings.PostsPageEntryID.String == entryID {
		return fmt.Errorf("%w: cannot trash the posts page", ErrTrashProtected)
	}
	// Hierarchical descendants check? For trash we allow? But publishing had descendant check for private.
	// For consistency, check if entry has published descendants when hierarchical.
	entry, _ := s.queries.GetEntry(ctx, entryID)
	if entry.ContentTypeID != "" {
		def := content.DefinitionFor(entry.ContentTypeID)
		if catalogDef, err := content.NewCatalog(s.queries).GetDefinition(ctx, entry.ContentTypeID); err == nil {
			def = catalogDef
		}
		if def.Capabilities.Hierarchical {
			rows, err := s.queries.ListPublishedHierarchyForContentType(ctx, entry.ContentTypeID)
			if err == nil {
				nodes := make([]content.HierarchyNode, 0, len(rows))
				for _, r := range rows {
					parent := ""
					if r.ParentEntryID.Valid {
						parent = r.ParentEntryID.String
					}
					nodes = append(nodes, content.HierarchyNode{EntryID: r.EntryID, ParentEntryID: parent})
				}
				h, err := content.NewHierarchy(nodes)
				if err == nil && len(h.Descendants(entryID)) != 0 {
					return fmt.Errorf("%w: entry has published descendants", ErrTrashProtected)
				}
			}
		}
	}
	return nil
}

var _ = audit.Event{}
