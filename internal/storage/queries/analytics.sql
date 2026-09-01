-- Site hourly upsert
-- name: UpsertAnalyticsSiteHourly :exec
INSERT INTO analytics_site_hourly (
    hour, requests, views, human_views, crawler_views, speculative_requests,
    cache_hits, cache_misses, status_2xx, status_3xx, status_4xx, status_5xx,
    response_bytes, response_count, response_duration_sum,
    latency_lt5, latency_lt20, latency_lt50, latency_lt100, latency_lt250, latency_lt1000, latency_gte1000
) VALUES (
    ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?
) ON CONFLICT(hour) DO UPDATE SET
    requests = analytics_site_hourly.requests + excluded.requests,
    views = analytics_site_hourly.views + excluded.views,
    human_views = analytics_site_hourly.human_views + excluded.human_views,
    crawler_views = analytics_site_hourly.crawler_views + excluded.crawler_views,
    speculative_requests = analytics_site_hourly.speculative_requests + excluded.speculative_requests,
    cache_hits = analytics_site_hourly.cache_hits + excluded.cache_hits,
    cache_misses = analytics_site_hourly.cache_misses + excluded.cache_misses,
    status_2xx = analytics_site_hourly.status_2xx + excluded.status_2xx,
    status_3xx = analytics_site_hourly.status_3xx + excluded.status_3xx,
    status_4xx = analytics_site_hourly.status_4xx + excluded.status_4xx,
    status_5xx = analytics_site_hourly.status_5xx + excluded.status_5xx,
    response_bytes = analytics_site_hourly.response_bytes + excluded.response_bytes,
    response_count = analytics_site_hourly.response_count + excluded.response_count,
    response_duration_sum = analytics_site_hourly.response_duration_sum + excluded.response_duration_sum,
    latency_lt5 = analytics_site_hourly.latency_lt5 + excluded.latency_lt5,
    latency_lt20 = analytics_site_hourly.latency_lt20 + excluded.latency_lt20,
    latency_lt50 = analytics_site_hourly.latency_lt50 + excluded.latency_lt50,
    latency_lt100 = analytics_site_hourly.latency_lt100 + excluded.latency_lt100,
    latency_lt250 = analytics_site_hourly.latency_lt250 + excluded.latency_lt250,
    latency_lt1000 = analytics_site_hourly.latency_lt1000 + excluded.latency_lt1000,
    latency_gte1000 = analytics_site_hourly.latency_gte1000 + excluded.latency_gte1000;

-- name: ListAnalyticsSiteHourly :many
SELECT hour, requests, views, human_views, crawler_views, speculative_requests, cache_hits, cache_misses, status_2xx, status_3xx, status_4xx, status_5xx, response_bytes, response_count, response_duration_sum, latency_lt5, latency_lt20, latency_lt50, latency_lt100, latency_lt250, latency_lt1000, latency_gte1000
FROM analytics_site_hourly
WHERE hour >= ? AND hour <= ?
ORDER BY hour ASC;

-- name: DeleteAnalyticsSiteHourlyBefore :exec
DELETE FROM analytics_site_hourly WHERE hour < ?;

-- name: DeleteAllAnalyticsSiteHourly :exec
DELETE FROM analytics_site_hourly;

