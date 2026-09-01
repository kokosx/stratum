package analytics

import (
	"context"
	"database/sql"
	"time"

	db "github.com/kokosx/stratum/internal/storage/sqlc"
)

// Store handles SQLite persistence for analytics.
// It performs batch UPSERTs in a single short transaction.
// It also enforces cardinality caps at persistence boundary (restart-safe).

type Store struct {
	db      *sql.DB
	queries *db.Queries
}

func NewStore(database *sql.DB) *Store {
	return &Store{db: database, queries: db.New(database)}
}

// Flush batch writes aggregates. It is called by worker with snapshot data (already swapped, no lock needed).
// It enforces dimension cardinality via DB queries before UPSERT.
func (s *Store) Flush(ctx context.Context, site map[int64]*siteHourlyAgg, page map[string]*pageDailyAgg, dim map[string]*dimAgg, trans map[string]*transAgg) error {
	if len(site) == 0 && len(page) == 0 && len(dim) == 0 && len(trans) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	qtx := s.queries.WithTx(tx)

	// Site hourly
	for hour, v := range site {
		err := qtx.UpsertAnalyticsSiteHourly(ctx, db.UpsertAnalyticsSiteHourlyParams{
			Hour:                hour,
			Requests:            v.Requests,
			Views:               v.Views,
			HumanViews:          v.HumanViews,
			CrawlerViews:        v.CrawlerViews,
			SpeculativeRequests: v.Speculative,
			CacheHits:           v.CacheHits,
			CacheMisses:         v.CacheMisses,
			Status2xx:           v.Status2xx,
			Status3xx:           v.Status3xx,
			Status4xx:           v.Status4xx,
			Status5xx:           v.Status5xx,
			ResponseBytes:       v.ResponseBytes,
			ResponseCount:       v.ResponseCount,
			ResponseDurationSum: v.ResponseDurationSum,
			LatencyLt5:          v.Latency[0],
			LatencyLt20:         v.Latency[1],
			LatencyLt50:         v.Latency[2],
			LatencyLt100:        v.Latency[3],
			LatencyLt250:        v.Latency[4],
			LatencyLt1000:       v.Latency[5],
			LatencyGte1000:      v.Latency[6],
		})
		if err != nil {
			return err
		}
	}

	// Page daily
	for _, v := range page {
		entryID := sql.NullString{}
		if v.Resource.EntryID != "" {
			entryID = sql.NullString{String: v.Resource.EntryID, Valid: true}
		}
		revID := sql.NullString{}
		if v.Resource.RevisionID != "" {
			revID = sql.NullString{String: v.Resource.RevisionID, Valid: true}
		}
		ctID := sql.NullString{}
		if v.Resource.ContentTypeID != "" {
			ctID = sql.NullString{String: v.Resource.ContentTypeID, Valid: true}
		}
		taxID := sql.NullString{}
		if v.Resource.TaxonomyID != "" {
			taxID = sql.NullString{String: v.Resource.TaxonomyID, Valid: true}
		}
		termID := sql.NullString{}
		if v.Resource.TermID != "" {
			termID = sql.NullString{String: v.Resource.TermID, Valid: true}
		}
		// Normalize route_type: ensure one of allowed values
		rt := v.Resource.RouteType
		if rt != "entry" && rt != "archive" && rt != "taxonomy" && rt != "system" {
			rt = "system"
		}
		err := qtx.UpsertAnalyticsPageDaily(ctx, db.UpsertAnalyticsPageDailyParams{
			Day:                 v.Day,
			ResourceKey:         v.Resource.Key,
			Path:                v.Resource.Path,
			RouteType:           rt,
			EntryID:             entryID,
			RevisionID:          revID,
			ContentTypeID:       ctID,
			TaxonomyID:          taxID,
			TermID:              termID,
			Views:               v.Views,
			HumanViews:          v.HumanViews,
			CrawlerViews:        v.CrawlerViews,
			DirectViews:         v.Direct,
			InternalViews:       v.Internal,
			OrganicSearchViews:  v.OrganicSearch,
			OrganicSocialViews:  v.OrganicSocial,
			AiReferralViews:     v.AIReferral,
			ReferralViews:       v.Referral,
			CampaignViews:       v.Campaign,
			CacheHits:           v.CacheHits,
			CacheMisses:         v.CacheMisses,
			ResponseCount:       v.ResponseCount,
			ResponseBytes:       v.ResponseBytes,
			ResponseDurationSum: v.DurationSum,
			LatencyLt5:          v.Latency[0],
			LatencyLt20:         v.Latency[1],
			LatencyLt50:         v.Latency[2],
			LatencyLt100:        v.Latency[3],
			LatencyLt250:        v.Latency[4],
			LatencyLt1000:       v.Latency[5],
			LatencyGte1000:      v.Latency[6],
		})
		if err != nil {
			return err
		}
	}

	// Dimension daily: need cardinality protection restart-safe.
	// For each distinct day|dimension, we will query existing distinct count and existing values set once per batch per day/dim, then map overflow to "other".
	// We collect pending dimensions grouped by day|dimension.
	type groupKey struct {
		day string
		dim string
	}
	grouped := map[groupKey]map[string]*dimAgg{}
	for _, v := range dim {
		gk := groupKey{day: v.Day, dim: v.Dimension}
		if grouped[gk] == nil {
			grouped[gk] = make(map[string]*dimAgg)
		}
		// merge counts for same value already in pending (could be multiple pending with same key? Map already deduped)
		grouped[gk][v.Value] = v
	}

	for gk, pendingVals := range grouped {
		// Query existing count and values
		existingCount, err := qtx.GetAnalyticsDimensionCount(ctx, db.GetAnalyticsDimensionCountParams{Day: gk.day, Dimension: gk.dim})
		if err != nil {
			return err
		}
		existingValsRows, err := qtx.ListAnalyticsDimensionValues(ctx, db.ListAnalyticsDimensionValuesParams{Day: gk.day, Dimension: gk.dim})
		if err != nil {
			return err
		}
		existingSet := make(map[string]bool, len(existingValsRows))
		for _, ev := range existingValsRows {
			existingSet[ev] = true
		}
		// Also consider "other" as already allowed even if we exceed
		// We'll decide for each pending value if it needs to go to other.
		otherAggregated := int64(0)
		for val, agg := range pendingVals {
			if val == "other" {
				// directly upsert other
				err := qtx.UpsertAnalyticsDimensionDaily(ctx, db.UpsertAnalyticsDimensionDailyParams{Day: agg.Day, Dimension: agg.Dimension, Value: agg.Value, Count: agg.Count})
				if err != nil {
					return err
				}
				delete(pendingVals, val)
				continue
			}
			if existingSet[val] {
				// already exists, safe
				err := qtx.UpsertAnalyticsDimensionDaily(ctx, db.UpsertAnalyticsDimensionDailyParams{Day: agg.Day, Dimension: agg.Dimension, Value: agg.Value, Count: agg.Count})
				if err != nil {
					return err
				}
				delete(pendingVals, val)
				continue
			}
			// Not existing: check if we would exceed cap
			if existingCount >= MaxDimensionCardinality {
				otherAggregated += agg.Count
				// don't insert this distinct value
				continue
			}
			// Under cap, we will insert new value, bump existingCount for next pending values in same batch
			err := qtx.UpsertAnalyticsDimensionDaily(ctx, db.UpsertAnalyticsDimensionDailyParams{Day: agg.Day, Dimension: agg.Dimension, Value: agg.Value, Count: agg.Count})
			if err != nil {
				return err
			}
			existingCount++
			existingSet[val] = true
			delete(pendingVals, val)
		}
		if otherAggregated > 0 {
			if err := qtx.UpsertAnalyticsDimensionDaily(ctx, db.UpsertAnalyticsDimensionDailyParams{Day: gk.day, Dimension: gk.dim, Value: "other", Count: otherAggregated}); err != nil {
				return err
			}
		}
	}

	// Transitions
	for _, v := range trans {
		err := qtx.UpsertAnalyticsTransitionDaily(ctx, db.UpsertAnalyticsTransitionDailyParams{
			Day:             v.Day,
			FromResourceKey: v.FromKey,
			ToResourceKey:   v.ToKey,
			FromPath:        v.FromPath,
			ToPath:          v.ToPath,
			Count:           v.Count,
		})
		if err != nil {
			return err
		}
	}

	if err := tx.Commit(); err != nil {
		return err
	}
	return nil
}

