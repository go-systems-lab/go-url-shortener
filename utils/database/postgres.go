package database

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/jmoiron/sqlx"
)

// PostgreSQL connection manager - Production configuration with pgx
type PostgreSQL struct {
	Pool *pgxpool.Pool
	DB   *sqlx.DB
	ctx  context.Context
}

// URLMapping represents the URL table structure from HLD
type URLMapping struct {
	ID           int64        `db:"id" json:"id"`
	ShortCode    string       `db:"short_code" json:"short_code"`
	LongURL      string       `db:"long_url" json:"long_url"`
	UserID       string       `db:"user_id" json:"user_id"`
	CreatedAt    time.Time    `db:"created_at" json:"created_at"`
	ExpiresAt    sql.NullTime `db:"expires_at" json:"expires_at"`
	ClickCount   int64        `db:"click_count" json:"click_count"`
	LastAccessed sql.NullTime `db:"last_accessed" json:"last_accessed"`
	IsActive     bool         `db:"is_active" json:"is_active"`
	Metadata     string       `db:"metadata" json:"metadata"` // PostgreSQL JSONB
}

// ClickEvent represents the analytics table structure
type ClickEvent struct {
	ID          int64     `db:"id" json:"id"`
	ShortCode   string    `db:"short_code" json:"short_code"`
	Timestamp   time.Time `db:"timestamp" json:"timestamp"`
	IPAddress   string    `db:"ip_address" json:"ip_address"`
	UserAgent   string    `db:"user_agent" json:"user_agent"`
	Referrer    string    `db:"referrer" json:"referrer"`
	CountryCode string    `db:"country_code" json:"country_code"`
	CountryName string    `db:"country_name" json:"country_name"`
	City        string    `db:"city" json:"city"`
	DeviceType  string    `db:"device_type" json:"device_type"`
	Browser     string    `db:"browser" json:"browser"`
	OS          string    `db:"os" json:"os"`
	IsUnique    bool      `db:"is_unique" json:"is_unique"`
	SessionID   string    `db:"session_id" json:"session_id"`
}

// NewPostgreSQL creates a new PostgreSQL connection with production settings using pgx
func NewPostgreSQL() *PostgreSQL {
	ctx := context.Background()

	// Get database URL from environment
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		// Default development configuration
		dbURL = "postgres://postgres:password@localhost:5432/url_shortener?sslmode=disable"
	}

	// Parse connection config for pgxpool
	config, err := pgxpool.ParseConfig(dbURL)
	if err != nil {
		log.Fatalf("Failed to parse database URL: %v", err)
	}

	// Production-optimized connection pool settings
	config.MaxConns = 25                       // Maximum connections (from HLD specs)
	config.MinConns = 5                        // Minimum idle connections
	config.MaxConnLifetime = 5 * time.Minute   // Connection lifetime
	config.MaxConnIdleTime = 30 * time.Second  // Idle timeout
	config.HealthCheckPeriod = 1 * time.Minute // Health check interval

	// Create connection pool
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		log.Fatalf("Failed to create connection pool: %v", err)
	}

	// Test the connection
	if err := pool.Ping(ctx); err != nil {
		log.Fatalf("Failed to ping PostgreSQL: %v", err)
	}

	// Create sqlx wrapper using pgx stdlib driver
	sqlxDB := sqlx.NewDb(stdlib.OpenDBFromPool(pool), "pgx")

	log.Println("✅ Successfully connected to PostgreSQL with pgx")

	return &PostgreSQL{
		Pool: pool,
		DB:   sqlxDB,
		ctx:  ctx,
	}
}