-- Page daily upsert
-- name: UpsertAnalyticsPageDaily :exec
INSERT INTO analytics_page_daily (
    day, resource_key, path, route_type, entry_id, revision_id, content_type_id, taxonomy_id, term_id,
    views, human_views, crawler_views,
    direct_views, internal_views, organic_search_views, organic_social_views, ai_referral_views, referral_views, campaign_views,
    cache_hits, cache_misses, response_count, response_bytes, response_duration_sum,
    latency_lt5, latency_lt20, latency_lt50, latency_lt100, latency_lt250, latency_lt1000, latency_gte1000
) VALUES (
    ?, ?, ?, ?, ?, ?, ?, ?, ?,
    ?, ?, ?,
    ?, ?, ?, ?, ?, ?, ?,
    ?, ?, ?, ?, ?,
    ?, ?, ?, ?, ?, ?, ?
) ON CONFLICT(day, resource_key) DO UPDATE SET
    path = excluded.path,
    route_type = excluded.route_type,
    entry_id = COALESCE(excluded.entry_id, analytics_page_daily.entry_id),
    revision_id = COALESCE(excluded.revision_id, analytics_page_daily.revision_id),
    content_type_id = COALESCE(excluded.content_type_id, analytics_page_daily.content_type_id),
    taxonomy_id = COALESCE(excluded.taxonomy_id, analytics_page_daily.taxonomy_id),
    term_id = COALESCE(excluded.term_id, analytics_page_daily.term_id),
    views = analytics_page_daily.views + excluded.views,
    human_views = analytics_page_daily.human_views + excluded.human_views,
    crawler_views = analytics_page_daily.crawler_views + excluded.crawler_views,
    direct_views = analytics_page_daily.direct_views + excluded.direct_views,
    internal_views = analytics_page_daily.internal_views + excluded.internal_views,
    organic_search_views = analytics_page_daily.organic_search_views + excluded.organic_search_views,
    organic_social_views = analytics_page_daily.organic_social_views + excluded.organic_social_views,
    ai_referral_views = analytics_page_daily.ai_referral_views + excluded.ai_referral_views,
    referral_views = analytics_page_daily.referral_views + excluded.referral_views,
    campaign_views = analytics_page_daily.campaign_views + excluded.campaign_views,
    cache_hits = analytics_page_daily.cache_hits + excluded.cache_hits,
    cache_misses = analytics_page_daily.cache_misses + excluded.cache_misses,
    response_count = analytics_page_daily.response_count + excluded.response_count,
    response_bytes = analytics_page_daily.response_bytes + excluded.response_bytes,
    response_duration_sum = analytics_page_daily.response_duration_sum + excluded.response_duration_sum,
    latency_lt5 = analytics_page_daily.latency_lt5 + excluded.latency_lt5,
    latency_lt20 = analytics_page_daily.latency_lt20 + excluded.latency_lt20,
    latency_lt50 = analytics_page_daily.latency_lt50 + excluded.latency_lt50,
    latency_lt100 = analytics_page_daily.latency_lt100 + excluded.latency_lt100,
    latency_lt250 = analytics_page_daily.latency_lt250 + excluded.latency_lt250,
    latency_lt1000 = analytics_page_daily.latency_lt1000 + excluded.latency_lt1000,
    latency_gte1000 = analytics_page_daily.latency_gte1000 + excluded.latency_gte1000;

-- name: ListAnalyticsPageDaily :many
SELECT day, resource_key, path, route_type, entry_id, revision_id, content_type_id, taxonomy_id, term_id,
       views, human_views, crawler_views, direct_views, internal_views, organic_search_views, organic_social_views, ai_referral_views, referral_views, campaign_views,
       cache_hits, cache_misses, response_count, response_bytes, response_duration_sum,
       latency_lt5, latency_lt20, latency_lt50, latency_lt100, latency_lt250, latency_lt1000, latency_gte1000
FROM analytics_page_daily
WHERE day >= ? AND day <= ?
ORDER BY views DESC
LIMIT ? OFFSET ?;

-- name: ListAnalyticsPageDailyByEntry :many
SELECT day, resource_key, path, route_type, entry_id, revision_id, content_type_id, taxonomy_id, term_id,
       views, human_views, crawler_views, direct_views, internal_views, organic_search_views, organic_social_views, ai_referral_views, referral_views, campaign_views,
       cache_hits, cache_misses, response_count, response_bytes, response_duration_sum,
       latency_lt5, latency_lt20, latency_lt50, latency_lt100, latency_lt250, latency_lt1000, latency_gte1000
FROM analytics_page_daily
WHERE entry_id = ? AND day >= ? AND day <= ?
ORDER BY day ASC;

-- name: GetAnalyticsPageDaily :one
SELECT day, resource_key, path, route_type, entry_id, revision_id, content_type_id, taxonomy_id, term_id,
       views, human_views, crawler_views, direct_views, internal_views, organic_search_views, organic_social_views, ai_referral_views, referral_views, campaign_views,
       cache_hits, cache_misses, response_count, response_bytes, response_duration_sum,
       latency_lt5, latency_lt20, latency_lt50, latency_lt100, latency_lt250, latency_lt1000, latency_gte1000
FROM analytics_page_daily
WHERE day = ? AND resource_key = ?
LIMIT 1;

-- name: SumAnalyticsPageDaily :one
SELECT
    CAST(COALESCE(SUM(views),0) AS INTEGER) AS total_views,
    CAST(COALESCE(SUM(human_views),0) AS INTEGER) AS total_human,
    CAST(COALESCE(SUM(crawler_views),0) AS INTEGER) AS total_crawler,
    CAST(COALESCE(SUM(direct_views),0) AS INTEGER) AS total_direct,
    CAST(COALESCE(SUM(internal_views),0) AS INTEGER) AS total_internal,
    CAST(COALESCE(SUM(organic_search_views),0) AS INTEGER) AS total_search,
    CAST(COALESCE(SUM(organic_social_views),0) AS INTEGER) AS total_social,
    CAST(COALESCE(SUM(ai_referral_views),0) AS INTEGER) AS total_ai,
    CAST(COALESCE(SUM(referral_views),0) AS INTEGER) AS total_referral,
    CAST(COALESCE(SUM(campaign_views),0) AS INTEGER) AS total_campaign
FROM analytics_page_daily
WHERE day >= ? AND day <= ?;

