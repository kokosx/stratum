package routing

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/kokosx/stratum/internal/content"
	db "github.com/kokosx/stratum/internal/storage/sqlc"
)

// HierarchyEntry is the selected-revision data needed to compile hierarchy
// routes. It intentionally contains no HTTP concerns.
type HierarchyEntry struct {
	EntryID       string
	ContentTypeID string
	Slug          string
	Status        string
	Title         string
	ParentEntryID string
	MenuOrder     int64
}

// SyncHierarchyPublish atomically compiles the prospective published hierarchy
// into entry routes. It rebuilds only the published entry's subtree; public
// requests continue to use the precomputed route runtime map.
func SyncHierarchyPublish(ctx context.Context, q *db.Queries, prospective HierarchyEntry, now int64) ([]string, error) {
	rows, err := q.ListPublishedHierarchyForContentType(ctx, prospective.ContentTypeID)
	if err != nil {
		return nil, err
	}
	nodes := make([]content.HierarchyNode, 0, len(rows)+1)
	for _, row := range rows {
		if row.EntryID == prospective.EntryID {
			continue
		}
		nodes = append(nodes, content.HierarchyNode{EntryID: row.EntryID, Slug: row.Slug, ParentEntryID: nullString(row.ParentEntryID), MenuOrder: row.MenuOrder, Title: row.Title})
	}
	nodes = append(nodes, content.HierarchyNode{EntryID: prospective.EntryID, Slug: prospective.Slug, ParentEntryID: prospective.ParentEntryID, MenuOrder: prospective.MenuOrder, Title: prospective.Title})
	hierarchy, err := content.NewHierarchy(nodes)
	if err != nil {
		return nil, err
	}
	if prospective.ParentEntryID != "" {
		parent, ok := hierarchy.Node(prospective.ParentEntryID)
		if !ok || parent.EntryID == prospective.EntryID {
			return nil, errors.New("selected parent does not exist")
		}
		// The parent has to be part of the prior published graph. A prospective
		// child may be new, but publishing it must never create a child of a 404.
		published := false
		for _, row := range rows {
			if row.EntryID == prospective.ParentEntryID && row.Status == "active" {
				published = true
			}
		}
		if !published {
			return nil, errors.New("Publish the parent page first.")
		}
	}

	routes, err := q.ListRoutes(ctx)
	if err != nil {
		return nil, err
	}
	byEntry := map[string]db.Route{}
	byPath := map[string]db.Route{}
	for _, route := range routes {
		byPath[route.Path] = route
		if route.RouteType == RouteTypeEntry && route.EntryID.Valid {
			byEntry[route.EntryID.String] = route
		}
	}
	settings, err := q.GetSiteSettings(ctx)
	if err != nil {
		return nil, err
	}
	paths := make(map[string]string, len(nodes))
	var compile func(string) (string, error)
	compile = func(id string) (string, error) {
		if path, ok := paths[id]; ok {
			return path, nil
		}
		node, ok := hierarchy.Node(id)
		if !ok {
			return "", fmt.Errorf("hierarchy entry %s is missing", id)
		}
		var path string
		if node.ParentEntryID == "" {
			path = EntryPath(prospective.ContentTypeID, node.Slug, settings.PostsBasePath)
			if settings.HomepageEntryID.Valid && settings.HomepageEntryID.String == id {
				path = "/"
			}
		} else {
			parentPath, err := compile(node.ParentEntryID)
			if err != nil {
				return "", err
			}
			path = ChildEntryPath(parentPath, node.Slug)
		}
		paths[id] = path
		return path, nil
	}
	affectedNodes := append([]content.HierarchyNode{{EntryID: prospective.EntryID}}, hierarchy.Descendants(prospective.EntryID)...)
	affected := make([]string, 0, len(affectedNodes))
	affectedSet := make(map[string]bool, len(affectedNodes))
	desired := make(map[string]string, len(affectedNodes))
	for _, node := range affectedNodes {
		affectedSet[node.EntryID] = true
	}
	for _, node := range affectedNodes {
		path, err := compile(node.EntryID)
		if err != nil {
			return nil, err
		}
		if err := validateHierarchyPath(path); err != nil {
			return nil, err
		}
		if owner, exists := byPath[path]; exists && owner.RouteType != RouteTypeRedirect && (!owner.EntryID.Valid || !affectedSet[owner.EntryID.String]) && !(owner.RouteType == RouteTypeEntry && owner.EntryID.Valid && owner.EntryID.String == node.EntryID) {
			return nil, fmt.Errorf("route conflict at %s", path)
		}
		affected = append(affected, node.EntryID)
		desired[node.EntryID] = path
	}
	// Check collisions among paths being created before removing any live route.
	seenPaths := map[string]string{}
	for id, path := range desired {
		if other, exists := seenPaths[path]; exists && other != id {
			return nil, fmt.Errorf("route conflict at %s", path)
		}
		seenPaths[path] = id
	}
	oldPaths := map[string]string{}
	for _, id := range affected {
		if route, ok := byEntry[id]; ok {
			oldPaths[id] = route.Path
			if err := q.DeleteRoute(ctx, route.ID); err != nil {
				return nil, err
			}
		}
	}
	// A canonical live route supersedes an old redirect at its destination.
	for _, path := range desired {
		if route, ok := byPath[path]; ok && route.RouteType == RouteTypeRedirect {
			if err := q.DeleteRoute(ctx, route.ID); err != nil {
				return nil, err
			}
		}
	}
	for _, id := range affected {
		newID, err := randomID()
		if err != nil {
			return nil, err
		}
		if err := q.CreateRoute(ctx, db.CreateRouteParams{ID: newID, Path: desired[id], EntryID: sql.NullString{String: id, Valid: true}, RouteType: RouteTypeEntry, CreatedAt: now, UpdatedAt: now}); err != nil {
			return nil, err
		}
	}
	for _, id := range affected {
		oldPath := oldPaths[id]
		if oldPath == "" || oldPath == desired[id] || seenPaths[oldPath] != "" {
			continue
		}
		if err := UpsertRedirectRoute(ctx, q, oldPath, desired[id], now); err != nil {
			return nil, err
		}
	}
	return affected, nil
}

func validateHierarchyPath(path string) error {
	path = NormalizePath(path)
	for _, prefix := range []string{"/admin", "/stratum", "/media"} {
		if path == prefix || strings.HasPrefix(path, prefix+"/") {
			return fmt.Errorf("path %s is reserved for a core Stratum endpoint", path)
		}
	}
	for _, reserved := range []string{"/sitemap.xml", "/robots.txt", "/feed.xml", "/favicon.ico"} {
		if path == reserved {
			return fmt.Errorf("path %s is reserved for a core Stratum endpoint", path)
		}
	}
	return nil
}

func nullString(value sql.NullString) string {
	if value.Valid {
		return value.String
	}
	return ""
}
