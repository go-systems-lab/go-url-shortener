package database

import (
	"database/sql"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

type PostgreSQLTestSuite struct {
	suite.Suite
	db *PostgreSQL
}

func (suite *PostgreSQLTestSuite) SetupSuite() {
	// Set test database URL
	os.Setenv("DATABASE_URL", "postgres://postgres:password@localhost:5432/url_shortener?sslmode=disable")

	// Initialize database connection
	suite.db = NewPostgreSQL()

	// Drop existing tables to ensure fresh start
	suite.db.Pool.Exec(suite.db.ctx, "DROP TABLE IF EXISTS click_events CASCADE")
	suite.db.Pool.Exec(suite.db.ctx, "DROP TABLE IF EXISTS url_mappings CASCADE")

	// Run migrations
	err := suite.db.AutoMigrate()
	assert.NoError(suite.T(), err)
}

func (suite *PostgreSQLTestSuite) TearDownSuite() {
	// Clean up
	if suite.db != nil {
		suite.db.Close()
	}
}

func (suite *PostgreSQLTestSuite) SetupTest() {
	// Clean up data before each test
	suite.db.Pool.Exec(suite.db.ctx, "TRUNCATE TABLE url_mappings, click_events CASCADE")
}

func (suite *PostgreSQLTestSuite) TestDatabaseConnection() {
	// Test basic connectivity
	err := suite.db.HealthCheck()
	assert.NoError(suite.T(), err)
}

func (suite *PostgreSQLTestSuite) TestURLMappingCRUD() {
	// Test Create
	urlMapping := &URLMapping{
		ShortCode: "abc123",
		LongURL:   "https://example.com/very/long/url",
		UserID:    "user_123",
		IsActive:  true,
		Metadata:  `{"source": "api", "campaign": "test"}`,
		ExpiresAt: sql.NullTime{Valid: false}, // No expiration
	}

	err := suite.db.CreateURL(urlMapping)
	assert.NoError(suite.T(), err)
	assert.NotZero(suite.T(), urlMapping.ID)
	assert.False(suite.T(), urlMapping.CreatedAt.IsZero())

	// Test Read
	retrieved, err := suite.db.GetURLByShortCode("abc123")
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), "abc123", retrieved.ShortCode)
	assert.Equal(suite.T(), "https://example.com/very/long/url", retrieved.LongURL)
	assert.Equal(suite.T(), "user_123", retrieved.UserID)

	// Test Update (click count)
	err = suite.db.UpdateClickCount("abc123")
	assert.NoError(suite.T(), err)

	updated, err := suite.db.GetURLByShortCode("abc123")
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), int64(1), updated.ClickCount)

	// Test Delete (soft delete)
	err = suite.db.DeleteURL("abc123", "user_123")
	assert.NoError(suite.T(), err)

	deleted, err := suite.db.GetURLByShortCode("abc123")
	assert.Error(suite.T(), err) // Should not find deleted record
	assert.Nil(suite.T(), deleted)
}

func (suite *PostgreSQLTestSuite) TestClickEventCRUD() {
	// Create a URL mapping first
	urlMapping := &URLMapping{
		ShortCode: "test456",
		LongURL:   "https://test.com",
		UserID:    "user_456",
		IsActive:  true,
		Metadata:  `{}`, // Valid empty JSON
		ExpiresAt: sql.NullTime{Valid: false},
	}
	err := suite.db.CreateURL(urlMapping)
	assert.NoError(suite.T(), err)

	// Test Create Click Event
	clickEvent := &ClickEvent{
		ShortCode:   "test456",
		Timestamp:   time.Now(),
		IPAddress:   "192.168.1.1",
		UserAgent:   "Mozilla/5.0",
		Referrer:    "https://google.com",
		CountryCode: "US",
		CountryName: "United States",
		City:        "New York",
		DeviceType:  "desktop",
		Browser:     "Chrome",
		OS:          "Windows",
		IsUnique:    true,
		SessionID:   "session_123",
	}

	err = suite.db.CreateClickEvent(clickEvent)
	assert.NoError(suite.T(), err)
	assert.NotZero(suite.T(), clickEvent.ID)

	// Test Read
	events, err := suite.db.GetClickEventsByShortCode("test456", 10)
	assert.NoError(suite.T(), err)
	assert.Len(suite.T(), events, 1)
	assert.Equal(suite.T(), "test456", events[0].ShortCode)
	assert.Equal(suite.T(), "US", events[0].CountryCode)
	assert.Equal(suite.T(), "desktop", events[0].DeviceType)
}