// Clear deletes all analytics tables. Must be called within worker serialized context.
func (s *Store) Clear(ctx context.Context) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	qtx := s.queries.WithTx(tx)
	if err := qtx.DeleteAllAnalyticsSiteHourly(ctx); err != nil {
		return err
	}
	if err := qtx.DeleteAllAnalyticsPageDaily(ctx); err != nil {
		return err
	}
	if err := qtx.DeleteAllAnalyticsDimensionDaily(ctx); err != nil {
		return err
	}
	if err := qtx.DeleteAllAnalyticsTransitionDaily(ctx); err != nil {
		return err
	}
	return tx.Commit()
}

// Retention deletes old rows before cutoff.
func (s *Store) Retention(ctx context.Context, retentionDays, hourlyRetentionDays int64) error {
	now := DayBucketNow()
	// compute cutoff days
	// day format YYYY-MM-DD, we need to delete where day < cutoffDay
	// hour cutoff is unix hour
	// For simplicity, compute using Go time
	cutoffDay := now.AddDate(0, 0, -int(retentionDays)).Format("2006-01-02")
	cutoffHour := now.AddDate(0, 0, -int(hourlyRetentionDays)).Truncate(time.Hour).Unix()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	qtx := s.queries.WithTx(tx)
	if err := qtx.DeleteAnalyticsSiteHourlyBefore(ctx, cutoffHour); err != nil {
		return err
	}
	if err := qtx.DeleteAnalyticsPageDailyBefore(ctx, cutoffDay); err != nil {
		return err
	}
	if err := qtx.DeleteAnalyticsDimensionDailyBefore(ctx, cutoffDay); err != nil {
		return err
	}
	if err := qtx.DeleteAnalyticsTransitionDailyBefore(ctx, cutoffDay); err != nil {
		return err
	}
	return tx.Commit()
}

// Helper for retention now
var DayBucketNow = func() time.Time { return time.Now().UTC() }
