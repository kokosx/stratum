package content

import "database/sql"

// Entry is the domain view of an entries row, decoupled from sqlc.
type Entry struct {
	ID                  string
	ContentTypeID       ContentTypeID
	Slug                string
	Status              string
	AuthorID            sql.NullString
	PublishedRevisionID sql.NullString
	CreatedAt           int64
	UpdatedAt           int64
	PublishedAt         sql.NullInt64
	FirstPublishedAt    sql.NullInt64
}

// PublishedEntry is the view returned by EntryQuery (joined with revision + route).
type PublishedEntry struct {
	ID               string
	Slug             string
	ContentTypeID    ContentTypeID
	Title            string
	Excerpt          string
	FeaturedMediaID  sql.NullString
	RoutePath        string
	PublishedAt      sql.NullInt64
	FirstPublishedAt sql.NullInt64
	RevisionID       string
}
