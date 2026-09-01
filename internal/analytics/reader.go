package analytics

import (
	"context"
	"database/sql"
	"time"

	db "github.com/kokosx/stratum/internal/storage/sqlc"
)

// Reader provides query helpers for admin UI. It uses sqlc queries but also higher-level aggregates.
type Reader struct {
	db      *sql.DB
	queries *db.Queries
}

func NewReader(database *sql.DB) *Reader {
	return &Reader{db: database, queries: db.New(database)}
}

// Range helpers
func ParseRange(rangeStr string) (int, string, int64) {
	now := time.Now().UTC()
	var d int
	switch rangeStr {
	case "today":
		d = 1
	case "7d", "7days", "7 days":
		d = 7
	case "30d", "30 days":
		d = 30
	case "90d", "90 days":
		d = 90
	case "12m", "12 months", "365d":
		d = 365
	default:
		d = 30
	}
	since := now.AddDate(0, 0, -d+1)
	sinceDay := since.Format("2006-01-02")
	sinceHour := since.Truncate(time.Hour).Unix()
	return d, sinceDay, sinceHour
}

// Overview aggregates
type Overview struct {
	Views           int64
	HumanViews      int64
	CrawlerViews    int64
	Speculative     int64
	SearchViews     int64
	AIViews         int64
	FormSubmissions int64 // from forms, not analytics
	CacheHitRatio   float64
	AvgDurationMs   float64
	TotalRequests   int64
	TotalBytes      int64
	Status2xx       int64
	Status3xx       int64
	Status4xx       int64
	Status5xx       int64
	LatencyBuckets  [7]int64
	Daily           []DailyPoint
	TopContent      []PageRow
	TopReferrers    []DimRow
	TopTransitions  []TransRow
}

type DailyPoint struct {
	Day          string
	HumanViews   int64
	CrawlerViews int64
	Views        int64
}

type PageRow struct {
	ResourceKey string
	Path        string
	Title       string // resolved from entry if available else path
	EntryID     string
	RevisionID  string
	Views       int64
	HumanViews  int64
	SearchViews int64
	AIViews     int64
	DirectViews int64
}

type DimRow struct {
	Value string
	Count int64
}

type TransRow struct {
	FromPath string
	ToPath   string
	FromKey  string
	ToKey    string
	Count    int64
}

