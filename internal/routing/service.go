package routing

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"strings"

	db "github.com/kokosx/stratum/internal/storage/sqlc"
)

// UpsertEntryRoute ensures an entry owns exactly one canonical entry-type route
// at path, creating redirects from the old location.
func UpsertEntryRoute(ctx context.Context, queries *db.Queries, entryID, path string, now int64) error {
	reserved := map[string]bool{"admin": true, "stratum": true, "sitemap.xml": true, "robots.txt": true}
	if path != "/" && reserved[strings.TrimPrefix(path, "/")] {
		return errors.New("this slug is reserved for a core Stratum endpoint")
	}
	if settings, err := queries.GetSiteSettings(ctx); err == nil && settings.HomepageEntryID.Valid && settings.HomepageEntryID.String == entryID {
		path = "/"
	}
	byPath, err := queries.GetRouteByPath(ctx, path)
	if err == nil && byPath.EntryID.Valid && byPath.EntryID.String != entryID {
		return errors.New("a route already uses this slug")
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("check entry route: %w", err)
	}
	if err == nil && !byPath.EntryID.Valid {
		if delErr := queries.DeleteRoute(ctx, byPath.ID); delErr != nil {
			return fmt.Errorf("clear stale redirect: %w", delErr)
		}
	}
	route, err := queries.GetEntryRoute(ctx, sql.NullString{String: entryID, Valid: true})
	if errors.Is(err, sql.ErrNoRows) {
		id, idErr := randomID()
		if idErr != nil {
			return idErr
		}
		return queries.CreateRoute(ctx, db.CreateRouteParams{
			ID: id, Path: path, EntryID: sql.NullString{String: entryID, Valid: true},
			RouteType: RouteTypeEntry, ContentTypeID: sql.NullString{}, CreatedAt: now, UpdatedAt: now,
		})
	}
	if err != nil {
		return fmt.Errorf("get entry route: %w", err)
	}
	if route.Path == path {
		return nil
	}
	oldPath := route.Path
	if err := queries.UpdateRoute(ctx, db.UpdateRouteParams{
		ID: route.ID, Path: path, EntryID: sql.NullString{String: entryID, Valid: true},
		RouteType: RouteTypeEntry, ContentTypeID: sql.NullString{}, UpdatedAt: now,
	}); err != nil {
		return fmt.Errorf("move entry route: %w", err)
	}
	return UpsertRedirectRoute(ctx, queries, oldPath, path, now)
}

// UpsertRedirectRoute records a 301 redirect from source to target, flattening chains.
func UpsertRedirectRoute(ctx context.Context, queries *db.Queries, source, target string, now int64) error {
	inbound, err := queries.ListRedirectsToTarget(ctx, sql.NullString{String: source, Valid: true})
	if err != nil {
		return fmt.Errorf("list redirects to %s: %w", source, err)
	}
	for _, inboundRoute := range inbound {
		if inboundRoute.Path == target {
			continue
		}
		if err := queries.UpdateRoute(ctx, db.UpdateRouteParams{
			ID: inboundRoute.ID, Path: inboundRoute.Path, EntryID: sql.NullString{},
			RouteType: RouteTypeRedirect, ContentTypeID: sql.NullString{},
			RedirectTo:     sql.NullString{String: target, Valid: true},
			RedirectStatus: sql.NullInt64{Int64: http.StatusMovedPermanently, Valid: true}, UpdatedAt: now,
		}); err != nil {
			return fmt.Errorf("flatten redirect chain %s: %w", inboundRoute.Path, err)
		}
	}
	existing, err := queries.GetRouteByPath(ctx, source)
	if err == nil {
		return queries.UpdateRoute(ctx, db.UpdateRouteParams{
			ID: existing.ID, Path: source, EntryID: sql.NullString{},
			RouteType: RouteTypeRedirect, ContentTypeID: sql.NullString{},
			RedirectTo:     sql.NullString{String: target, Valid: true},
			RedirectStatus: sql.NullInt64{Int64: http.StatusMovedPermanently, Valid: true}, UpdatedAt: now,
		})
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("check redirect route: %w", err)
	}
	id, idErr := randomID()
	if idErr != nil {
		return idErr
	}
	return queries.CreateRoute(ctx, db.CreateRouteParams{
		ID: id, Path: source, EntryID: sql.NullString{},
		RouteType: RouteTypeRedirect, ContentTypeID: sql.NullString{},
		RedirectTo:     sql.NullString{String: target, Valid: true},
		RedirectStatus: sql.NullInt64{Int64: http.StatusMovedPermanently, Valid: true}, CreatedAt: now, UpdatedAt: now,
	})
}

