package routing

import "database/sql"

const (
	RouteTypeEntry    = "entry"
	RouteTypeArchive  = "archive"
	RouteTypeRedirect = "redirect"
	RouteTypeSystem   = "system"
)

type Route struct {
	ID             string
	Path           string
	EntryID        sql.NullString
	RouteType      string
	ContentTypeID  sql.NullString
	TaxonomyID     sql.NullString
	TermID         sql.NullString
	RedirectTo     sql.NullString
	RedirectStatus sql.NullInt64
	CreatedAt      int64
	UpdatedAt      int64
	// Publication metadata for entry routes (only set when RouteType == entry).
	PublishedRevisionID sql.NullString
	Visibility          string // "public", "password", "private" (private routes are deleted, so typically not present)
}

func (r Route) IsArchive() bool { return r.RouteType == RouteTypeArchive }
func (r Route) IsEntry() bool   { return r.RouteType == RouteTypeEntry }
func (r Route) ArchiveContentType() string {
	if !r.IsArchive() || !r.ContentTypeID.Valid {
		return ""
	}
	return r.ContentTypeID.String
}
