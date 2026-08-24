package routing

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"strings"

	db "github.com/kokosx/stratum/internal/storage/sqlc"
)

// ResolvedRoute is the result of resolving a public path before rendering.
type ResolvedRoute struct {
	Path       string
	Route      *db.Route
	IsArchive  bool
	IsRedirect bool
	RedirectTo string
	Pagination PaginationResolution
}

// PaginationResolution holds pagination parsing for archive children.
type PaginationResolution struct {
	IsPagination bool
	BasePath     string
	Page         int
}

// Resolver is the single place that maps an incoming request path to a route,
// handling exact routes, pagination children (/blog/page/2), and redirect routes
// that were left by slug changes. Handlers must not implement this logic inline.
type Resolver struct {
	queries *db.Queries
}

// NewResolver creates a resolver.
func NewResolver(queries *db.Queries) *Resolver { return &Resolver{queries: queries} }

// Resolve returns the route for path, checking exact route, then pagination, then fallback.
func (r *Resolver) Resolve(ctx context.Context, path string) (ResolvedRoute, error) {
	path = NormalizePath(path)
	if route, err := r.queries.GetRouteByPath(ctx, path); err == nil {
		return ResolvedRoute{
			Path:       path,
			Route:      &route,
			IsArchive:  route.RouteType == RouteTypeArchive,
			IsRedirect: route.RouteType == RouteTypeRedirect,
			RedirectTo: route.RedirectTo.String,
		}, nil
	} else if !errors.Is(err, sql.ErrNoRows) {
		return ResolvedRoute{}, err
	}
	// Pagination child: /blog/page/2 or /page/2
	if base, page, ok := ParsePagination(path); ok {
		if rt, err := r.queries.GetRouteByPath(ctx, base); err == nil && rt.RouteType == RouteTypeArchive {
			return ResolvedRoute{
				Path:       path,
				Route:      &rt,
				IsArchive:  true,
				Pagination: PaginationResolution{IsPagination: true, BasePath: base, Page: page},
			}, nil
		}
		if base == "/" {
			// Home archive (latest_posts mode) has no route row at "/" when not configured as archive.
			// Caller checks site snapshot homepageMode separately.
			return ResolvedRoute{
				Path:       path,
				Pagination: PaginationResolution{IsPagination: true, BasePath: base, Page: page},
			}, nil
		}
	}
	return ResolvedRoute{}, sql.ErrNoRows
}

// ParsePagination extracts base and page for paths like /blog/page/3. Returns ok=false for non-pagination.
func ParsePagination(path string) (base string, page int, ok bool) {
	path = strings.TrimSuffix(path, "/")
	if strings.HasSuffix(path, "/page/1") {
		base = strings.TrimSuffix(path, "/page/1")
		if base == "" {
			base = "/"
		}
		return base, 1, true
	}
	if idx := strings.LastIndex(path, "/page/"); idx != -1 {
		suffix := path[idx+6:]
		if n, err := strconv.Atoi(suffix); err == nil && n > 1 {
			base = path[:idx]
			if base == "" {
				base = "/"
			}
			return base, n, true
		}
	}
	return "", 0, false
}

// RedirectStatus returns the HTTP status for a redirect route, defaulting to 301.
func (r *Resolver) RedirectStatus(route db.Route) int {
	if route.RedirectStatus.Valid && route.RedirectStatus.Int64 != 0 {
		return int(route.RedirectStatus.Int64)
	}
	return http.StatusMovedPermanently
}
