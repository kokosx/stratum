package content

import (
	"context"
	"database/sql"
	"strings"

	db "github.com/kokosx/stratum/internal/storage/sqlc"
)

type AdminStatus string

const (
	AdminStatusAll       AdminStatus = "all"
	AdminStatusPublished AdminStatus = "published"
	AdminStatusDraft     AdminStatus = "draft"
	AdminStatusPrivate   AdminStatus = "private"
	AdminStatusTrash     AdminStatus = "trash"
)

func NormalizeAdminStatus(raw string) AdminStatus {
	s := strings.ToLower(strings.TrimSpace(raw))
	switch s {
	case "published":
		return AdminStatusPublished
	case "draft":
		return AdminStatusDraft
	case "private":
		return AdminStatusPrivate
	case "trash":
		return AdminStatusTrash
	case "all", "":
		return AdminStatusAll
	default:
		return AdminStatusAll
	}
}

type EntryAdminListQuery struct {
	ContentType ContentTypeID
	Search      string
	Status      AdminStatus
	Page        int
	PerPage     int
	AuthorID    string
}

func (q EntryAdminListQuery) Normalized() EntryAdminListQuery {
	if q.Page <= 0 {
		q.Page = 1
	}
	if q.PerPage <= 0 {
		q.PerPage = 20
	}
	if q.PerPage > 100 {
		q.PerPage = 100
	}
	q.Search = strings.TrimSpace(q.Search)
	if q.Status == "" {
		q.Status = AdminStatusAll
	}
	switch q.Status {
	case AdminStatusAll, AdminStatusPublished, AdminStatusDraft, AdminStatusPrivate, AdminStatusTrash:
	default:
		q.Status = AdminStatusAll
	}
	return q
}

type EntryStatusCounts struct {
	All       int64
	Published int64
	Draft     int64
	Private   int64
	Trash     int64
}

type AdminListResult struct {
	Entries []AdminEntry
	Counts  EntryStatusCounts
	Total   int64
	Page    int
}

// AdminEntry is the admin-list boundary. Storage rows stay inside the repository.
type AdminEntry struct {
	ID                  string
	Slug                string
	Status              string
	UpdatedAt           int64
	PublishedRevisionID string
	Title               string
	PublicPath          string
}

func (r *Repository) AdminList(ctx context.Context, q EntryAdminListQuery) (*AdminListResult, error) {
	q = q.Normalized()
	statusFilter := string(q.Status)
	if q.Status == AdminStatusAll {
		statusFilter = "all"
	}
	total, err := r.queries.CountEntriesAdmin(ctx, db.CountEntriesAdminParams{
		ContentTypeID: string(q.ContentType),
		StatusFilter:  statusFilter,
		Search:        q.Search,
		AuthorID:      nullableString(q.AuthorID),
	})
	if err != nil {
		return nil, err
	}
	totalPages := int((total + int64(q.PerPage) - 1) / int64(q.PerPage))
	if totalPages < 1 {
		totalPages = 1
	}
	if q.Page > totalPages {
		q.Page = totalPages
	}
	rows, err := r.queries.ListEntriesAdmin(ctx, db.ListEntriesAdminParams{
		ContentTypeID: string(q.ContentType),
		StatusFilter:  statusFilter,
		Search:        q.Search,
		AuthorID:      nullableString(q.AuthorID),
		Limit:         int64(q.PerPage),
		Offset:        int64((q.Page - 1) * q.PerPage),
	})
	if err != nil {
		return nil, err
	}
	countsRow, err := r.queries.CountEntriesByAdminStatus(ctx, db.CountEntriesByAdminStatusParams{ContentTypeID: string(q.ContentType), AuthorID: nullableString(q.AuthorID)})
	if err != nil {
		return nil, err
	}
	counts := EntryStatusCounts{
		All:       nullFloatToInt(countsRow.AllCount),
		Published: nullFloatToInt(countsRow.PublishedCount),
		Draft:     nullFloatToInt(countsRow.DraftCount),
		Private:   nullFloatToInt(countsRow.PrivateCount),
		Trash:     nullFloatToInt(countsRow.TrashCount),
	}
	entries := make([]AdminEntry, 0, len(rows))
	for _, row := range rows {
		entry := AdminEntry{ID: row.ID, Slug: row.Slug, Status: row.Status, UpdatedAt: row.UpdatedAt}
		if row.PublishedRevisionID.Valid {
			entry.PublishedRevisionID = row.PublishedRevisionID.String
		}
		if row.Title.Valid {
			entry.Title = row.Title.String
		}
		if row.PublicPath.Valid {
			entry.PublicPath = row.PublicPath.String
		}
		entries = append(entries, entry)
	}
	return &AdminListResult{
		Entries: entries,
		Counts:  counts,
		Total:   total,
		Page:    q.Page,
	}, nil
}

func nullableString(value string) sql.NullString {
	return sql.NullString{String: value, Valid: value != ""}
}

func nullFloatToInt(v sql.NullFloat64) int64 {
	if v.Valid {
		return int64(v.Float64)
	}
	return 0
}