-- name: DeleteAnalyticsPageDailyBefore :exec
DELETE FROM analytics_page_daily WHERE day < ?;

-- name: DeleteAllAnalyticsPageDaily :exec
DELETE FROM analytics_page_daily;

-- Dimension upsert
-- name: UpsertAnalyticsDimensionDaily :exec
INSERT INTO analytics_dimension_daily (day, dimension, value, count)
VALUES (?, ?, ?, ?)
ON CONFLICT(day, dimension, value) DO UPDATE SET count = analytics_dimension_daily.count + excluded.count;

-- name: ListAnalyticsDimensionDaily :many
SELECT day, dimension, value, count
FROM analytics_dimension_daily
WHERE day >= ? AND day <= ? AND dimension = ?
ORDER BY count DESC
LIMIT ?;

-- name: GetAnalyticsDimensionCount :one
SELECT COUNT(*) FROM analytics_dimension_daily WHERE day = ? AND dimension = ?;

-- name: ListAnalyticsDimensionValues :many
SELECT value FROM analytics_dimension_daily WHERE day = ? AND dimension = ?;

-- name: DeleteAnalyticsDimensionDailyBefore :exec
DELETE FROM analytics_dimension_daily WHERE day < ?;

-- name: DeleteAllAnalyticsDimensionDaily :exec
DELETE FROM analytics_dimension_daily;

-- Transition upsert
-- name: UpsertAnalyticsTransitionDaily :exec
INSERT INTO analytics_transition_daily (day, from_resource_key, to_resource_key, from_path, to_path, count)
VALUES (?, ?, ?, ?, ?, ?)
ON CONFLICT(day, from_resource_key, to_resource_key) DO UPDATE SET count = analytics_transition_daily.count + excluded.count, from_path = excluded.from_path, to_path = excluded.to_path;

-- name: ListAnalyticsTransitions :many
SELECT day, from_resource_key, to_resource_key, from_path, to_path, count
FROM analytics_transition_daily
WHERE day >= ? AND day <= ?
ORDER BY count DESC
LIMIT ?;

-- name: DeleteAnalyticsTransitionDailyBefore :exec
DELETE FROM analytics_transition_daily WHERE day < ?;

-- name: DeleteAllAnalyticsTransitionDaily :exec
DELETE FROM analytics_transition_daily;

-- For cardinality protection: count distinct values per dimension per day already via GetAnalyticsDimensionCount, but we also need to check existence of value
-- name: ExistsAnalyticsDimensionValue :one
SELECT EXISTS(SELECT 1 FROM analytics_dimension_daily WHERE day = ? AND dimension = ? AND value = ?);

-- Aggregates for overview: site hourly sum
-- name: SumAnalyticsSiteHourly :one
SELECT
    CAST(COALESCE(SUM(requests),0) AS INTEGER) AS total_requests,
    CAST(COALESCE(SUM(views),0) AS INTEGER) AS total_views,
    CAST(COALESCE(SUM(human_views),0) AS INTEGER) AS total_human,
    CAST(COALESCE(SUM(crawler_views),0) AS INTEGER) AS total_crawler,
    CAST(COALESCE(SUM(speculative_requests),0) AS INTEGER) AS total_speculative,
    CAST(COALESCE(SUM(cache_hits),0) AS INTEGER) AS total_hits,
    CAST(COALESCE(SUM(cache_misses),0) AS INTEGER) AS total_misses,
    CAST(COALESCE(SUM(status_2xx),0) AS INTEGER) AS total_2xx,
    CAST(COALESCE(SUM(status_3xx),0) AS INTEGER) AS total_3xx,
    CAST(COALESCE(SUM(status_4xx),0) AS INTEGER) AS total_4xx,
    CAST(COALESCE(SUM(status_5xx),0) AS INTEGER) AS total_5xx,
    CAST(COALESCE(SUM(response_bytes),0) AS INTEGER) AS total_bytes,
    CAST(COALESCE(SUM(response_count),0) AS INTEGER) AS total_count,
    CAST(COALESCE(SUM(response_duration_sum),0) AS INTEGER) AS total_duration,
    CAST(COALESCE(SUM(latency_lt5),0) AS INTEGER) AS l_lt5,
    CAST(COALESCE(SUM(latency_lt20),0) AS INTEGER) AS l_lt20,
    CAST(COALESCE(SUM(latency_lt50),0) AS INTEGER) AS l_lt50,
    CAST(COALESCE(SUM(latency_lt100),0) AS INTEGER) AS l_lt100,
    CAST(COALESCE(SUM(latency_lt250),0) AS INTEGER) AS l_lt250,
    CAST(COALESCE(SUM(latency_lt1000),0) AS INTEGER) AS l_lt1000,
    CAST(COALESCE(SUM(latency_gte1000),0) AS INTEGER) AS l_gte1000
FROM analytics_site_hourly
WHERE hour >= ? AND hour <= ?;
