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
	Entries []db.ListEntriesAdminRow
	Counts  EntryStatusCounts
	Total   int64
}

func (r *Repository) AdminList(ctx context.Context, q EntryAdminListQuery) (*AdminListResult, error) {
	q = q.Normalized()
	offset := int64((q.Page - 1) * q.PerPage)
	limit := int64(q.PerPage)
	statusFilter := string(q.Status)
	if q.Status == AdminStatusAll {
		statusFilter = "all"
	}
	rows, err := r.queries.ListEntriesAdmin(ctx, db.ListEntriesAdminParams{
		ContentTypeID: string(q.ContentType),
		StatusFilter:  statusFilter,
		Search:        q.Search,
		Limit:         limit,
		Offset:        offset,
	})
	if err != nil {
		return nil, err
	}
	total, err := r.queries.CountEntriesAdmin(ctx, db.CountEntriesAdminParams{
		ContentTypeID: string(q.ContentType),
		StatusFilter:  statusFilter,
		Search:        q.Search,
	})
	if err != nil {
		return nil, err
	}
	countsRow, err := r.queries.CountEntriesByAdminStatus(ctx, string(q.ContentType))
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
	return &AdminListResult{
		Entries: rows,
		Counts:  counts,
		Total:   total,
	}, nil
}

func nullFloatToInt(v sql.NullFloat64) int64 {
	if v.Valid {
		return int64(v.Float64)
	}
	return 0
}