// SyncContentTypeRouting atomically moves a custom type's archive and every
// published single route. The caller owns the transaction; all collisions are
// checked before the first mutation so failure cannot leave a partial move.
// It uses bounded batches (500) so the 1001st entry is never silently ignored.
func SyncContentTypeRouting(ctx context.Context, q *db.Queries, contentType, oldBase, newBase string, archive bool, now int64) error {
	if err := ValidateRouteBase(newBase); err != nil {
		return err
	}
	oldBase = NormalizePath(oldBase)
	newBase = NormalizePath(newBase)

	// Detect hierarchical to use tree-aware compilation. Flat custom types can
	// be handled with a simple base+slug move; hierarchical must preserve
	// parent/child path derivation so /docs/a/b stays coherent when /docs moves.
	isHierarchical, hierarchicalDef := isHierarchicalContentType(ctx, q, contentType)

	var moves []struct{ entryID, oldPath, newPath string }
	if isHierarchical {
		hRows, err := q.ListPublishedHierarchyForContentType(ctx, contentType)
		if err != nil {
			return fmt.Errorf("list published hierarchy: %w", err)
		}
		nodes := make([]hierarchyNodeLite, 0, len(hRows))
		for _, r := range hRows {
			parent := ""
			if r.ParentEntryID.Valid {
				parent = r.ParentEntryID.String
			}
			nodes = append(nodes, hierarchyNodeLite{ID: r.EntryID, Slug: r.Slug, ParentID: parent})
		}
		desired, err := computeHierarchyPaths(ctx, q, hierarchicalDef, newBase, nodes)
		if err != nil {
			return err
		}
		moves = make([]struct{ entryID, oldPath, newPath string }, 0, len(desired))
		for id, newPath := range desired {
			route, err := q.GetEntryRoute(ctx, sql.NullString{String: id, Valid: true})
			oldPath := ""
			if err == nil {
				oldPath = route.Path
			} else if !errors.Is(err, sql.ErrNoRows) {
				return err
			}
			moves = append(moves, struct{ entryID, oldPath, newPath string }{entryID: id, oldPath: oldPath, newPath: newPath})
		}
	} else {
		// Flat: bounded batches.
		const batchSize = 500
		var allRows []db.ListPublishedEntriesByContentTypeRow
		for offset := int64(0); ; offset += batchSize {
			batch, err := q.ListPublishedEntriesByContentType(ctx, db.ListPublishedEntriesByContentTypeParams{ContentTypeID: contentType, Limit: batchSize, Offset: offset})
			if err != nil {
				return fmt.Errorf("list published entries: %w", err)
			}
			if len(batch) == 0 {
				break
			}
			allRows = append(allRows, batch...)
			if int64(len(batch)) < batchSize {
				break
			}
		}
		moves = make([]struct{ entryID, oldPath, newPath string }, 0, len(allRows))
		for _, row := range allRows {
			newPath := NormalizePath(newBase + "/" + strings.Trim(row.Slug, "/"))
			moves = append(moves, struct{ entryID, oldPath, newPath string }{entryID: row.ID, oldPath: row.RoutePath, newPath: newPath})
		}
	}
	for _, m := range moves {
		existing, err := q.GetRouteByPath(ctx, m.newPath)
		if err == nil && (!existing.EntryID.Valid || existing.EntryID.String != m.entryID) {
			return fmt.Errorf("route %s already exists", m.newPath)
		}
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
	}
	archiveRoute, archiveErr := q.GetArchiveRouteByContentType(ctx, sql.NullString{String: contentType, Valid: true})
	if archiveErr != nil && !errors.Is(archiveErr, sql.ErrNoRows) {
		return archiveErr
	}
	if archive {
		existing, err := q.GetRouteByPath(ctx, newBase)
		if err == nil && (archiveErr != nil || existing.ID != archiveRoute.ID) {
			return fmt.Errorf("route %s already exists", newBase)
		}
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
	}
	for _, m := range moves {
		if m.oldPath != m.newPath {
			if err := UpsertEntryRoute(ctx, q, m.entryID, m.newPath, now); err != nil {
				return err
			}
		}
	}
	if archive {
		if errors.Is(archiveErr, sql.ErrNoRows) {
			id, err := randomID()
			if err != nil {
				return err
			}
			if err := q.CreateRoute(ctx, db.CreateRouteParams{ID: id, Path: newBase, RouteType: RouteTypeArchive, ContentTypeID: sql.NullString{String: contentType, Valid: true}, CreatedAt: now, UpdatedAt: now}); err != nil {
				return err
			}
		}
		if archiveErr == nil && archiveRoute.Path != newBase {
			old := archiveRoute.Path
			if err := q.UpdateRoute(ctx, db.UpdateRouteParams{ID: archiveRoute.ID, Path: newBase, RouteType: RouteTypeArchive, ContentTypeID: sql.NullString{String: contentType, Valid: true}, UpdatedAt: now}); err != nil {
				return err
			}
			if err := UpsertRedirectRoute(ctx, q, old, newBase, now); err != nil {
				return err
			}
		}
	} else if archiveErr == nil {
		if err := q.DeleteRoute(ctx, archiveRoute.ID); err != nil {
			return err
		}
	}
	return nil
}