// CreateTables creates the database tables with proper indexes
func (p *PostgreSQL) CreateTables() error {
	log.Println("🔧 Creating database tables...")

	// Create URL mappings table
	urlMappingsSQL := `
	CREATE TABLE IF NOT EXISTS url_mappings (
		id BIGSERIAL PRIMARY KEY,
		short_code VARCHAR(10) UNIQUE NOT NULL,
		long_url TEXT NOT NULL,
		user_id VARCHAR(50),
		created_at TIMESTAMPTZ DEFAULT NOW(),
		expires_at TIMESTAMPTZ,
		click_count BIGINT DEFAULT 0,
		last_accessed TIMESTAMPTZ,
		is_active BOOLEAN DEFAULT true,
		metadata JSONB DEFAULT '{}'::jsonb
	);`

	if _, err := p.Pool.Exec(p.ctx, urlMappingsSQL); err != nil {
		return fmt.Errorf("failed to create url_mappings table: %v", err)
	}

	// Create click events table
	clickEventsSQL := `
	CREATE TABLE IF NOT EXISTS click_events (
		id BIGSERIAL PRIMARY KEY,
		short_code VARCHAR(10) NOT NULL,
		timestamp TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		ip_address VARCHAR(45),
		user_agent TEXT,
		referrer TEXT,
		country_code VARCHAR(2),
		country_name VARCHAR(100),
		city VARCHAR(100),
		device_type VARCHAR(20),
		browser VARCHAR(50),
		os VARCHAR(50),
		is_unique BOOLEAN DEFAULT false,
		session_id VARCHAR(100)
	);`

	if _, err := p.Pool.Exec(p.ctx, clickEventsSQL); err != nil {
		return fmt.Errorf("failed to create click_events table: %v", err)
	}

	log.Println("✅ Database tables created successfully")
	return nil
}

// CreateIndexes creates performance indexes as specified in HLD
func (p *PostgreSQL) CreateIndexes() error {
	log.Println("🔧 Creating performance indexes...")

	indexes := []string{
		// URL mappings indexes
		"CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_url_mappings_short_code ON url_mappings(short_code);",
		"CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_url_mappings_user_id ON url_mappings(user_id);",
		"CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_url_mappings_created_at ON url_mappings(created_at DESC);",
		"CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_url_mappings_expires_at ON url_mappings(expires_at) WHERE expires_at IS NOT NULL;",
		"CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_url_mappings_user_created ON url_mappings(user_id, created_at DESC);",
		"CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_url_mappings_active ON url_mappings(is_active) WHERE is_active = true;",

		// Click events indexes for analytics
		"CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_click_events_short_code ON click_events(short_code);",
		"CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_click_events_timestamp ON click_events(timestamp DESC);",
		"CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_click_events_country_code ON click_events(country_code);",
		"CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_click_events_device_type ON click_events(device_type);",
		"CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_click_events_shortcode_time ON click_events(short_code, timestamp DESC);",
		"CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_click_events_country_time ON click_events(country_code, timestamp DESC);",
		"CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_click_events_device_time ON click_events(device_type, timestamp DESC);",
	}

	for _, indexSQL := range indexes {
		if _, err := p.Pool.Exec(p.ctx, indexSQL); err != nil {
			log.Printf("Warning: Failed to create index: %v", err)
			// Continue with other indexes even if one fails
		}
	}

	log.Println("✅ Performance indexes created successfully")
	return nil
}

// AutoMigrate creates tables and indexes
func (p *PostgreSQL) AutoMigrate() error {
	if err := p.CreateTables(); err != nil {
		return err
	}
	return p.CreateIndexes()
}

// CreateURL inserts a new URL mapping
func (p *PostgreSQL) CreateURL(url *URLMapping) error {
	query := `
		INSERT INTO url_mappings (short_code, long_url, user_id, expires_at, metadata)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, created_at`

	var expiresAt interface{}
	if url.ExpiresAt.Valid {
		expiresAt = url.ExpiresAt.Time
	} else {
		expiresAt = nil
	}

	return p.Pool.QueryRow(p.ctx, query,
		url.ShortCode, url.LongURL, url.UserID, expiresAt, url.Metadata,
	).Scan(&url.ID, &url.CreatedAt)
}

