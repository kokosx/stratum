package routing

import "database/sql"

// RouteType enumerates the kinds of public routes.
const (
	RouteTypeEntry    = "entry"
	RouteTypeArchive  = "archive"
	RouteTypeRedirect = "redirect"
	RouteTypeSystem   = "system"
)

// Route is the domain view of a routes row.
// content_type_id is nullable and only set for archive routes (e.g. archive of "post").
type Route struct {
	ID             string
	Path           string
	EntryID        sql.NullString
	RouteType      string
	ContentTypeID  sql.NullString
	RedirectTo     sql.NullString
	RedirectStatus sql.NullInt64
	CreatedAt      int64
	UpdatedAt      int64
}

// IsArchive reports whether this route is an archive.
func (r Route) IsArchive() bool { return r.RouteType == RouteTypeArchive }

// IsEntry reports whether this route is a single entry.
func (r Route) IsEntry() bool { return r.RouteType == RouteTypeEntry }

// ArchiveContentType returns the content type for an archive route, or "" if not an archive.
func (r Route) ArchiveContentType() string {
	if !r.IsArchive() || !r.ContentTypeID.Valid {
		return ""
	}
	return r.ContentTypeID.String
}
