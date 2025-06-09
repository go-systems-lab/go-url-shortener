-- Redirect Service - Initial Schema Migration
-- This migration creates tables for URL resolution caching and redirect tracking

-- Enable required extensions
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- URL cache table for fast redirect lookups
CREATE TABLE url_cache (
    id BIGSERIAL PRIMARY KEY,
    short_code VARCHAR(10) UNIQUE NOT NULL,
    long_url TEXT NOT NULL,
    is_active BOOLEAN DEFAULT true,
    expires_at TIMESTAMPTZ,
    cached_at TIMESTAMPTZ DEFAULT NOW(),
    last_accessed TIMESTAMPTZ DEFAULT NOW(),
    access_count BIGINT DEFAULT 0
);

-- Performance indexes for redirect service
CREATE INDEX idx_url_cache_short_code ON url_cache(short_code);
CREATE INDEX idx_url_cache_active ON url_cache(is_active) WHERE is_active = true;
CREATE INDEX idx_url_cache_expires_at ON url_cache(expires_at) WHERE expires_at IS NOT NULL;
CREATE INDEX idx_url_cache_last_accessed ON url_cache(last_accessed);

-- Redirect logs table for monitoring and debugging
CREATE TABLE redirect_logs (
    id BIGSERIAL PRIMARY KEY,
    short_code VARCHAR(10) NOT NULL,
    long_url TEXT NOT NULL,
    client_ip INET,
    user_agent TEXT,
    referrer TEXT,
    response_time_ms INTEGER,
    status_code INTEGER DEFAULT 301,
    timestamp TIMESTAMPTZ DEFAULT NOW()
);

-- Indexes for redirect logs
CREATE INDEX idx_redirect_logs_short_code ON redirect_logs(short_code);
CREATE INDEX idx_redirect_logs_timestamp ON redirect_logs(timestamp);
CREATE INDEX idx_redirect_logs_status ON redirect_logs(status_code);

-- Note: Monthly partitioning would typically be set up with pg_partman in production
-- Removed problematic partial index with NOW() function 