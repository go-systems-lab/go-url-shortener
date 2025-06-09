-- Rollback URL Shortener Service - Initial Schema Migration

-- Drop trigger
DROP TRIGGER IF EXISTS update_url_mappings_updated_at ON url_mappings;

-- Drop function
DROP FUNCTION IF EXISTS update_updated_at_column();

-- Drop indexes
DROP INDEX IF EXISTS idx_url_mappings_short_code;
DROP INDEX IF EXISTS idx_url_mappings_user_id;
DROP INDEX IF EXISTS idx_url_mappings_created_at;
DROP INDEX IF EXISTS idx_url_mappings_active;
DROP INDEX IF EXISTS idx_url_mappings_expires_at;

-- Drop tables
DROP TABLE IF EXISTS url_mappings CASCADE; 