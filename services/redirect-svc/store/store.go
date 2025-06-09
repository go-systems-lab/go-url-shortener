package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/redis/go-redis/v9"

	"github.com/go-systems-lab/go-url-shortener/services/url-shortener-svc/domain"
)

// RedirectStore handles URL resolution with cache-first strategy
type RedirectStore struct {
	db    *sqlx.DB
	redis *redis.Client
}

// CacheEntry represents a cached URL mapping
type CacheEntry struct {
	ShortCode  string     `json:"short_code"`
	LongURL    string     `json:"long_url"`
	CreatedAt  time.Time  `json:"created_at"`
	ExpiresAt  *time.Time `json:"expires_at,omitempty"`
	ClickCount int64      `json:"click_count"`
	IsActive   bool       `json:"is_active"`
}

// NewRedirectStore creates a new redirect store
func NewRedirectStore(db *sqlx.DB, redis *redis.Client) *RedirectStore {
	return &RedirectStore{
		db:    db,
		redis: redis,
	}
}

// ResolveURL resolves a short code to long URL using cache-first strategy
func (s *RedirectStore) ResolveURL(ctx context.Context, shortCode string) (*domain.URL, error) {
	// 1. Check Redis cache first (95% hit ratio expected)
	cacheKey := fmt.Sprintf("url:short:%s", shortCode)

	// Try to get from cache
	cached, err := s.redis.Get(ctx, cacheKey).Result()
	if err == nil {
		// Cache hit - parse and return
		var entry CacheEntry
		if err := json.Unmarshal([]byte(cached), &entry); err == nil {
			// Check if expired
			if entry.ExpiresAt != nil && time.Now().After(*entry.ExpiresAt) {
				// Expired - remove from cache and treat as miss
				s.redis.Del(ctx, cacheKey)
				return nil, fmt.Errorf("URL expired")
			}

			// Valid cache hit
			return &domain.URL{
				ID:         0, // Not needed for redirect
				ShortCode:  entry.ShortCode,
				LongURL:    entry.LongURL,
				CreatedAt:  entry.CreatedAt,
				ExpiresAt:  entry.ExpiresAt,
				ClickCount: entry.ClickCount,
				IsActive:   entry.IsActive,
			}, nil
		}
	}

	// 2. Cache miss - query database (5% of requests)
	// Use a temporary struct without metadata to avoid JSON unmarshaling issues
	var dbResult struct {
		ID           int64      `db:"id"`
		ShortCode    string     `db:"short_code"`
		LongURL      string     `db:"long_url"`
		UserID       string     `db:"user_id"`
		CreatedAt    time.Time  `db:"created_at"`
		ExpiresAt    *time.Time `db:"expires_at"`
		ClickCount   int64      `db:"click_count"`
		LastAccessed *time.Time `db:"last_accessed"`
		IsActive     bool       `db:"is_active"`
	}

	query := `
		SELECT id, short_code, long_url, user_id, created_at, expires_at, 
		       click_count, last_accessed, is_active
		FROM url_mappings 
		WHERE short_code = $1 AND is_active = true
	`

	err = s.db.GetContext(ctx, &dbResult, query, shortCode)
	if err != nil {
		return nil, fmt.Errorf("short URL not found")
	}

	// Convert to domain.URL
	url := &domain.URL{
		ID:           dbResult.ID,
		ShortCode:    dbResult.ShortCode,
		LongURL:      dbResult.LongURL,
		UserID:       dbResult.UserID,
		CreatedAt:    dbResult.CreatedAt,
		ExpiresAt:    dbResult.ExpiresAt,
		ClickCount:   dbResult.ClickCount,
		LastAccessed: dbResult.LastAccessed,
		IsActive:     dbResult.IsActive,
		Metadata:     make(map[string]string), // Empty metadata for redirect service
	}

	// Check if URL has expired
	if url.ExpiresAt != nil && time.Now().After(*url.ExpiresAt) {
		return nil, fmt.Errorf("URL expired")
	}

	// 3. Update cache for future requests (write-through)
	cacheEntry := CacheEntry{
		ShortCode:  url.ShortCode,
		LongURL:    url.LongURL,
		CreatedAt:  url.CreatedAt,
		ExpiresAt:  url.ExpiresAt,
		ClickCount: url.ClickCount,
		IsActive:   url.IsActive,
	}

	if entryJSON, err := json.Marshal(cacheEntry); err == nil {
		// Cache for 24 hours as per HLD
		s.redis.Set(ctx, cacheKey, entryJSON, 24*time.Hour)
	}

	return url, nil
}

