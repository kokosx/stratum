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
			RedirectTo: sql.NullString{String: target, Valid: true},
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
			RedirectTo: sql.NullString{String: target, Valid: true},
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
		RedirectTo: sql.NullString{String: target, Valid: true},
		RedirectStatus: sql.NullInt64{Int64: http.StatusMovedPermanently, Valid: true}, CreatedAt: now, UpdatedAt: now,
	})
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
