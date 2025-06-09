-- Analytics Service - Rollback Initial Schema
-- This drops all analytics-related tables and triggers

-- Drop triggers first
DROP TRIGGER IF EXISTS update_analytics_summary_updated_at ON analytics_summary;
DROP TRIGGER IF EXISTS update_url_performance_updated_at ON url_performance;
DROP TRIGGER IF EXISTS update_analytics_config_updated_at ON analytics_config;

-- Drop tables
DROP TABLE IF EXISTS analytics_config;
DROP TABLE IF EXISTS url_performance;
DROP TABLE IF EXISTS analytics_summary; 