-- ClickHouse Analytics Service - Initial Schema
-- This creates the analytics database and tables for high-performance time-series data

-- Create analytics database
CREATE DATABASE IF NOT EXISTS analytics;

-- Switch to analytics database
USE analytics;

-- Click analytics table optimized for time-series queries
CREATE TABLE IF NOT EXISTS click_analytics (
    short_code String,
    long_url String,
    client_ip String,  -- Using String to handle both IPv4 and IPv6
    user_agent String,
    referrer String,
    country String,
    city String,
    device_type String,
    browser String,
    os String,
    timestamp DateTime64(3) DEFAULT now(),
    session_id String,
    is_unique UInt8 DEFAULT 0,
    created_at DateTime64(3) DEFAULT now()
) ENGINE = MergeTree()
PARTITION BY toYYYYMM(timestamp)
ORDER BY (short_code, timestamp)
SETTINGS index_granularity = 8192;

-- URL summary table for fast aggregations
CREATE TABLE IF NOT EXISTS url_summary (
    short_code String,
    date Date,
    hour UInt8,
    total_clicks UInt64,
    unique_clicks UInt64,
    top_country String,
    top_device String,
    top_browser String,
    created_at DateTime64(3) DEFAULT now(),
    updated_at DateTime64(3) DEFAULT now()
) ENGINE = ReplacingMergeTree(updated_at)
PARTITION BY toYYYYMM(date)
ORDER BY (short_code, date, hour)
SETTINGS index_granularity = 8192;

-- Daily aggregation materialized view for dashboard queries
CREATE MATERIALIZED VIEW IF NOT EXISTS daily_analytics
TO url_summary
AS SELECT
    short_code,
    toDate(timestamp) as date,
    toHour(timestamp) as hour,
    count() as total_clicks,
    uniq(session_id) as unique_clicks,
    topK(1)(country)[1] as top_country,
    topK(1)(device_type)[1] as top_device,
    topK(1)(browser)[1] as top_browser,
    now() as created_at,
    now() as updated_at
FROM click_analytics
GROUP BY short_code, date, hour;

-- Real-time dashboard view for fast queries
CREATE VIEW IF NOT EXISTS dashboard_metrics AS
SELECT 
    toInt64(uniq(short_code)) as total_urls,
    toInt64(count()) as total_clicks,
    toInt64(uniq(session_id)) as unique_clicks,
    toInt64(uniqExact(short_code)) as active_urls
FROM click_analytics 
WHERE timestamp >= now() - INTERVAL 30 DAY;

-- Top URLs view for analytics
CREATE VIEW IF NOT EXISTS top_urls AS
SELECT 
    short_code,
    count() as total_clicks,
    uniq(session_id) as unique_clicks,
    max(timestamp) as last_click
FROM click_analytics 
WHERE timestamp >= now() - INTERVAL 7 DAY
GROUP BY short_code
ORDER BY total_clicks DESC;

-- Country analytics view
CREATE VIEW IF NOT EXISTS country_analytics AS
SELECT 
    short_code,
    country,
    count() as clicks,
    uniq(session_id) as unique_visitors
FROM click_analytics 
WHERE timestamp >= now() - INTERVAL 7 DAY
  AND country != ''
GROUP BY short_code, country
ORDER BY short_code, clicks DESC; 