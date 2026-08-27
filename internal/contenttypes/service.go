package contenttypes

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/kokosx/stratum/internal/content"
	"github.com/kokosx/stratum/internal/routing"
	db "github.com/kokosx/stratum/internal/storage/sqlc"
)

// Service provides transactional content type updates with routing transitions.
type Service struct {
	db      *sql.DB
	queries *db.Queries
}

func New(database *sql.DB, queries *db.Queries) *Service {
	return &Service{db: database, queries: queries}
}

// Update performs an atomic content type config update and routing transition.
func (s *Service) Update(ctx context.Context, id string, input content.ContentTypeInput) error {
	if s.db == nil {
		return errors.New("database not configured")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	qtx := s.queries.WithTx(tx)

	cat := content.NewCatalog(qtx)
	previous, err := cat.GetDefinition(ctx, id)
	if err != nil {
		return err
	}
	input.ID = content.ContentTypeID(id)

	if input.Config.SchemaVersion == 0 {
		input.Config.SchemaVersion = 2
	}
	if !isBuiltin(input.ID) {
		// LEGACY STORAGE COMPATIBILITY ONLY: public = single
		input.Public = input.Config.Routing.Single
	}
	if err := content.ValidateContentTypeInput(input, true); err != nil {
		return err
	}
	if isBuiltin(input.ID) {
		input.Hierarchical = previous.Capabilities.Hierarchical
		input.Public = previous.Capabilities.Public
		input.Config.Routing.Single = previous.Routing.Single
		input.Config.Routing.Archive = previous.Routing.Archive
		input.Config.Routing.BasePath = previous.Routing.BasePath
	}
	if err := content.ValidateFieldEvolution(previous.Fields, input.Config.Fields); err != nil {
		return err
	}
	if input.Config.Routing.BasePath != "" && input.Config.Routing.BasePath != previous.Routing.BasePath {
		if err := cat.EnsureBasePathUnique(ctx, string(input.ID), input.Config.Routing.BasePath); err != nil {
			return err
		}
	}
	// SchemaVersion semantics: only bump when field schema meaningfully changes
	if content.SchemaChanged(previous.Fields, input.Config.Fields) {
		if input.Config.SchemaVersion <= previous.SchemaVersion {
			input.Config.SchemaVersion = previous.SchemaVersion + 1
		}
	} else {
		input.Config.SchemaVersion = previous.SchemaVersion
	}
	encoded, err := content.EncodeContentTypeConfig(input.Config)
	if err != nil {
		return err
	}
	if err := qtx.UpdateContentType(ctx, db.UpdateContentTypeParams{
		ID: string(input.ID), DisplayName: input.Name, PluralName: input.PluralName,
		Hierarchical: boolInt(input.Hierarchical), Public: boolInt(input.Public),
		ConfigJson: encoded, UpdatedAt: time.Now().Unix(),
	}); err != nil {
		return err
	}

	prevSingle := previous.Routing.Single
	newSingle := input.Config.Routing.Single
	prevArchive := previous.Routing.Archive
	newArchive := input.Config.Routing.Archive
	prevBase := previous.Routing.BasePath
	newBase := input.Config.Routing.BasePath
	now := time.Now().Unix()

	if prevSingle && !newSingle {
		if err := deleteEntryRoutesForContentType(ctx, qtx, string(previous.ID)); err != nil {
			return fmt.Errorf("remove entry routes: %w", err)
		}
	}
	if prevArchive && !newArchive {
		if route, err := qtx.GetArchiveRouteByContentType(ctx, sql.NullString{String: string(previous.ID), Valid: true}); err == nil {
			if err := qtx.DeleteRoute(ctx, route.ID); err != nil {
				return err
			}
		} else if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
	}

	if !prevSingle && newSingle {
		if newBase == "" {
			return fmt.Errorf("URL base required when single routing is enabled")
		}
		if err := createEntryRoutesForContentType(ctx, qtx, string(previous.ID), newBase, now, previous.Capabilities.Hierarchical || input.Hierarchical); err != nil {
			return err
		}
		if !prevArchive && newArchive {
			if err := ensureArchiveRoute(ctx, qtx, string(previous.ID), newBase, now); err != nil {
				return err
			}
		} else if prevArchive && newArchive && prevBase != newBase {
			if err := moveArchiveRoute(ctx, qtx, string(previous.ID), prevBase, newBase, now); err != nil {
				return err
			}
		}
	} else if prevSingle && newSingle {
		if prevBase != newBase || prevArchive != newArchive {
			if err := routing.SyncContentTypeRouting(ctx, qtx, string(previous.ID), prevBase, newBase, newArchive, now); err != nil {
				return err
			}
		}
	} else {
		if !prevArchive && newArchive {
			if err := ensureArchiveRoute(ctx, qtx, string(previous.ID), newBase, now); err != nil {
				return err
			}
		} else if prevArchive && newArchive && prevBase != newBase {
			if err := moveArchiveRoute(ctx, qtx, string(previous.ID), prevBase, newBase, now); err != nil {
				return err
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return err
	}
	return nil
}

// Create wraps catalog creation with archive route handling.
func (s *Service) Create(ctx context.Context, input content.ContentTypeInput) error {
	if s.db == nil {
		return errors.New("database not configured")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	qtx := s.queries.WithTx(tx)
	cat := content.NewCatalog(qtx)
	if err := cat.CreateContentType(ctx, input); err != nil {
		return err
	}
	if input.Config.Routing.Archive && input.Config.Routing.BasePath != "" {
		now := time.Now().Unix()
		if err := ensureArchiveRoute(ctx, qtx, string(input.ID), input.Config.Routing.BasePath, now); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	return nil
}

func deleteEntryRoutesForContentType(ctx context.Context, qtx *db.Queries, contentType string) error {
	hRows, err := qtx.ListPublishedHierarchyForContentType(ctx, contentType)
	if err == nil && len(hRows) > 0 {
		for _, row := range hRows {
			if rt, err := qtx.GetEntryRoute(ctx, sql.NullString{String: row.EntryID, Valid: true}); err == nil {
				if err := qtx.DeleteRoute(ctx, rt.ID); err != nil {
					return err
				}
			} else if !errors.Is(err, sql.ErrNoRows) {
				return err
			}
		}
		return nil
	}
	const batchSize = 500
	for offset := int64(0); ; offset += batchSize {
		batch, err := qtx.ListPublishedEntriesByContentType(ctx, db.ListPublishedEntriesByContentTypeParams{ContentTypeID: contentType, Limit: batchSize, Offset: offset})
		if err != nil {
			return err
		}
		if len(batch) == 0 {
			break
		}
		for _, row := range batch {
			if rt, err := qtx.GetEntryRoute(ctx, sql.NullString{String: row.ID, Valid: true}); err == nil {
				if err := qtx.DeleteRoute(ctx, rt.ID); err != nil {
					return err
				}
			} else if !errors.Is(err, sql.ErrNoRows) {
				return err
			}
		}
		if int64(len(batch)) < batchSize {
			break
		}
	}
	return nil
}

func createEntryRoutesForContentType(ctx context.Context, qtx *db.Queries, contentType, basePath string, now int64, hierarchical bool) error {
	basePath = routing.NormalizePath(basePath)
	if err := routing.ValidateRouteBase(basePath); err != nil {
		return err
	}
	if hierarchical {
		hRows, err := qtx.ListPublishedHierarchyForContentType(ctx, contentType)
		if err != nil {
			return err
		}
		if len(hRows) == 0 {
			return nil
		}
		nodes := make([]content.HierarchyNode, 0, len(hRows))
		for _, r := range hRows {
			parent := ""
			if r.ParentEntryID.Valid {
				parent = r.ParentEntryID.String
			}
			nodes = append(nodes, content.HierarchyNode{EntryID: r.EntryID, Slug: r.Slug, ParentEntryID: parent, MenuOrder: r.MenuOrder, Title: r.Title})
		}
		tree, err := content.NewHierarchy(nodes)
		if err != nil {
			return err
		}
		paths := make(map[string]string, len(nodes))
		var compile func(string) (string, error)
		compile = func(id string) (string, error) {
			if p, ok := paths[id]; ok {
				return p, nil
			}
			n, ok := tree.Node(id)
			if !ok {
				return "", fmt.Errorf("missing %s", id)
			}
			var p string
			if n.ParentEntryID == "" {
				def := content.ContentTypeDefinition{ID: content.ContentTypeID(contentType), Routing: content.RoutingPolicy{BasePath: basePath, Single: true}}
				p = routing.EntryPathForDefinition(def, n.Slug, "")
			} else {
				pp, err := compile(n.ParentEntryID)
				if err != nil {
					return "", err
				}
				p = routing.ChildEntryPath(pp, n.Slug)
			}
			paths[id] = p
			return p, nil
		}
		for _, n := range nodes {
			if _, err := compile(n.EntryID); err != nil {
				return err
			}
		}
		for id, p := range paths {
			if existing, err := qtx.GetRouteByPath(ctx, p); err == nil && (!existing.EntryID.Valid || existing.EntryID.String != id) {
				return fmt.Errorf("route %s already exists", p)
			} else if err != nil && !errors.Is(err, sql.ErrNoRows) {
				return err
			}
		}
		for id, p := range paths {
			if err := routing.UpsertEntryRoute(ctx, qtx, id, p, now); err != nil {
				return err
			}
		}
		return nil
	}
	const batchSize = 500
	var allRows []db.ListPublishedEntriesByContentTypeRow
	for offset := int64(0); ; offset += batchSize {
		batch, err := qtx.ListPublishedEntriesByContentType(ctx, db.ListPublishedEntriesByContentTypeParams{ContentTypeID: contentType, Limit: batchSize, Offset: offset})
		if err != nil {
			return err
		}
		if len(batch) == 0 {
			break
		}
		allRows = append(allRows, batch...)
		if int64(len(batch)) < batchSize {
			break
		}
	}
	for _, row := range allRows {
		newPath := routing.NormalizePath(basePath + "/" + strings.Trim(row.Slug, "/"))
		if existing, err := qtx.GetRouteByPath(ctx, newPath); err == nil && (!existing.EntryID.Valid || existing.EntryID.String != row.ID) {
			return fmt.Errorf("route %s already exists", newPath)
		} else if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
	}
	for _, row := range allRows {
		newPath := routing.NormalizePath(basePath + "/" + strings.Trim(row.Slug, "/"))
		if err := routing.UpsertEntryRoute(ctx, qtx, row.ID, newPath, now); err != nil {
			return err
		}
	}
	return nil
}

func ensureArchiveRoute(ctx context.Context, qtx *db.Queries, contentType, basePath string, now int64) error {
	basePath = routing.NormalizePath(basePath)
	if err := routing.ValidateRouteBase(basePath); err != nil {
		return err
	}
	if _, err := qtx.GetArchiveRouteByContentType(ctx, sql.NullString{String: contentType, Valid: true}); err == nil {
		return nil
	} else if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if existing, err := qtx.GetRouteByPath(ctx, basePath); err == nil {
		_ = existing
		return fmt.Errorf("route %s already exists", basePath)
	} else if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	id, err := randomID()
	if err != nil {
		return err
	}
	return qtx.CreateRoute(ctx, db.CreateRouteParams{ID: id, Path: basePath, RouteType: routing.RouteTypeArchive, ContentTypeID: sql.NullString{String: contentType, Valid: true}, CreatedAt: now, UpdatedAt: now})
}

func moveArchiveRoute(ctx context.Context, qtx *db.Queries, contentType, oldBase, newBase string, now int64) error {
	oldBase = routing.NormalizePath(oldBase)
	newBase = routing.NormalizePath(newBase)
	if oldBase == newBase {
		return nil
	}
	if err := routing.ValidateRouteBase(newBase); err != nil {
		return err
	}
	route, err := qtx.GetArchiveRouteByContentType(ctx, sql.NullString{String: contentType, Valid: true})
	if errors.Is(err, sql.ErrNoRows) {
		return ensureArchiveRoute(ctx, qtx, contentType, newBase, now)
	}
	if err != nil {
		return err
	}
	if route.Path == newBase {
		return nil
	}
	if existing, err := qtx.GetRouteByPath(ctx, newBase); err == nil && existing.ID != route.ID {
		return fmt.Errorf("route %s already exists", newBase)
	} else if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	oldPath := route.Path
	if err := qtx.UpdateRoute(ctx, db.UpdateRouteParams{ID: route.ID, Path: newBase, RouteType: routing.RouteTypeArchive, ContentTypeID: sql.NullString{String: contentType, Valid: true}, UpdatedAt: now}); err != nil {
		return err
	}
	return routing.UpsertRedirectRoute(ctx, qtx, oldPath, newBase, now)
}

func randomID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func isBuiltin(id content.ContentTypeID) bool {
	return id == content.ContentTypePage || id == content.ContentTypePost
}
func boolInt(v bool) int64 {
	if v {
		return 1
	}
	return 0
}