// hierarchy helpers for SyncContentTypeRouting (tree-aware base move).
type hierarchyNodeLite struct {
	ID       string
	Slug     string
	ParentID string
}

func isHierarchicalContentType(ctx context.Context, q *db.Queries, contentType string) (bool, string) {
	// Lightweight check: query content_types row directly; fall back to definition.
	row, err := q.GetContentType(ctx, contentType)
	if err != nil {
		return false, ""
	}
	isHier := row.Hierarchical == 1
	// Decode base for hierarchicalDef (reused as routing base).
	if isHier {
		// Use row's base path as hierarchicalDef fallback; full decode via content package is heavier.
		// We extract from config_json via simple parse; failure still returns base as empty.
		return true, row.ConfigJson
	}
	return false, ""
}

func computeHierarchyPaths(ctx context.Context, q *db.Queries, configJson, newBase string, nodes []hierarchyNodeLite) (map[string]string, error) {
	// Build parent map and compute paths via BFS from roots.
	byID := make(map[string]hierarchyNodeLite, len(nodes))
	children := make(map[string][]string)
	var roots []string
	for _, n := range nodes {
		byID[n.ID] = n
		if n.ParentID == "" {
			roots = append(roots, n.ID)
		} else {
			children[n.ParentID] = append(children[n.ParentID], n.ID)
		}
	}
	settings, _ := q.GetSiteSettings(ctx)
	postsBase := ""
	if settings.PostsBasePath != "" {
		postsBase = settings.PostsBasePath
	}
	// Resolve def for new base without full catalog decode: build minimal definition.
	// For flat hierarchical non-post types, root path is simply newBase + "/" + slug.
	// We avoid importing content to keep cycle free; do direct string logic.
	desired := make(map[string]string, len(nodes))
	var dfs func(id, parentPath string) error
	dfs = func(id, parentPath string) error {
		node, ok := byID[id]
		if !ok {
			return nil
		}
		var path string
		if node.ParentID == "" {
			s := strings.Trim(node.Slug, "/")
			if s == "" {
				path = "/"
			} else if newBase != "" {
				path = NormalizePath(newBase + "/" + s)
			} else if postsBase != "" {
				_ = postsBase
				path = "/" + s
			} else {
				path = "/" + s
			}
			if settings.HomepageEntryID.Valid && settings.HomepageEntryID.String == id {
				path = "/"
			}
		} else {
			// Child: derive from parent effective path.
			if parentPath == "/" {
				path = "/" + strings.Trim(node.Slug, "/")
			} else {
				path = NormalizePath(parentPath + "/" + strings.Trim(node.Slug, "/"))
			}
		}
		if err := validateHierarchyPath(path); err != nil {
			return err
		}
		desired[id] = path
		for _, childID := range children[id] {
			if err := dfs(childID, path); err != nil {
				return err
			}
		}
		return nil
	}
	for _, r := range roots {
		if err := dfs(r, ""); err != nil {
			return nil, err
		}
	}
	// Also handle any disconnected nodes (orphaned roots not in roots slice due to missing parent filtered out earlier)
	for _, n := range nodes {
		if _, ok := desired[n.ID]; !ok {
			// Treat as root if parent missing
			s := strings.Trim(n.Slug, "/")
			path := NormalizePath(newBase + "/" + s)
			desired[n.ID] = path
		}
	}
	_ = configJson
	return desired, nil
}

