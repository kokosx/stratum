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
}

func (r Route) IsArchive() bool { return r.RouteType == RouteTypeArchive }
func (r Route) IsEntry() bool   { return r.RouteType == RouteTypeEntry }
func (r Route) ArchiveContentType() string {
	if !r.IsArchive() || !r.ContentTypeID.Valid {
		return ""
	}
	return r.ContentTypeID.String
}
