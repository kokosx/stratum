package entryops

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/kokosx/stratum/internal/audit"
	"github.com/kokosx/stratum/internal/authz"
)

// Publish publishes the exact revision. Requires entries.publish permission scoped to the entry's type.
func (s *Service) Publish(ctx context.Context, actor authz.Actor, entryID, revisionID string) (string, error) {
	if s.publishing == nil {
		return "", errors.New("publishing service not configured")
	}
	// Load entry to determine content type for authz before tx
	entry, err := s.queries.GetEntry(ctx, entryID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", fmt.Errorf("%w: entry not found", ErrNotFound)
		}
		return "", err
	}
	if entry.Status == "trash" {
		return "", fmt.Errorf("%w: cannot publish trashed entry", ErrValidation)
	}
	if !authz.Allowed(actor, authz.PermEntriesPublish, authz.Resource{ContentTypeID: entry.ContentTypeID, EntryID: entryID, OwnerID: stringValue(entry.AuthorID)}, loadGrantsForActor(ctx, s, actor)) {
		return "", &ForbiddenError{Permission: string(authz.PermEntriesPublish), Scope: "content_type:" + entry.ContentTypeID}
	}
	// Load revision to verify belongs to entry
	rev, err := s.queries.GetEntryRevision(ctx, revisionID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", fmt.Errorf("%w: revision not found", ErrNotFound)
		}
		return "", err
	}
	if rev.EntryID != entryID {
		return "", fmt.Errorf("%w: revision does not belong to entry", ErrValidation)
	}
	now := time.Now().Unix()
	if err := s.publishing.PublishRevision(ctx, entryID, revisionID, now); err != nil {
		return "", err
	}
	// Post-commit invalidation and audit (publish already committed, so audit is best-effort outside tx)
	if s.audit != nil {
		_ = s.audit.Record(ctx, nil, actor, transportForActor(actor), audit.Event{
			Action: "entry.publish", ResourceType: "entry", ResourceID: entryID, RevisionID: revisionID,
			Metadata: map[string]any{"content_type": entry.ContentTypeID},
		})
	}
	// Runtime invalidation
	if s.runtime != nil {
		s.runtime.InvalidateEntry(entryID, entry.ContentTypeID)
		s.runtime.InvalidateContent()
		_ = s.runtime.ReloadRoutes(ctx)
	}
	if s.searchRefresh != nil {
		_ = s.searchRefresh(context.Background(), entryID)
	}
	// Determine public path
	publicPath := ""
	if r, err := s.queries.GetEntryRoute(ctx, sql.NullString{String: entryID, Valid: true}); err == nil {
		publicPath = r.Path
	}
	return publicPath, nil
}

// Unpublish removes the published revision (draft remains).
func (s *Service) Unpublish(ctx context.Context, actor authz.Actor, entryID string) error {
	entry, err := s.queries.GetEntry(ctx, entryID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("%w: entry not found", ErrNotFound)
		}
		return err
	}
	if !authz.Allowed(actor, authz.PermEntriesPublish, authz.Resource{ContentTypeID: entry.ContentTypeID, EntryID: entryID, OwnerID: stringValue(entry.AuthorID)}, loadGrantsForActor(ctx, s, actor)) {
		return &ForbiddenError{Permission: string(authz.PermEntriesPublish), Scope: "content_type:" + entry.ContentTypeID}
	}
	if s.publishing == nil {
		return errors.New("publishing service not configured")
	}
	if err := s.publishing.Unpublish(ctx, entryID, time.Now().Unix()); err != nil {
		return err
	}
	if s.audit != nil {
		_ = s.audit.Record(ctx, nil, actor, transportForActor(actor), audit.Event{
			Action: "entry.unpublish", ResourceType: "entry", ResourceID: entryID,
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

// publish helpers to allow mock
var _ = audit.Event{}
