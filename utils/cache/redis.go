package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/redis/go-redis/v9"
)

// Redis cache manager - Production configuration for 95% cache hit ratio
type Redis struct {
	Client *redis.Client
	ctx    context.Context
}

// CacheItem represents a cached item with metadata
type CacheItem struct {
	Value     string                 `json:"value"`
	CreatedAt time.Time              `json:"created_at"`
	TTL       int64                  `json:"ttl"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

// NewRedis creates a new Redis client with production settings
func NewRedis() *Redis {
	// Get Redis URL from environment
	redisURL := os.Getenv("REDIS_URL")
	if redisURL == "" {
		// Default development configuration
		redisURL = "redis://:redispassword@localhost:6379/0"
	}

	// Parse Redis URL
	opt, err := redis.ParseURL(redisURL)
	if err != nil {
		log.Fatalf("Failed to parse Redis URL: %v", err)
	}

	// Production-optimized Redis configuration
	opt.MaxRetries = 3
	opt.MinRetryBackoff = 8 * time.Millisecond
	opt.MaxRetryBackoff = 512 * time.Millisecond
	opt.PoolSize = 30     // Connection pool size for high concurrency
	opt.MinIdleConns = 10 // Minimum idle connections
	opt.PoolTimeout = 30 * time.Second

	// Create Redis client
	client := redis.NewClient(opt)

	// Test connection
	ctx := context.Background()
	_, err = client.Ping(ctx).Result()
	if err != nil {
		log.Fatalf("Failed to connect to Redis: %v", err)
	}

	log.Println("✅ Successfully connected to Redis")

	return &Redis{
		Client: client,
		ctx:    ctx,
	}
}

// Set stores a value in Redis with TTL (from HLD: 24 hour TTL for URL mappings)
func (r *Redis) Set(key string, value interface{}, ttl time.Duration) error {
	// Serialize value to JSON
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("failed to marshal value: %v", err)
	}

	// Store in Redis with TTL
	err = r.Client.Set(r.ctx, key, data, ttl).Err()
	if err != nil {
		return fmt.Errorf("failed to set cache: %v", err)
	}

	return nil
}

// Get retrieves a value from Redis
func (r *Redis) Get(key string) (string, bool) {
	result, err := r.Client.Get(r.ctx, key).Result()
	if err == redis.Nil {
		return "", false // Key does not exist
	} else if err != nil {
		log.Printf("Redis GET error for key %s: %v", key, err)
		return "", false
	}

	return result, true
}

// GetJSON retrieves and unmarshals JSON data from Redis
func (r *Redis) GetJSON(key string, dest interface{}) (bool, error) {
	result, err := r.Client.Get(r.ctx, key).Result()
	if err == redis.Nil {
		return false, nil // Key does not exist
	} else if err != nil {
		return false, fmt.Errorf("redis error: %v", err)
	}

	err = json.Unmarshal([]byte(result), dest)
	if err != nil {
		return false, fmt.Errorf("failed to unmarshal JSON: %v", err)
	}

	return true, nil
}

// SetJSON stores JSON data in Redis
func (r *Redis) SetJSON(key string, value interface{}, ttl time.Duration) error {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("failed to marshal JSON: %v", err)
	}

	return r.Client.Set(r.ctx, key, data, ttl).Err()
}

// Delete removes a key from Redis
func (r *Redis) Delete(key string) error {
	return r.Client.Del(r.ctx, key).Err()
}

// Exists checks if a key exists in Redis
func (r *Redis) Exists(key string) (bool, error) {
	result, err := r.Client.Exists(r.ctx, key).Result()
	if err != nil {
		return false, err
	}
	return result > 0, nil
}

// Increment increments a counter in Redis (for analytics)
func (r *Redis) Increment(key string) (int64, error) {
	return r.Client.Incr(r.ctx, key).Result()
}

// IncrementBy increments a counter by a specific value
func (r *Redis) IncrementBy(key string, value int64) (int64, error) {
	return r.Client.IncrBy(r.ctx, key, value).Result()
}

// SetTTL sets TTL for an existing key
func (r *Redis) SetTTL(key string, ttl time.Duration) error {
	return r.Client.Expire(r.ctx, key, ttl).Err()
}

// GetTTL returns the TTL of a key
func (r *Redis) GetTTL(key string) (time.Duration, error) {
	return r.Client.TTL(r.ctx, key).Result()
}

// MGet retrieves multiple keys at once (batch operation)
func (r *Redis) MGet(keys ...string) ([]interface{}, error) {
	return r.Client.MGet(r.ctx, keys...).Result()
}

// MSet sets multiple key-value pairs at once
func (r *Redis) MSet(pairs ...interface{}) error {
	return r.Client.MSet(r.ctx, pairs...).Err()
}

// Pipeline creates a Redis pipeline for batch operations
func (r *Redis) Pipeline() redis.Pipeliner {
	return r.Client.Pipeline()
}

// PrewarmCache preloads popular URLs into cache (from HLD prewarm strategy)
func (r *Redis) PrewarmCache(urlMappings map[string]string, ttl time.Duration) error {
	pipe := r.Client.Pipeline()

	for shortCode, longURL := range urlMappings {
		pipe.Set(r.ctx, shortCode, longURL, ttl)
	}

	_, err := pipe.Exec(r.ctx)
	if err != nil {
		return fmt.Errorf("failed to prewarm cache: %v", err)
	}

	log.Printf("✅ Prewarmed cache with %d URL mappings", len(urlMappings))
	return nil
}

// GetCacheStats returns Redis performance statistics
func (r *Redis) GetCacheStats() map[string]interface{} {
	info, err := r.Client.Info(r.ctx, "stats", "memory").Result()
	if err != nil {
		return map[string]interface{}{"error": err.Error()}
	}

	poolStats := r.Client.PoolStats()

	return map[string]interface{}{
		"redis_info":  info,
		"hits":        poolStats.Hits,
		"misses":      poolStats.Misses,
		"timeouts":    poolStats.Timeouts,
		"total_conns": poolStats.TotalConns,
		"idle_conns":  poolStats.IdleConns,
		"stale_conns": poolStats.StaleConns,
	}
}

// HealthCheck verifies Redis connectivity
func (r *Redis) HealthCheck() error {
	_, err := r.Client.Ping(r.ctx).Result()
	return err
}

// FlushDB clears all data (development/testing only)
func (r *Redis) FlushDB() error {
	if os.Getenv("GO_ENV") == "production" {
		return fmt.Errorf("cannot flush Redis in production")
	}
	return r.Client.FlushDB(r.ctx).Err()
}

// Close closes the Redis connection
func (r *Redis) Close() error {
	return r.Client.Close()
}

// CacheKey generates standardized cache keys
func CacheKey(prefix, identifier string) string {
	return fmt.Sprintf("urlshortener:%s:%s", prefix, identifier)
}

// URLCacheKey generates cache key for URL mappings
func URLCacheKey(shortCode string) string {
	return CacheKey("url", shortCode)
}

// AnalyticsCacheKey generates cache key for analytics data
func AnalyticsCacheKey(shortCode, metric string) string {
	return CacheKey("analytics", fmt.Sprintf("%s:%s", shortCode, metric))
}

// UserCacheKey generates cache key for user data
func UserCacheKey(userID, dataType string) string {
	return CacheKey("user", fmt.Sprintf("%s:%s", userID, dataType))
}