// IncrementClickCount atomically increments the click count
func (s *RedirectStore) IncrementClickCount(ctx context.Context, shortCode string) error {
	// 1. Increment in database (persistent)
	query := `
		UPDATE url_mappings 
		SET click_count = click_count + 1, 
		    last_accessed = NOW() 
		WHERE short_code = $1
	`

	_, err := s.db.ExecContext(ctx, query, shortCode)
	if err != nil {
		return fmt.Errorf("failed to increment database count: %w", err)
	}

	// 2. Increment in cache (for fast access)
	cacheKey := fmt.Sprintf("url:short:%s", shortCode)

	// Get current cache entry
	cached, err := s.redis.Get(ctx, cacheKey).Result()
	if err == nil {
		var entry CacheEntry
		if json.Unmarshal([]byte(cached), &entry) == nil {
			// Increment count and update cache
			entry.ClickCount++
			if entryJSON, err := json.Marshal(entry); err == nil {
				s.redis.Set(ctx, cacheKey, entryJSON, 24*time.Hour)
			}
		}
	}

	// 3. Track click count in Redis counter (for analytics)
	counterKey := fmt.Sprintf("clicks:counter:%s", shortCode)
	s.redis.Incr(ctx, counterKey)
	s.redis.Expire(ctx, counterKey, 30*24*time.Hour) // 30 days retention

	return nil
}

// GetClickCount gets the current click count from cache or database
func (s *RedirectStore) GetClickCount(ctx context.Context, shortCode string) (int64, error) {
	// Try cache first
	cacheKey := fmt.Sprintf("url:short:%s", shortCode)
	cached, err := s.redis.Get(ctx, cacheKey).Result()
	if err == nil {
		var entry CacheEntry
		if json.Unmarshal([]byte(cached), &entry) == nil {
			return entry.ClickCount, nil
		}
	}

	// Fallback to database
	var clickCount int64
	query := `SELECT click_count FROM url_mappings WHERE short_code = $1`
	err = s.db.GetContext(ctx, &clickCount, query, shortCode)
	if err != nil {
		return 0, fmt.Errorf("short URL not found")
	}

	return clickCount, nil
}

// PrewarmCache preloads popular URLs into cache
func (s *RedirectStore) PrewarmCache(ctx context.Context, shortCodes []string) error {
	if len(shortCodes) == 0 {
		return nil
	}

	// Batch fetch from database
	query := `
		SELECT short_code, long_url, created_at, expires_at, click_count, is_active
		FROM url_mappings 
		WHERE short_code = ANY($1) AND is_active = true
	`

	rows, err := s.db.QueryContext(ctx, query, shortCodes)
	if err != nil {
		return fmt.Errorf("failed to fetch URLs for prewarming: %w", err)
	}
	defer rows.Close()

	// Batch update cache
	pipe := s.redis.Pipeline()
	for rows.Next() {
		var entry CacheEntry
		err := rows.Scan(&entry.ShortCode, &entry.LongURL, &entry.CreatedAt,
			&entry.ExpiresAt, &entry.ClickCount, &entry.IsActive)
		if err == nil {
			if entryJSON, err := json.Marshal(entry); err == nil {
				cacheKey := fmt.Sprintf("url:short:%s", entry.ShortCode)
				pipe.Set(ctx, cacheKey, entryJSON, 24*time.Hour)
			}
		}
	}

	_, err = pipe.Exec(ctx)
	return err
}

// InvalidateCache removes a URL from cache (useful for updates/deletions)
func (s *RedirectStore) InvalidateCache(ctx context.Context, shortCode string) error {
	cacheKey := fmt.Sprintf("url:short:%s", shortCode)
	return s.redis.Del(ctx, cacheKey).Err()
}

// GetCacheStats returns cache performance metrics
func (s *RedirectStore) GetCacheStats(ctx context.Context) (map[string]interface{}, error) {
	info := s.redis.Info(ctx, "stats").Val()

	stats := map[string]interface{}{
		"cache_info": info,
		"timestamp":  time.Now(),
	}

	return stats, nil
}
