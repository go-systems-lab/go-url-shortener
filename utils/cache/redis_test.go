package cache

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

type RedisTestSuite struct {
	suite.Suite
	redis *Redis
}

func (suite *RedisTestSuite) SetupSuite() {
	// Set test Redis URL
	os.Setenv("REDIS_URL", "redis://localhost:6379/1") // Use database 1 for testing

	// Initialize Redis connection
	suite.redis = NewRedis()

	// Clear test database
	suite.redis.FlushDB()
}

func (suite *RedisTestSuite) TearDownSuite() {
	// Clean up
	if suite.redis != nil {
		suite.redis.FlushDB()
		suite.redis.Close()
	}
}

func (suite *RedisTestSuite) SetupTest() {
	// Clear data before each test
	suite.redis.FlushDB()
}

func (suite *RedisTestSuite) TestRedisConnection() {
	// Test basic connectivity
	err := suite.redis.HealthCheck()
	assert.NoError(suite.T(), err)
}

func (suite *RedisTestSuite) TestSetAndGet() {
	// Test basic set and get
	err := suite.redis.Set("test_key", "test_value", time.Hour)
	assert.NoError(suite.T(), err)

	value, found := suite.redis.Get("test_key")
	assert.True(suite.T(), found)
	assert.Equal(suite.T(), "\"test_value\"", value) // JSON encoded
}

func (suite *RedisTestSuite) TestSetAndGetJSON() {
	// Test JSON set and get
	testData := map[string]interface{}{
		"short_code": "abc123",
		"long_url":   "https://example.com",
		"user_id":    "user_123",
	}

	err := suite.redis.SetJSON("test_json", testData, time.Hour)
	assert.NoError(suite.T(), err)

	var retrieved map[string]interface{}
	found, err := suite.redis.GetJSON("test_json", &retrieved)
	assert.NoError(suite.T(), err)
	assert.True(suite.T(), found)
	assert.Equal(suite.T(), "abc123", retrieved["short_code"])
	assert.Equal(suite.T(), "https://example.com", retrieved["long_url"])
	assert.Equal(suite.T(), "user_123", retrieved["user_id"])
}

func (suite *RedisTestSuite) TestKeyNotFound() {
	// Test getting non-existent key
	value, found := suite.redis.Get("non_existent_key")
	assert.False(suite.T(), found)
	assert.Empty(suite.T(), value)

	var data map[string]interface{}
	found, err := suite.redis.GetJSON("non_existent_json", &data)
	assert.NoError(suite.T(), err)
	assert.False(suite.T(), found)
}

func (suite *RedisTestSuite) TestDelete() {
	// Test delete functionality
	suite.redis.Set("delete_test", "value", time.Hour)

	// Verify it exists
	_, found := suite.redis.Get("delete_test")
	assert.True(suite.T(), found)

	// Delete it
	err := suite.redis.Delete("delete_test")
	assert.NoError(suite.T(), err)

	// Verify it's gone
	_, found = suite.redis.Get("delete_test")
	assert.False(suite.T(), found)
}

func (suite *RedisTestSuite) TestExists() {
	// Test exists functionality
	exists, err := suite.redis.Exists("exists_test")
	assert.NoError(suite.T(), err)
	assert.False(suite.T(), exists)

	// Set a key
	suite.redis.Set("exists_test", "value", time.Hour)

	// Check it exists
	exists, err = suite.redis.Exists("exists_test")
	assert.NoError(suite.T(), err)
	assert.True(suite.T(), exists)
}

func (suite *RedisTestSuite) TestIncrement() {
	// Test increment functionality
	count, err := suite.redis.Increment("counter")
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), int64(1), count)

	count, err = suite.redis.Increment("counter")
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), int64(2), count)

	count, err = suite.redis.IncrementBy("counter", 5)
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), int64(7), count)
}

func (suite *RedisTestSuite) TestTTL() {
	// Test TTL functionality
	suite.redis.Set("ttl_test", "value", time.Minute)

	ttl, err := suite.redis.GetTTL("ttl_test")
	assert.NoError(suite.T(), err)
	assert.Greater(suite.T(), ttl.Seconds(), float64(50)) // Should be close to 60 seconds

	// Update TTL
	err = suite.redis.SetTTL("ttl_test", time.Hour)
	assert.NoError(suite.T(), err)

	ttl, err = suite.redis.GetTTL("ttl_test")
	assert.NoError(suite.T(), err)
	assert.Greater(suite.T(), ttl.Seconds(), float64(3500)) // Should be close to 3600 seconds
}

func (suite *RedisTestSuite) TestPrewarmCache() {
	// Test prewarm cache functionality
	urlMappings := map[string]string{
		"abc123": "https://example1.com",
		"def456": "https://example2.com",
		"ghi789": "https://example3.com",
	}

	err := suite.redis.PrewarmCache(urlMappings, time.Hour)
	assert.NoError(suite.T(), err)

	// Verify all URLs are cached (PrewarmCache stores values directly, not JSON encoded)
	for shortCode, longURL := range urlMappings {
		value, found := suite.redis.Get(shortCode)
		assert.True(suite.T(), found)
		assert.Equal(suite.T(), longURL, value) // Direct value, not JSON encoded
	}
}

func (suite *RedisTestSuite) TestCacheKeys() {
	// Test cache key generation functions
	urlKey := URLCacheKey("abc123")
	assert.Equal(suite.T(), "urlshortener:url:abc123", urlKey)

	analyticsKey := AnalyticsCacheKey("abc123", "clicks")
	assert.Equal(suite.T(), "urlshortener:analytics:abc123:clicks", analyticsKey)

	userKey := UserCacheKey("user123", "profile")
	assert.Equal(suite.T(), "urlshortener:user:user123:profile", userKey)
}

func (suite *RedisTestSuite) TestCacheStats() {
	// Test cache statistics
	stats := suite.redis.GetCacheStats()
	assert.NotNil(suite.T(), stats)
	assert.Contains(suite.T(), stats, "redis_info")
	assert.Contains(suite.T(), stats, "hits")
	assert.Contains(suite.T(), stats, "misses")
}

func TestRedisTestSuite(t *testing.T) {
	suite.Run(t, new(RedisTestSuite))
}
