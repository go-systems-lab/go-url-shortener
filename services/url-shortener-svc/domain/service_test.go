package domain

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"github.com/go-systems-lab/go-url-shortener/utils/cache"
	"github.com/go-systems-lab/go-url-shortener/utils/database"
)

type URLServiceTestSuite struct {
	suite.Suite
	service *URLService
	db      *database.PostgreSQL
	cache   *cache.Redis
}

func (suite *URLServiceTestSuite) SetupSuite() {
	// Set test database URL
	os.Setenv("DATABASE_URL", "postgres://postgres:password@localhost:5432/url_shortener_db?sslmode=disable")
	os.Setenv("REDIS_URL", "redis://localhost:6379/1") // Use DB 1 for tests

	// Initialize database and cache
	suite.db = database.NewPostgreSQL()
	suite.cache = cache.NewRedis()

	// Drop and recreate tables for fresh start
	ctx := context.Background()
	suite.db.Pool.Exec(ctx, "DROP TABLE IF EXISTS click_events CASCADE")
	suite.db.Pool.Exec(ctx, "DROP TABLE IF EXISTS url_mappings CASCADE")

	// Run migrations
	err := suite.db.AutoMigrate()
	assert.NoError(suite.T(), err)

	// Create service
	suite.service = NewURLService(suite.db, suite.cache)
}

func (suite *URLServiceTestSuite) TearDownSuite() {
	if suite.cache != nil {
		suite.cache.FlushDB() // Clear test cache
		suite.cache.Close()
	}
	if suite.db != nil {
		suite.db.Close()
	}
}

func (suite *URLServiceTestSuite) SetupTest() {
	// Clean up data before each test
	ctx := context.Background()
	suite.db.Pool.Exec(ctx, "TRUNCATE TABLE url_mappings, click_events CASCADE")
	suite.cache.FlushDB() // Clear cache
}

func (suite *URLServiceTestSuite) TestShortenURL() {
	// Test basic URL shortening
	req := &CreateURLRequest{
		LongURL: "https://example.com/very/long/url/path",
		UserID:  "test_user_123",
		Metadata: map[string]string{
			"source":   "api",
			"campaign": "test",
		},
	}

	url, err := suite.service.ShortenURL(req)
	assert.NoError(suite.T(), err)
	assert.NotNil(suite.T(), url)
	assert.NotEmpty(suite.T(), url.ShortCode)
	assert.Equal(suite.T(), req.LongURL, url.LongURL)
	assert.Equal(suite.T(), req.UserID, url.UserID)
	assert.True(suite.T(), url.IsActive)
	assert.NotZero(suite.T(), url.ID)
	assert.False(suite.T(), url.CreatedAt.IsZero())
}

func (suite *URLServiceTestSuite) TestShortenURLWithCustomAlias() {
	// Test URL shortening with custom alias
	req := &CreateURLRequest{
		LongURL:     "https://example.com/custom",
		CustomAlias: "mycustom",
		UserID:      "test_user_123",
	}

	url, err := suite.service.ShortenURL(req)
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), "mycustom", url.ShortCode)
}

func (suite *URLServiceTestSuite) TestShortenURLWithExpiration() {
	// Test URL shortening with expiration
	expirationTime := time.Now().Add(24 * time.Hour)
	req := &CreateURLRequest{
		LongURL:        "https://example.com/expires",
		UserID:         "test_user_123",
		ExpirationTime: &expirationTime,
	}

	url, err := suite.service.ShortenURL(req)
	assert.NoError(suite.T(), err)
	assert.NotNil(suite.T(), url.ExpiresAt)
	assert.WithinDuration(suite.T(), expirationTime, *url.ExpiresAt, time.Second)
}

func (suite *URLServiceTestSuite) TestGetURL() {
	// First create a URL
	req := &CreateURLRequest{
		LongURL: "https://example.com/get-test",
		UserID:  "test_user_123",
	}

	createdURL, err := suite.service.ShortenURL(req)
	assert.NoError(suite.T(), err)

	// Test getting the URL
	retrievedURL, err := suite.service.GetURL(createdURL.ShortCode, "test_user_123")
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), createdURL.ShortCode, retrievedURL.ShortCode)
	assert.Equal(suite.T(), createdURL.LongURL, retrievedURL.LongURL)
	assert.Equal(suite.T(), createdURL.UserID, retrievedURL.UserID)
}

func (suite *URLServiceTestSuite) TestGetURLUnauthorized() {
	// First create a URL
	req := &CreateURLRequest{
		LongURL: "https://example.com/unauthorized-test",
		UserID:  "test_user_123",
	}

	createdURL, err := suite.service.ShortenURL(req)
	assert.NoError(suite.T(), err)

	// Test getting the URL with wrong user
	_, err = suite.service.GetURL(createdURL.ShortCode, "wrong_user")
	assert.Error(suite.T(), err)
	assert.Equal(suite.T(), ErrUnauthorized, err)
}

func (suite *URLServiceTestSuite) TestDeleteURL() {
	// First create a URL
	req := &CreateURLRequest{
		LongURL: "https://example.com/delete-test",
		UserID:  "test_user_123",
	}

	createdURL, err := suite.service.ShortenURL(req)
	assert.NoError(suite.T(), err)

	// Delete the URL
	err = suite.service.DeleteURL(createdURL.ShortCode, "test_user_123")
	assert.NoError(suite.T(), err)

	// Try to get the deleted URL
	_, err = suite.service.GetURL(createdURL.ShortCode, "test_user_123")
	assert.Error(suite.T(), err)
	assert.Equal(suite.T(), ErrURLNotFound, err)
}

func (suite *URLServiceTestSuite) TestValidateURL() {
	// Test invalid URLs
	invalidURLs := []string{
		"",
		"not-a-url",
		"ftp://example.com",
		"http://",
		"https://",
	}

	for _, invalidURL := range invalidURLs {
		err := suite.service.validateURL(invalidURL)
		assert.Error(suite.T(), err, "URL should be invalid: %s", invalidURL)
		assert.Equal(suite.T(), ErrInvalidURL, err)
	}

	// Test valid URLs
	validURLs := []string{
		"https://example.com",
		"http://example.com",
		"https://example.com/path",
		"https://subdomain.example.com",
	}

	for _, validURL := range validURLs {
		err := suite.service.validateURL(validURL)
		assert.NoError(suite.T(), err, "URL should be valid: %s", validURL)
	}
}

func (suite *URLServiceTestSuite) TestValidateShortCode() {
	// Test invalid short codes
	invalidCodes := []string{
		"",
		"ab",          // too short
		"abcdefghijk", // too long
		"abc-def",     // contains hyphen
		"abc def",     // contains space
		"abc@def",     // contains special char
	}

	for _, invalidCode := range invalidCodes {
		err := suite.service.validateShortCode(invalidCode)
		assert.Error(suite.T(), err, "Short code should be invalid: %s", invalidCode)
		assert.Equal(suite.T(), ErrInvalidShortCode, err)
	}

	// Test valid short codes
	validCodes := []string{
		"abc",
		"abc123",
		"ABC123",
		"abcDEF123",
	}

	for _, validCode := range validCodes {
		err := suite.service.validateShortCode(validCode)
		assert.NoError(suite.T(), err, "Short code should be valid: %s", validCode)
	}
}

func TestURLServiceTestSuite(t *testing.T) {
	suite.Run(t, new(URLServiceTestSuite))
}
