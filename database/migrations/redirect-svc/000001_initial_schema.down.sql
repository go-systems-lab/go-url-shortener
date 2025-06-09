-- Rollback Redirect Service - Initial Schema Migration

-- Drop indexes for redirect logs
DROP INDEX IF EXISTS idx_redirect_logs_short_code;
DROP INDEX IF EXISTS idx_redirect_logs_timestamp;
DROP INDEX IF EXISTS idx_redirect_logs_status;

-- Drop indexes for URL cache
DROP INDEX IF EXISTS idx_url_cache_short_code;
DROP INDEX IF EXISTS idx_url_cache_active;
DROP INDEX IF EXISTS idx_url_cache_expires_at;
DROP INDEX IF EXISTS idx_url_cache_last_accessed;

-- Drop tables
DROP TABLE IF EXISTS redirect_logs CASCADE;
DROP TABLE IF EXISTS url_cache CASCADE; 