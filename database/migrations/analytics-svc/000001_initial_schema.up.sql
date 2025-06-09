-- Analytics Service - Initial Schema Migration
-- This creates PostgreSQL tables for analytics summaries and reporting
-- Note: Raw click events are stored in ClickHouse for high-volume analytics

-- Enable required extensions
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- Analytics summary table (aggregated data from ClickHouse)
CREATE TABLE analytics_summary (
    id BIGSERIAL PRIMARY KEY,
    short_code VARCHAR(10) NOT NULL,
    date_key DATE NOT NULL,
    hour_key SMALLINT NOT NULL CHECK (hour_key >= 0 AND hour_key <= 23),
    total_clicks BIGINT DEFAULT 0,
    unique_clicks BIGINT DEFAULT 0,
    bounce_rate DECIMAL(5,2) DEFAULT 0.00,
    avg_session_duration INTEGER DEFAULT 0, -- in seconds
    top_countries JSONB DEFAULT '[]'::jsonb,
    top_referrers JSONB DEFAULT '[]'::jsonb,
    device_breakdown JSONB DEFAULT '{}'::jsonb,
    browser_breakdown JSONB DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

-- Unique constraint for time-series data
CREATE UNIQUE INDEX idx_analytics_summary_unique 
ON analytics_summary(short_code, date_key, hour_key);

-- Performance indexes
CREATE INDEX idx_analytics_summary_short_code ON analytics_summary(short_code);
CREATE INDEX idx_analytics_summary_date ON analytics_summary(date_key DESC);
CREATE INDEX idx_analytics_summary_hour ON analytics_summary(hour_key);
CREATE INDEX idx_analytics_summary_clicks ON analytics_summary(total_clicks DESC);

-- URL performance metrics table
CREATE TABLE url_performance (
    id BIGSERIAL PRIMARY KEY,
    short_code VARCHAR(10) UNIQUE NOT NULL,
    total_clicks BIGINT DEFAULT 0,
    unique_visitors BIGINT DEFAULT 0,
    conversion_rate DECIMAL(5,2) DEFAULT 0.00,
    avg_click_rate_per_day DECIMAL(10,2) DEFAULT 0.00,
    peak_hour SMALLINT DEFAULT 0,
    peak_day_of_week SMALLINT DEFAULT 0,
    geographic_distribution JSONB DEFAULT '{}'::jsonb,
    last_updated TIMESTAMPTZ DEFAULT NOW()
);

-- Performance indexes for URL metrics
CREATE INDEX idx_url_performance_short_code ON url_performance(short_code);
CREATE INDEX idx_url_performance_total_clicks ON url_performance(total_clicks DESC);
CREATE INDEX idx_url_performance_conversion ON url_performance(conversion_rate DESC);

-- Analytics configuration table
CREATE TABLE analytics_config (
    id SERIAL PRIMARY KEY,
    key VARCHAR(100) UNIQUE NOT NULL,
    value TEXT NOT NULL,
    description TEXT,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

-- Insert default configuration
INSERT INTO analytics_config (key, value, description) VALUES
('clickhouse_sync_interval', '300', 'Sync interval with ClickHouse in seconds'),
('summary_retention_days', '365', 'Days to retain summary data'),
('batch_size', '1000', 'Batch size for data processing'),
('enable_real_time_analytics', 'true', 'Enable real-time analytics processing');

-- Updated timestamp trigger for analytics tables
CREATE TRIGGER update_analytics_summary_updated_at
    BEFORE UPDATE ON analytics_summary
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_url_performance_updated_at
    BEFORE UPDATE ON url_performance
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_analytics_config_updated_at
    BEFORE UPDATE ON analytics_config
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column(); 