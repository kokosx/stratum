-- Analytics Core: aggregate-first, privacy-safe analytics.

-- Extend site_settings with analytics configuration (typed columns, not generic options).
ALTER TABLE site_settings ADD COLUMN analytics_enabled INTEGER NOT NULL DEFAULT 1
    CHECK (analytics_enabled IN (0,1));
ALTER TABLE site_settings ADD COLUMN analytics_retention_days INTEGER NOT NULL DEFAULT 730
    CHECK (analytics_retention_days IN (90,180,365,730,1095));
ALTER TABLE site_settings ADD COLUMN analytics_hourly_retention_days INTEGER NOT NULL DEFAULT 90
    CHECK (analytics_hourly_retention_days IN (30,90,180));

-- Site-wide hourly counters (operational + time series).
CREATE TABLE analytics_site_hourly (
    hour INTEGER NOT NULL PRIMARY KEY,
    requests INTEGER NOT NULL DEFAULT 0,
    views INTEGER NOT NULL DEFAULT 0,
    human_views INTEGER NOT NULL DEFAULT 0,
    crawler_views INTEGER NOT NULL DEFAULT 0,
    speculative_requests INTEGER NOT NULL DEFAULT 0,
    cache_hits INTEGER NOT NULL DEFAULT 0,
    cache_misses INTEGER NOT NULL DEFAULT 0,
    status_2xx INTEGER NOT NULL DEFAULT 0,
    status_3xx INTEGER NOT NULL DEFAULT 0,
    status_4xx INTEGER NOT NULL DEFAULT 0,
    status_5xx INTEGER NOT NULL DEFAULT 0,
    response_bytes INTEGER NOT NULL DEFAULT 0,
    response_count INTEGER NOT NULL DEFAULT 0,
    response_duration_sum INTEGER NOT NULL DEFAULT 0,
    latency_lt5 INTEGER NOT NULL DEFAULT 0,
    latency_lt20 INTEGER NOT NULL DEFAULT 0,
    latency_lt50 INTEGER NOT NULL DEFAULT 0,
    latency_lt100 INTEGER NOT NULL DEFAULT 0,
    latency_lt250 INTEGER NOT NULL DEFAULT 0,
    latency_lt1000 INTEGER NOT NULL DEFAULT 0,
    latency_gte1000 INTEGER NOT NULL DEFAULT 0
) STRICT;

-- Per-resource daily counters (content/revision aware).
CREATE TABLE analytics_page_daily (
    day TEXT NOT NULL,
    resource_key TEXT NOT NULL,
    path TEXT NOT NULL,
    route_type TEXT NOT NULL CHECK (route_type IN ('entry','archive','taxonomy','system')),
    entry_id TEXT,
    revision_id TEXT,
    content_type_id TEXT,
    taxonomy_id TEXT,
    term_id TEXT,
    views INTEGER NOT NULL DEFAULT 0,
    human_views INTEGER NOT NULL DEFAULT 0,
    crawler_views INTEGER NOT NULL DEFAULT 0,
    direct_views INTEGER NOT NULL DEFAULT 0,
    internal_views INTEGER NOT NULL DEFAULT 0,
    organic_search_views INTEGER NOT NULL DEFAULT 0,
    organic_social_views INTEGER NOT NULL DEFAULT 0,
    ai_referral_views INTEGER NOT NULL DEFAULT 0,
    referral_views INTEGER NOT NULL DEFAULT 0,
    campaign_views INTEGER NOT NULL DEFAULT 0,
    cache_hits INTEGER NOT NULL DEFAULT 0,
    cache_misses INTEGER NOT NULL DEFAULT 0,
    response_count INTEGER NOT NULL DEFAULT 0,
    response_bytes INTEGER NOT NULL DEFAULT 0,
    response_duration_sum INTEGER NOT NULL DEFAULT 0,
    latency_lt5 INTEGER NOT NULL DEFAULT 0,
    latency_lt20 INTEGER NOT NULL DEFAULT 0,
    latency_lt50 INTEGER NOT NULL DEFAULT 0,
    latency_lt100 INTEGER NOT NULL DEFAULT 0,
    latency_lt250 INTEGER NOT NULL DEFAULT 0,
    latency_lt1000 INTEGER NOT NULL DEFAULT 0,
    latency_gte1000 INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (day, resource_key)
) STRICT;

CREATE INDEX idx_analytics_page_daily_day ON analytics_page_daily(day);
CREATE INDEX idx_analytics_page_daily_entry ON analytics_page_daily(entry_id);
CREATE INDEX idx_analytics_page_daily_ct ON analytics_page_daily(content_type_id);

-- Controlled dimensional counters.
CREATE TABLE analytics_dimension_daily (
    day TEXT NOT NULL,
    dimension TEXT NOT NULL CHECK (dimension IN ('referrer_host','utm_source','utm_medium','utm_campaign','browser','os','device','language','crawler')),
    value TEXT NOT NULL,
    count INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (day, dimension, value)
) STRICT;

CREATE INDEX idx_analytics_dimension_daily_day_dim ON analytics_dimension_daily(day, dimension);

-- Aggregate transitions (no session).
CREATE TABLE analytics_transition_daily (
    day TEXT NOT NULL,
    from_resource_key TEXT NOT NULL,
    to_resource_key TEXT NOT NULL,
    from_path TEXT NOT NULL,
    to_path TEXT NOT NULL,
    count INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (day, from_resource_key, to_resource_key)
) STRICT;

CREATE INDEX idx_analytics_transition_daily_day ON analytics_transition_daily(day);