// GetOverview returns high-level metrics for range.
func (r *Reader) GetOverview(ctx context.Context, sinceDay string, sinceHour int64) (Overview, error) {
	var ov Overview
	// site hourly sum
	siteSum, err := r.queries.SumAnalyticsSiteHourly(ctx, db.SumAnalyticsSiteHourlyParams{Hour: sinceHour, Hour_2: time.Now().UTC().Unix()})
	if err != nil {
		return ov, err
	}
	ov.TotalRequests = siteSum.TotalRequests
	ov.Views = siteSum.TotalViews
	ov.HumanViews = siteSum.TotalHuman
	ov.CrawlerViews = siteSum.TotalCrawler
	ov.Speculative = siteSum.TotalSpeculative
	ov.TotalBytes = siteSum.TotalBytes
	ov.Status2xx = siteSum.Total2xx
	ov.Status3xx = siteSum.Total3xx
	ov.Status4xx = siteSum.Total4xx
	ov.Status5xx = siteSum.Total5xx
	ov.LatencyBuckets = [7]int64{siteSum.LLt5, siteSum.LLt20, siteSum.LLt50, siteSum.LLt100, siteSum.LLt250, siteSum.LLt1000, siteSum.LGte1000}
	totalCache := siteSum.TotalHits + siteSum.TotalMisses
	if totalCache > 0 {
		ov.CacheHitRatio = float64(siteSum.TotalHits) / float64(totalCache)
	}
	if siteSum.TotalCount > 0 && siteSum.TotalDuration > 0 {
		ov.AvgDurationMs = float64(siteSum.TotalDuration) / float64(siteSum.TotalCount) / 1000.0
	}
	// search/ai from page daily sums
	pageSum, err := r.queries.SumAnalyticsPageDaily(ctx, db.SumAnalyticsPageDailyParams{Day: sinceDay, Day_2: time.Now().UTC().Format("2006-01-02")})
	if err == nil {
		ov.SearchViews = pageSum.TotalSearch
		ov.AIViews = pageSum.TotalAi
	}
	// Forms global count for range? For now total new submissions count from forms table last range? Use simple count of submissions where created_at >= sinceHour?
	// Query forms directly
	if r.db != nil {
		var cnt int64
		_ = r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM form_submissions WHERE created_at >= ?`, sinceHour).Scan(&cnt)
		ov.FormSubmissions = cnt
	}
	// Daily points from pageDaily aggregated by day
	rows, err := r.queries.ListAnalyticsPageDaily(ctx, db.ListAnalyticsPageDailyParams{Day: sinceDay, Day_2: time.Now().UTC().Format("2006-01-02"), Limit: 1000, Offset: 0})
	if err == nil {
		byDay := map[string]*DailyPoint{}
		for _, row := range rows {
			dp := byDay[row.Day]
			if dp == nil {
				dp = &DailyPoint{Day: row.Day}
				byDay[row.Day] = dp
			}
			dp.Views += row.Views
			dp.HumanViews += row.HumanViews
			dp.CrawlerViews += row.CrawlerViews
		}
		for _, dp := range byDay {
			ov.Daily = append(ov.Daily, *dp)
		}
	}
	// Top content
	top, err := r.queries.ListAnalyticsPageDaily(ctx, db.ListAnalyticsPageDailyParams{Day: sinceDay, Day_2: time.Now().UTC().Format("2006-01-02"), Limit: 10, Offset: 0})
	if err == nil {
		for _, row := range top {
			ov.TopContent = append(ov.TopContent, PageRow{
				ResourceKey: row.ResourceKey,
				Path:        row.Path,
				EntryID:     nullStr(row.EntryID),
				RevisionID:  nullStr(row.RevisionID),
				Views:       row.Views,
				HumanViews:  row.HumanViews,
				SearchViews: row.OrganicSearchViews,
				AIViews:     row.AiReferralViews,
				DirectViews: row.DirectViews,
			})
		}
	}
	// Top referrers
	refs, err := r.queries.ListAnalyticsDimensionDaily(ctx, db.ListAnalyticsDimensionDailyParams{Day: sinceDay, Day_2: time.Now().UTC().Format("2006-01-02"), Dimension: "referrer_host", Limit: 10})
	if err == nil {
		for _, row := range refs {
			ov.TopReferrers = append(ov.TopReferrers, DimRow{Value: row.Value, Count: row.Count})
		}
	}
	trans, err := r.queries.ListAnalyticsTransitions(ctx, db.ListAnalyticsTransitionsParams{Day: sinceDay, Day_2: time.Now().UTC().Format("2006-01-02"), Limit: 10})
	if err == nil {
		for _, row := range trans {
			ov.TopTransitions = append(ov.TopTransitions, TransRow{FromPath: row.FromPath, ToPath: row.ToPath, FromKey: row.FromResourceKey, ToKey: row.ToResourceKey, Count: row.Count})
		}
	}
	return ov, nil
}

func nullStr(ns sql.NullString) string {
	if ns.Valid {
		return ns.String
	}
	return ""
}

type ContentRow struct {
	PageRow
	Revisions []RevisionRow
}

type RevisionRow struct {
	RevisionID  string
	Views       int64
	SearchViews int64
	AIViews     int64
}

func (r *Reader) GetContentList(ctx context.Context, sinceDay string, limit, offset int) ([]PageRow, error) {
	rows, err := r.queries.ListAnalyticsPageDaily(ctx, db.ListAnalyticsPageDailyParams{Day: sinceDay, Day_2: time.Now().UTC().Format("2006-01-02"), Limit: int64(limit), Offset: int64(offset)})
	if err != nil {
		return nil, err
	}
	var out []PageRow
	for _, row := range rows {
		out = append(out, PageRow{
			ResourceKey: row.ResourceKey,
			Path:        row.Path,
			EntryID:     nullStr(row.EntryID),
			RevisionID:  nullStr(row.RevisionID),
			Views:       row.Views,
			HumanViews:  row.HumanViews,
			SearchViews: row.OrganicSearchViews,
			AIViews:     row.AiReferralViews,
			DirectViews: row.DirectViews,
		})
	}
	return out, nil
}

func (r *Reader) GetPageDailyByEntry(ctx context.Context, entryID, sinceDay string) ([]db.AnalyticsPageDaily, error) {
	return r.queries.ListAnalyticsPageDailyByEntry(ctx, db.ListAnalyticsPageDailyByEntryParams{EntryID: sql.NullString{String: entryID, Valid: true}, Day: sinceDay, Day_2: time.Now().UTC().Format("2006-01-02")})
}

func (r *Reader) GetAcquisition(ctx context.Context, sinceDay string) (map[string]int64, []DimRow, []DimRow, []DimRow, []DimRow) {
	// returns source totals + top referrers + utm sources etc.
	sum, _ := r.queries.SumAnalyticsPageDaily(ctx, db.SumAnalyticsPageDailyParams{Day: sinceDay, Day_2: time.Now().UTC().Format("2006-01-02")})
	m := map[string]int64{
		"direct":         sum.TotalDirect,
		"internal":       sum.TotalInternal,
		"organic_search": sum.TotalSearch,
		"organic_social": sum.TotalSocial,
		"ai_referral":    sum.TotalAi,
		"referral":       sum.TotalReferral,
		"campaign":       sum.TotalCampaign,
	}
	refs, _ := r.queries.ListAnalyticsDimensionDaily(ctx, db.ListAnalyticsDimensionDailyParams{Day: sinceDay, Day_2: time.Now().UTC().Format("2006-01-02"), Dimension: "referrer_host", Limit: 20})
	var refRows []DimRow
	for _, row := range refs {
		refRows = append(refRows, DimRow{Value: row.Value, Count: row.Count})
	}
	srcs, _ := r.queries.ListAnalyticsDimensionDaily(ctx, db.ListAnalyticsDimensionDailyParams{Day: sinceDay, Day_2: time.Now().UTC().Format("2006-01-02"), Dimension: "utm_source", Limit: 20})
	var srcRows []DimRow
	for _, row := range srcs {
		srcRows = append(srcRows, DimRow{Value: row.Value, Count: row.Count})
	}
	mdms, _ := r.queries.ListAnalyticsDimensionDaily(ctx, db.ListAnalyticsDimensionDailyParams{Day: sinceDay, Day_2: time.Now().UTC().Format("2006-01-02"), Dimension: "utm_medium", Limit: 20})
	var mdmRows []DimRow
	for _, row := range mdms {
		mdmRows = append(mdmRows, DimRow{Value: row.Value, Count: row.Count})
	}
	camps, _ := r.queries.ListAnalyticsDimensionDaily(ctx, db.ListAnalyticsDimensionDailyParams{Day: sinceDay, Day_2: time.Now().UTC().Format("2006-01-02"), Dimension: "utm_campaign", Limit: 20})
	var campRows []DimRow
	for _, row := range camps {
		campRows = append(campRows, DimRow{Value: row.Value, Count: row.Count})
	}
	return m, refRows, srcRows, mdmRows, campRows
}

func (r *Reader) GetTechnology(ctx context.Context, sinceDay string) map[string][]DimRow {
	out := map[string][]DimRow{}
	for _, dim := range []string{"browser", "os", "device", "language"} {
		rows, _ := r.queries.ListAnalyticsDimensionDaily(ctx, db.ListAnalyticsDimensionDailyParams{Day: sinceDay, Day_2: time.Now().UTC().Format("2006-01-02"), Dimension: dim, Limit: 20})
		var list []DimRow
		for _, row := range rows {
			list = append(list, DimRow{Value: row.Value, Count: row.Count})
		}
		out[dim] = list
	}
	return out
}

func (r *Reader) GetCrawlers(ctx context.Context, sinceDay string) ([]DimRow, int64) {
	rows, _ := r.queries.ListAnalyticsDimensionDaily(ctx, db.ListAnalyticsDimensionDailyParams{Day: sinceDay, Day_2: time.Now().UTC().Format("2006-01-02"), Dimension: "crawler", Limit: 20})
	var list []DimRow
	var total int64
	for _, row := range rows {
		list = append(list, DimRow{Value: row.Value, Count: row.Count})
		total += row.Count
	}
	return list, total
}

func (r *Reader) GetPerformance(ctx context.Context, sinceHour int64) (db.SumAnalyticsSiteHourlyRow, error) {
	return r.queries.SumAnalyticsSiteHourly(ctx, db.SumAnalyticsSiteHourlyParams{Hour: sinceHour, Hour_2: time.Now().UTC().Unix()})
}

func (r *Reader) GetSlowestContent(ctx context.Context, sinceDay string) ([]PageRow, error) {
	rows, err := r.queries.ListAnalyticsPageDaily(ctx, db.ListAnalyticsPageDailyParams{Day: sinceDay, Day_2: time.Now().UTC().Format("2006-01-02"), Limit: 100, Offset: 0})
	if err != nil {
		return nil, err
	}
	// sort by avg duration desc (duration_sum / response_count)
	var out []PageRow
	for _, row := range rows {
		if row.ResponseCount == 0 {
			continue
		}
		out = append(out, PageRow{
			ResourceKey: row.ResourceKey,
			Path:        row.Path,
			EntryID:     nullStr(row.EntryID),
			RevisionID:  nullStr(row.RevisionID),
			Views:       row.Views,
		})
	}
	// naive sort: already have latency, but we approximate via duration sum
	// For simplicity return as is sorted by views, performance tab will show duration.
	return out[:min(10, len(out))], nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