// SyncPostsPageSlugChanged moves the posts archive and post singles when the Posts Page slug changes.
func SyncPostsPageSlugChanged(ctx context.Context, q *db.Queries, entryID, newSlug, oldBase, newBase, homepageMode string, now int64) error {
	if newSlug == "" {
		return errors.New("new slug is required")
	}
	oldArch := PostsArchivePath(oldBase)
	newArch := PostsArchivePath(newBase)
	if homepageMode == "latest_posts" {
		return updatePostsBasePathOnly(ctx, q, newBase, now)
	}
	if oldArch == newArch {
		return updatePostsBasePathOnly(ctx, q, newBase, now)
	}
	if err := ValidatePostsBasePath(newBase); err != nil {
		return err
	}
	if rt, err := q.GetRouteByPath(ctx, oldArch); err == nil && rt.RouteType == RouteTypeArchive {
		if existing, er := q.GetRouteByPath(ctx, newArch); er == nil {
			if existing.RouteType == RouteTypeArchive {
				// Archive already at new path – keep old as redirect later.
			} else {
				// Collision: new archive path occupied by entry or redirect.
				return fmt.Errorf("a route already uses this slug")
			}
		} else if !errors.Is(er, sql.ErrNoRows) {
			return fmt.Errorf("check new archive route: %w", er)
		} else {
			if err := q.UpdateRoute(ctx, db.UpdateRouteParams{
				ID: rt.ID, Path: newArch, EntryID: rt.EntryID, RouteType: RouteTypeArchive,
				ContentTypeID: sql.NullString{String: "post", Valid: true}, UpdatedAt: now,
			}); err != nil {
				return fmt.Errorf("move archive route: %w", err)
			}
		}
	}
	if err := UpsertRedirectRoute(ctx, q, oldArch, newArch, now); err != nil {
		return fmt.Errorf("redirect archive: %w", err)
	}
	posts, _ := q.ListEntriesByContentType(ctx, "post")
	for _, p := range posts {
		if p.Status != "active" {
			continue
		}
		rt, rerr := q.GetEntryRoute(ctx, sql.NullString{String: p.ID, Valid: true})
		if rerr != nil {
			continue
		}
		if !strings.HasPrefix(rt.Path, oldArch+"/") && rt.Path != oldArch {
			continue
		}
		newP := EntryPath("post", p.Slug, newBase)
		if newP == rt.Path {
			continue
		}
		// Collision check for post single new path
		if byPath, err := q.GetRouteByPath(ctx, newP); err == nil && byPath.EntryID.Valid && byPath.EntryID.String != p.ID {
			return fmt.Errorf("a route already uses this slug")
		} else if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("check post route collision: %w", err)
		} else if err == nil && !byPath.EntryID.Valid {
			// Stale redirect at newP – clear it so post owns the path.
			if delErr := q.DeleteRoute(ctx, byPath.ID); delErr != nil {
				return fmt.Errorf("clear stale redirect for post %s: %w", p.ID, delErr)
			}
		}
		oldPath := rt.Path
		if err := q.UpdateRoute(ctx, db.UpdateRouteParams{
			ID: rt.ID, Path: newP, EntryID: sql.NullString{String: p.ID, Valid: true},
			RouteType: RouteTypeEntry, ContentTypeID: sql.NullString{}, UpdatedAt: now,
		}); err != nil {
			return fmt.Errorf("remap post %s: %w", p.ID, err)
		}
		if err := UpsertRedirectRoute(ctx, q, oldPath, newP, now); err != nil {
			return fmt.Errorf("redirect post %s: %w", p.ID, err)
		}
	}
	if err := updatePostsBasePathOnly(ctx, q, newBase, now); err != nil {
		return err
	}
	if _, err := q.GetRouteByPath(ctx, newArch); errors.Is(err, sql.ErrNoRows) {
		id, err := randomID()
		if err != nil {
			return err
		}
		if err := q.CreateRoute(ctx, db.CreateRouteParams{
			ID: id, Path: newArch, EntryID: sql.NullString{Valid: false},
			RouteType: RouteTypeArchive, ContentTypeID: sql.NullString{String: "post", Valid: true},
			CreatedAt: now, UpdatedAt: now,
		}); err != nil {
			return err
		}
	}
	return nil
}

func updatePostsBasePathOnly(ctx context.Context, q *db.Queries, newBase string, now int64) error {
	row, err := q.GetSiteSettings(ctx)
	if err != nil {
		return err
	}
	return q.UpdateSiteSettings(ctx, db.UpdateSiteSettingsParams{
		SiteTitle: row.SiteTitle, SiteTagline: row.SiteTagline, HomepageMode: row.HomepageMode,
		HomepageEntryID: row.HomepageEntryID, PostsPageEntryID: row.PostsPageEntryID,
		PostsPerPage: row.PostsPerPage, PostsBasePath: newBase, Language: row.Language, Timezone: row.Timezone,
		ActiveTheme: row.ActiveTheme, IndexingEnabled: row.IndexingEnabled, SiteUrl: row.SiteUrl,
		SitemapEnabled: row.SitemapEnabled, RobotsMode: row.RobotsMode, RobotsCustom: row.RobotsCustom,
		SpeculationMode: row.SpeculationMode, SpeculationEagerness: row.SpeculationEagerness,
		TitleSeparator: row.TitleSeparator, SiteSocialMediaID: row.SiteSocialMediaID,
		TwitterSite: row.TwitterSite, SiteRepresents: row.SiteRepresents, UpdatedAt: now,
	})
}

func randomID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