// GetURLByShortCode retrieves a URL mapping by short code
func (p *PostgreSQL) GetURLByShortCode(shortCode string) (*URLMapping, error) {
	var url URLMapping
	query := `
		SELECT id, short_code, long_url, user_id, created_at, expires_at,
		       click_count, last_accessed, is_active, metadata
		FROM url_mappings 
		WHERE short_code = $1 AND is_active = true`

	err := p.DB.Get(&url, query, shortCode)
	if err != nil {
		return nil, err
	}
	return &url, nil
}

// GetURLsByUserID retrieves all URLs for a user with pagination
func (p *PostgreSQL) GetURLsByUserID(userID string, limit, offset int) ([]URLMapping, error) {
	var urls []URLMapping
	query := `
		SELECT id, short_code, long_url, user_id, created_at, expires_at,
		       click_count, last_accessed, is_active, metadata
		FROM url_mappings 
		WHERE user_id = $1 AND is_active = true
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3`

	err := p.DB.Select(&urls, query, userID, limit, offset)
	return urls, err
}

// UpdateClickCount increments the click count for a URL
func (p *PostgreSQL) UpdateClickCount(shortCode string) error {
	query := `
		UPDATE url_mappings 
		SET click_count = click_count + 1, last_accessed = NOW()
		WHERE short_code = $1`

	_, err := p.Pool.Exec(p.ctx, query, shortCode)
	return err
}

// DeleteURL soft deletes a URL (sets is_active to false)
func (p *PostgreSQL) DeleteURL(shortCode, userID string) error {
	query := `
		UPDATE url_mappings 
		SET is_active = false 
		WHERE short_code = $1 AND user_id = $2`

	result, err := p.Pool.Exec(p.ctx, query, shortCode, userID)
	if err != nil {
		return err
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("URL not found or permission denied")
	}
	return nil
}

// CreateClickEvent records a click event for analytics
func (p *PostgreSQL) CreateClickEvent(event *ClickEvent) error {
	query := `
		INSERT INTO click_events (
			short_code, timestamp, ip_address, user_agent, referrer,
			country_code, country_name, city, device_type, browser,
			os, is_unique, session_id
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		RETURNING id`

	return p.Pool.QueryRow(p.ctx, query,
		event.ShortCode, event.Timestamp, event.IPAddress, event.UserAgent, event.Referrer,
		event.CountryCode, event.CountryName, event.City, event.DeviceType, event.Browser,
		event.OS, event.IsUnique, event.SessionID,
	).Scan(&event.ID)
}

// GetClickEventsByShortCode retrieves click events for analytics
func (p *PostgreSQL) GetClickEventsByShortCode(shortCode string, limit int) ([]ClickEvent, error) {
	var events []ClickEvent
	query := `
		SELECT id, short_code, timestamp, ip_address, user_agent, referrer,
		       country_code, country_name, city, device_type, browser,
		       os, is_unique, session_id
		FROM click_events 
		WHERE short_code = $1
		ORDER BY timestamp DESC
		LIMIT $2`

	err := p.DB.Select(&events, query, shortCode, limit)
	return events, err
}

// HealthCheck verifies database connectivity
func (p *PostgreSQL) HealthCheck() error {
	return p.Pool.Ping(p.ctx)
}

// GetStats returns database connection statistics
func (p *PostgreSQL) GetStats() map[string]interface{} {
	stats := p.Pool.Stat()
	return map[string]interface{}{
		"total_conns":        stats.TotalConns(),
		"acquired_conns":     stats.AcquiredConns(),
		"idle_conns":         stats.IdleConns(),
		"max_conns":          stats.MaxConns(),
		"constructing_conns": stats.ConstructingConns(),
	}
}

// Close closes the database connections
func (p *PostgreSQL) Close() {
	if p.Pool != nil {
		p.Pool.Close()
	}
	if p.DB != nil {
		p.DB.Close()
	}
}
