package content

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	db "github.com/kokosx/stratum/internal/storage/sqlc"
)

var (
	ErrNotTrashed               = errors.New("entry is not in trash")
	ErrAlreadyTrashed           = errors.New("entry is already in trash")
	ErrProtectedPage            = errors.New("this page is configured as Homepage or Posts Page and cannot be trashed or deleted until the Site Settings are changed")
	ErrPermanentDeleteOnlyTrash = errors.New("permanent delete is only allowed from trash")
	ErrRouteOccupied            = errors.New("restore failed: the entry route is occupied")
	ErrPublishedDescendants     = errors.New("cannot trash or permanently delete an entry with published descendants; move or trash those pages first")
	ErrHierarchyDescendants     = errors.New("cannot permanently delete an entry with draft descendants; move or trash those pages first")
	ErrParentUnavailable        = errors.New("restore failed: publish and restore the parent page first")
)

type LifecycleService struct {
	db      *sql.DB
	queries *db.Queries
}

func NewLifecycleService(database *sql.DB, queries *db.Queries) *LifecycleService {
	return &LifecycleService{db: database, queries: queries}
}

func (s *LifecycleService) MoveToTrash(ctx context.Context, entryID string) error {
	entry, err := s.queries.GetEntry(ctx, entryID)
	if err != nil {
		return err
	}
	if entry.Status == "trash" {
		return ErrAlreadyTrashed
	}
	if err := s.assertNotProtected(ctx, entryID); err != nil {
		return err
	}
	if err := s.assertNoPublishedDescendants(ctx, entry); err != nil {
		return err
	}
	if s.db == nil {
		return errors.New("database not configured")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	qtx := s.queries.WithTx(tx)
	now := time.Now().Unix()
	if err := qtx.MoveEntryToTrash(ctx, db.MoveEntryToTrashParams{TrashedAt: sql.NullInt64{Int64: now, Valid: true}, UpdatedAt: now, ID: entryID}); err != nil {
		return err
	}
	if rt, err := qtx.GetEntryRoute(ctx, sql.NullString{String: entryID, Valid: true}); err == nil {
		if err := qtx.DeleteRoute(ctx, rt.ID); err != nil {
			return err
		}
	} else if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	return nil
}

func (s *LifecycleService) Restore(ctx context.Context, entryID string) error {
	entry, err := s.queries.GetEntry(ctx, entryID)
	if err != nil {
		return err
	}
	if entry.Status != "trash" {
		return ErrNotTrashed
	}
	if s.db == nil {
		return errors.New("database not configured")
	}
	var targetPath string
	needsRoute := needsPublicRoute(entry)
	if needsRoute {
		settings, err := s.queries.GetSiteSettings(ctx)
		if err == nil {
			targetPath, err = s.restorePath(ctx, s.queries, entry, settings.PostsBasePath)
			if err != nil {
				return err
			}
			if settings.HomepageEntryID.Valid && settings.HomepageEntryID.String == entryID {
				targetPath = "/"
			}
			if byPath, err := s.queries.GetRouteByPath(ctx, targetPath); err == nil && byPath.EntryID.Valid && byPath.EntryID.String != entryID {
				return ErrRouteOccupied
			}
		}
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	qtx := s.queries.WithTx(tx)
	now := time.Now().Unix()
	if needsRoute && targetPath != "" {
		if byPath, err := qtx.GetRouteByPath(ctx, targetPath); err == nil && byPath.EntryID.Valid && byPath.EntryID.String != entryID {
			return ErrRouteOccupied
		}
		if byPath, err := qtx.GetRouteByPath(ctx, targetPath); err == nil && !byPath.EntryID.Valid {
			if err := qtx.DeleteRoute(ctx, byPath.ID); err != nil {
				return err
			}
		} else if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
	}
	if err := qtx.RestoreEntryFromTrash(ctx, db.RestoreEntryFromTrashParams{UpdatedAt: now, ID: entryID}); err != nil {
		return err
	}
	if needsRoute && targetPath != "" {
		settings, err := qtx.GetSiteSettings(ctx)
		if err != nil {
			return err
		}
		if settings.HomepageEntryID.Valid && settings.HomepageEntryID.String == entryID {
			targetPath = "/"
		}
		if err := createEntryRoute(ctx, qtx, entryID, targetPath, now); err != nil {
			return fmt.Errorf("recreate route: %w", err)
		}
	}
	return tx.Commit()
}

func (s *LifecycleService) DeletePermanently(ctx context.Context, entryID string) error {
	entry, err := s.queries.GetEntry(ctx, entryID)
	if err != nil {
		return err
	}
	if entry.Status != "trash" {
		return ErrPermanentDeleteOnlyTrash
	}
	if err := s.assertNotProtected(ctx, entryID); err != nil {
		return err
	}
	if err := s.assertNoPublishedDescendants(ctx, entry); err != nil {
		return err
	}
	if err := s.assertNoLatestDescendants(ctx, entry); err != nil {
		return err
	}
	return s.queries.DeleteEntry(ctx, entryID)
}

func (s *LifecycleService) assertNotProtected(ctx context.Context, entryID string) error {
	settings, err := s.queries.GetSiteSettings(ctx)
	if err != nil {
		return err
	}
	trim := strings.TrimSpace(entryID)
	if settings.HomepageEntryID.Valid && settings.HomepageEntryID.String == trim {
		return ErrProtectedPage
	}
	if settings.PostsPageEntryID.Valid && settings.PostsPageEntryID.String == trim {
		return ErrProtectedPage
	}
	return nil
}

func (s *LifecycleService) assertNoPublishedDescendants(ctx context.Context, entry db.Entry) error {
	if !DefinitionFor(entry.ContentTypeID).Capabilities.Hierarchical || !entry.PublishedRevisionID.Valid {
		return nil
	}
	rows, err := s.queries.ListPublishedHierarchyForContentType(ctx, entry.ContentTypeID)
	if err != nil {
		return err
	}
	nodes := make([]HierarchyNode, 0, len(rows))
	for _, row := range rows {
		parent := ""
		if row.ParentEntryID.Valid {
			parent = row.ParentEntryID.String
		}
		nodes = append(nodes, HierarchyNode{EntryID: row.EntryID, ParentEntryID: parent, MenuOrder: row.MenuOrder, Title: row.Title})
	}
	hierarchy, err := NewHierarchy(nodes)
	if err != nil {
		return err
	}
	if len(hierarchy.Descendants(entry.ID)) != 0 {
		return ErrPublishedDescendants
	}
	return nil
}

func (s *LifecycleService) assertNoLatestDescendants(ctx context.Context, entry db.Entry) error {
	if !DefinitionFor(entry.ContentTypeID).Capabilities.Hierarchical {
		return nil
	}
	rows, err := s.queries.ListLatestHierarchyForContentType(ctx, entry.ContentTypeID)
	if err != nil {
		return err
	}
	nodes := make([]HierarchyNode, 0, len(rows))
	for _, row := range rows {
		parent := ""
		if row.ParentEntryID.Valid {
			parent = row.ParentEntryID.String
		}
		nodes = append(nodes, HierarchyNode{EntryID: row.EntryID, ParentEntryID: parent, MenuOrder: row.MenuOrder, Title: row.Title})
	}
	hierarchy, err := NewHierarchy(nodes)
	if err != nil {
		return err
	}
	if len(hierarchy.Descendants(entry.ID)) != 0 {
		return ErrHierarchyDescendants
	}
	return nil
}

// restorePath uses the parent entry's existing compiled route. This preserves
// nested and Homepage-rooted URLs without doing public-time ancestry traversal.
func (s *LifecycleService) restorePath(ctx context.Context, q *db.Queries, entry db.Entry, postsBase string) (string, error) {
	if !entry.PublishedRevisionID.Valid {
		return "", sql.ErrNoRows
	}
	revision, err := q.GetEntryRevision(ctx, entry.PublishedRevisionID.String)
	if err != nil {
		return "", err
	}
	if !DefinitionFor(entry.ContentTypeID).Capabilities.Hierarchical || !revision.ParentEntryID.Valid {
		return entryPublicPath(entry.ContentTypeID, revision.Slug, postsBase), nil
	}
	parent, err := q.GetEntry(ctx, revision.ParentEntryID.String)
	if err != nil || parent.Status != "active" || !parent.PublishedRevisionID.Valid {
		return "", ErrParentUnavailable
	}
	parentRoute, err := q.GetEntryRoute(ctx, sql.NullString{String: parent.ID, Valid: true})
	if err != nil {
		return "", ErrParentUnavailable
	}
	if parentRoute.Path == "/" {
		return "/" + strings.Trim(revision.Slug, "/"), nil
	}
	return strings.TrimRight(parentRoute.Path, "/") + "/" + strings.Trim(revision.Slug, "/"), nil
}

// Unpublish removes a public route only when no published child would remain
// reachable beneath it.
func (s *LifecycleService) Unpublish(ctx context.Context, entryID string) error {
	entry, err := s.queries.GetEntry(ctx, entryID)
	if err != nil {
		return err
	}
	if err := s.assertNotProtected(ctx, entryID); err != nil {
		return err
	}
	if err := s.assertNoPublishedDescendants(ctx, entry); err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	qtx := s.queries.WithTx(tx)
	if err := qtx.ClearPublishedRevision(ctx, db.ClearPublishedRevisionParams{UpdatedAt: time.Now().Unix(), ID: entryID}); err != nil {
		return err
	}
	if route, err := qtx.GetEntryRoute(ctx, sql.NullString{String: entryID, Valid: true}); err == nil {
		if err := qtx.DeleteRoute(ctx, route.ID); err != nil {
			return err
		}
	} else if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	return tx.Commit()
}

func (s *LifecycleService) BulkTrash(ctx context.Context, contentTypeID string, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	for _, id := range ids {
		e, err := s.queries.GetEntry(ctx, id)
		if err != nil {
			return fmt.Errorf("entry %s: %w", id, err)
		}
		if e.ContentTypeID != contentTypeID {
			return fmt.Errorf("entry %s does not belong to %s", id, contentTypeID)
		}
		if e.Status == "trash" {
			return fmt.Errorf("entry %s is already in trash", id)
		}
		if err := s.assertNotProtected(ctx, id); err != nil {
			return err
		}
		if err := s.assertNoPublishedDescendants(ctx, e); err != nil {
			return err
		}
	}
	if s.db == nil {
		return errors.New("database not configured")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	qtx := s.queries.WithTx(tx)
	now := time.Now().Unix()
	for _, id := range ids {
		if err := qtx.MoveEntryToTrash(ctx, db.MoveEntryToTrashParams{TrashedAt: sql.NullInt64{Int64: now, Valid: true}, UpdatedAt: now, ID: id}); err != nil {
			return err
		}
		if rt, err := qtx.GetEntryRoute(ctx, sql.NullString{String: id, Valid: true}); err == nil {
			if err := qtx.DeleteRoute(ctx, rt.ID); err != nil {
				return err
			}
		} else if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
	}
	return tx.Commit()
}

func (s *LifecycleService) BulkRestore(ctx context.Context, contentTypeID string, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	settings, err := s.queries.GetSiteSettings(ctx)
	if err != nil {
		return err
	}
	for _, id := range ids {
		e, err := s.queries.GetEntry(ctx, id)
		if err != nil {
			return fmt.Errorf("entry %s: %w", id, err)
		}
		if e.ContentTypeID != contentTypeID {
			return fmt.Errorf("entry %s does not belong to %s", id, contentTypeID)
		}
		if e.Status != "trash" {
			return fmt.Errorf("entry %s is not in trash", id)
		}
		if needsPublicRoute(e) {
			targetPath, err := s.restorePath(ctx, s.queries, e, settings.PostsBasePath)
			if err != nil {
				return err
			}
			if settings.HomepageEntryID.Valid && settings.HomepageEntryID.String == id {
				targetPath = "/"
			}
			if byPath, err := s.queries.GetRouteByPath(ctx, targetPath); err == nil && byPath.EntryID.Valid && byPath.EntryID.String != id {
				return ErrRouteOccupied
			}
		}
	}
	if s.db == nil {
		return errors.New("database not configured")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	qtx := s.queries.WithTx(tx)
	now := time.Now().Unix()
	for _, id := range ids {
		ent, err := qtx.GetEntry(ctx, id)
		if err != nil {
			return err
		}
		needsRoute := needsPublicRoute(ent)
		targetPath := ""
		if needsRoute {
			s2 := settings
			cur, err := qtx.GetSiteSettings(ctx)
			if err != nil {
				return err
			}
			s2 = cur
			targetPath, err = s.restorePath(ctx, qtx, ent, s2.PostsBasePath)
			if err != nil {
				return err
			}
			if s2.HomepageEntryID.Valid && s2.HomepageEntryID.String == id {
				targetPath = "/"
			}
			if byPath, err := qtx.GetRouteByPath(ctx, targetPath); err == nil && !byPath.EntryID.Valid {
				if err := qtx.DeleteRoute(ctx, byPath.ID); err != nil {
					return err
				}
			} else if err != nil && !errors.Is(err, sql.ErrNoRows) {
				return err
			}
		}
		if err := qtx.RestoreEntryFromTrash(ctx, db.RestoreEntryFromTrashParams{UpdatedAt: now, ID: id}); err != nil {
			return err
		}
		if needsRoute && targetPath != "" {
			if err := createEntryRoute(ctx, qtx, id, targetPath, now); err != nil {
				return fmt.Errorf("recreate route for %s: %w", id, err)
			}
		}
	}
	return tx.Commit()
}

func needsPublicRoute(entry db.Entry) bool {
	return entry.PublishedRevisionID.Valid && entry.StatusBeforeTrash.Valid && entry.StatusBeforeTrash.String == "active"
}

func (s *LifecycleService) BulkDeletePermanently(ctx context.Context, contentTypeID string, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	for _, id := range ids {
		e, err := s.queries.GetEntry(ctx, id)
		if err != nil {
			return fmt.Errorf("entry %s: %w", id, err)
		}
		if e.ContentTypeID != contentTypeID {
			return fmt.Errorf("entry %s does not belong to %s", id, contentTypeID)
		}
		if e.Status != "trash" {
			return ErrPermanentDeleteOnlyTrash
		}
		if err := s.assertNotProtected(ctx, id); err != nil {
			return err
		}
		if err := s.assertNoPublishedDescendants(ctx, e); err != nil {
			return err
		}
		if err := s.assertNoLatestDescendants(ctx, e); err != nil {
			return err
		}
	}
	if s.db == nil {
		return errors.New("database not configured")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	qtx := s.queries.WithTx(tx)
	for _, id := range ids {
		if err := qtx.DeleteEntry(ctx, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func entryPublicPath(contentTypeID, slug, postsBase string) string {
	s := strings.Trim(slug, "/")
	if s == "" {
		return "/"
	}
	if contentTypeID == "post" {
		base := postsBase
		if strings.TrimSpace(base) == "" {
			base = "/blog"
		}
		base = strings.TrimRight(strings.TrimSpace(base), "/")
		if base == "" {
			base = "/blog"
		}
		if !strings.HasPrefix(base, "/") {
			base = "/" + base
		}
		if base == "/" {
			return "/" + s
		}
		return base + "/" + s
	}
	return "/" + s
}

func createEntryRoute(ctx context.Context, q *db.Queries, entryID, path string, now int64) error {
	if rt, err := q.GetEntryRoute(ctx, sql.NullString{String: entryID, Valid: true}); err == nil && rt.Path == path {
		return nil
	}
	if rt, err := q.GetEntryRoute(ctx, sql.NullString{String: entryID, Valid: true}); err == nil {
		return q.UpdateRoute(ctx, db.UpdateRouteParams{
			ID: rt.ID, Path: path, EntryID: sql.NullString{String: entryID, Valid: true},
			RouteType: "entry", UpdatedAt: now,
		})
	}
	id, err := randomID()
	if err != nil {
		return err
	}
	return q.CreateRoute(ctx, db.CreateRouteParams{
		ID: id, Path: path, EntryID: sql.NullString{String: entryID, Valid: true},
		RouteType: "entry", CreatedAt: now, UpdatedAt: now,
	})
}

func randomID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
