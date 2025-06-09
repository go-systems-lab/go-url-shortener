-- URL Shortener Service - Initial Schema Migration
-- This migration creates the core tables for URL mapping functionality

-- Enable required extensions
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE EXTENSION IF NOT EXISTS "pg_stat_statements";

-- URL mappings table (primary data store)
CREATE TABLE url_mappings (
    id BIGSERIAL PRIMARY KEY,
    short_code VARCHAR(10) UNIQUE NOT NULL,
    long_url TEXT NOT NULL,
    user_id VARCHAR(50) NOT NULL,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    expires_at TIMESTAMPTZ,
    click_count BIGINT DEFAULT 0,
    last_accessed TIMESTAMPTZ,
    is_active BOOLEAN DEFAULT true,
    metadata JSONB DEFAULT '{}'::jsonb
);

-- Performance indexes (without CONCURRENTLY for migration compatibility)
CREATE INDEX idx_url_mappings_short_code ON url_mappings(short_code);
CREATE INDEX idx_url_mappings_user_id ON url_mappings(user_id);
CREATE INDEX idx_url_mappings_created_at ON url_mappings(created_at);
CREATE INDEX idx_url_mappings_active ON url_mappings(is_active) WHERE is_active = true;
CREATE INDEX idx_url_mappings_expires_at ON url_mappings(expires_at) WHERE expires_at IS NOT NULL;

-- Updated timestamp trigger function
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ language 'plpgsql';

-- Apply trigger to url_mappings
CREATE TRIGGER update_url_mappings_updated_at
    BEFORE UPDATE ON url_mappings
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column(); 