func (suite *PostgreSQLTestSuite) TestGetURLsByUserID() {
	// Create multiple URLs for a user
	urls := []*URLMapping{
		{
			ShortCode: "user1_url1",
			LongURL:   "https://example1.com",
			UserID:    "test_user",
			Metadata:  `{}`,
			ExpiresAt: sql.NullTime{Valid: false},
		},
		{
			ShortCode: "user1_url2",
			LongURL:   "https://example2.com",
			UserID:    "test_user",
			Metadata:  `{}`,
			ExpiresAt: sql.NullTime{Valid: false},
		},
		{
			ShortCode: "user2_url1",
			LongURL:   "https://example3.com",
			UserID:    "other_user",
			Metadata:  `{}`,
			ExpiresAt: sql.NullTime{Valid: false},
		},
	}

	for _, url := range urls {
		err := suite.db.CreateURL(url)
		assert.NoError(suite.T(), err)
	}

	// Test pagination
	userURLs, err := suite.db.GetURLsByUserID("test_user", 10, 0)
	assert.NoError(suite.T(), err)
	assert.Len(suite.T(), userURLs, 2)

	// Verify only user's URLs are returned
	for _, url := range userURLs {
		assert.Equal(suite.T(), "test_user", url.UserID)
	}
}

func (suite *PostgreSQLTestSuite) TestPerformanceIndexes() {
	// Test that our performance indexes are created
	var indexCount int64

	// Check URL mappings indexes
	err := suite.db.Pool.QueryRow(suite.db.ctx, `
		SELECT COUNT(*) 
		FROM pg_indexes 
		WHERE tablename = 'url_mappings' 
		AND indexname LIKE 'idx_%'
	`).Scan(&indexCount)

	assert.NoError(suite.T(), err)
	assert.Greater(suite.T(), indexCount, int64(0), "Performance indexes should be created")

	// Check click events indexes
	err = suite.db.Pool.QueryRow(suite.db.ctx, `
		SELECT COUNT(*) 
		FROM pg_indexes 
		WHERE tablename = 'click_events' 
		AND indexname LIKE 'idx_%'
	`).Scan(&indexCount)

	assert.NoError(suite.T(), err)
	assert.Greater(suite.T(), indexCount, int64(0), "Click events indexes should be created")
}

func (suite *PostgreSQLTestSuite) TestDatabaseStats() {
	// Test database statistics
	stats := suite.db.GetStats()
	assert.NotNil(suite.T(), stats)
	assert.Contains(suite.T(), stats, "total_conns")
	assert.Contains(suite.T(), stats, "acquired_conns")
	assert.Contains(suite.T(), stats, "idle_conns")
}

func (suite *PostgreSQLTestSuite) TestUniqueConstraints() {
	// Test unique constraint on short_code
	urlMapping1 := &URLMapping{
		ShortCode: "unique123",
		LongURL:   "https://example1.com",
		UserID:    "user_1",
		IsActive:  true,
		Metadata:  `{}`, // Valid empty JSON
		ExpiresAt: sql.NullTime{Valid: false},
	}

	err := suite.db.CreateURL(urlMapping1)
	assert.NoError(suite.T(), err)

	// Try to create another with same short_code
	urlMapping2 := &URLMapping{
		ShortCode: "unique123",
		LongURL:   "https://example2.com",
		UserID:    "user_2",
		IsActive:  true,
		Metadata:  `{}`, // Valid empty JSON
		ExpiresAt: sql.NullTime{Valid: false},
	}

	err = suite.db.CreateURL(urlMapping2)
	assert.Error(suite.T(), err) // Should fail due to unique constraint
}

func TestPostgreSQLTestSuite(t *testing.T) {
	suite.Run(t, new(PostgreSQLTestSuite))
}
