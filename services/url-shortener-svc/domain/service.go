package domain

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"time"

	"github.com/go-systems-lab/go-url-shortener/utils/cache"
	"github.com/go-systems-lab/go-url-shortener/utils/database"
)

// URLService implements the core business logic from HLD design
type URLService struct {
	db    *database.PostgreSQL
	cache *cache.Redis
}

// NewURLService creates a new URL service instance
func NewURLService(db *database.PostgreSQL, redisCache *cache.Redis) *URLService {
	return &URLService{
		db:    db,
		cache: redisCache,
	}
}

// ShortenURL implements the core URL shortening algorithm from HLD
func (s *URLService) ShortenURL(req *CreateURLRequest) (*URL, error) {
	// Validate the long URL (business rule from HLD)
	if err := s.validateURL(req.LongURL); err != nil {
		return nil, fmt.Errorf("URL validation failed: %w", err)
	}

	// Generate or validate custom short code
	var shortCode string
	if req.CustomAlias != "" {
		if err := s.validateShortCode(req.CustomAlias); err != nil {
			return nil, fmt.Errorf("invalid custom alias: %w", err)
		}
		// Check if custom alias is available
		existing, _ := s.db.GetURLByShortCode(req.CustomAlias)
		if existing != nil {
			return nil, ErrCustomAliasUsed
		}
		shortCode = req.CustomAlias
	} else {
		// Generate unique short code using algorithm from HLD
		var err error
		shortCode, err = s.generateShortCode()
		if err != nil {
			return nil, fmt.Errorf("failed to generate short code: %w", err)
		}
	}

	// Convert metadata to JSON string for storage
	metadataJSON := "{}"
	if req.Metadata != nil && len(req.Metadata) > 0 {
		metadataBytes, err := json.Marshal(req.Metadata)
		if err != nil {
			return nil, fmt.Errorf("failed to serialize metadata: %w", err)
		}
		metadataJSON = string(metadataBytes)
	}

	// Create URL mapping in database (from HLD design)
	dbURL := &database.URLMapping{
		ShortCode: shortCode,
		LongURL:   req.LongURL,
		UserID:    req.UserID,
		IsActive:  true,
		Metadata:  metadataJSON,
	}

	// Set expiration if provided
	if req.ExpirationTime != nil {
		dbURL.ExpiresAt.Valid = true
		dbURL.ExpiresAt.Time = *req.ExpirationTime
	}

	// Save to database
	if err := s.db.CreateURL(dbURL); err != nil {
		return nil, fmt.Errorf("failed to save URL: %w", err)
	}

	// Cache the URL mapping for fast lookups (from HLD caching strategy)
	cacheKey := cache.URLCacheKey(shortCode)
	urlData := map[string]interface{}{
		"long_url":   req.LongURL,
		"user_id":    req.UserID,
		"created_at": dbURL.CreatedAt.Unix(),
		"is_active":  true,
	}

	if req.ExpirationTime != nil {
		urlData["expires_at"] = req.ExpirationTime.Unix()
	}

	// Cache with appropriate TTL (from HLD design)
	ttl := time.Hour * 24 // Default 24 hours
	if req.ExpirationTime != nil && req.ExpirationTime.Before(time.Now().Add(ttl)) {
		ttl = time.Until(*req.ExpirationTime)
	}

	if err := s.cache.SetJSON(cacheKey, urlData, ttl); err != nil {
		// Log warning but don't fail the request
		fmt.Printf("Warning: Failed to cache URL mapping: %v\n", err)
	}

	// Convert back to domain model
	return s.dbToDomainURL(dbURL), nil
}

// GetURL retrieves URL information with caching (from HLD design)
func (s *URLService) GetURL(shortCode, userID string) (*URL, error) {
	// Try cache first (from HLD caching strategy)
	cacheKey := cache.URLCacheKey(shortCode)
	var cachedData map[string]interface{}
	if found, err := s.cache.GetJSON(cacheKey, &cachedData); err == nil && found {
		url := s.cacheToURL(shortCode, cachedData)
		if url != nil {
			// Check authorization
			if userID != "" && !url.CanAccess(userID) {
				return nil, ErrUnauthorized
			}
			return url, nil
		}
	}

	// Fallback to database
	dbURL, err := s.db.GetURLByShortCode(shortCode)
	if err != nil {
		return nil, ErrURLNotFound
	}

	url := s.dbToDomainURL(dbURL)

	// Check authorization
	if userID != "" && !url.CanAccess(userID) {
		return nil, ErrUnauthorized
	}

	// Cache for future requests
	s.cacheURL(url)

	return url, nil
}

// GetUserURLs retrieves URLs for a user with pagination (from HLD design)
func (s *URLService) GetUserURLs(req *GetUserURLsRequest) (*GetUserURLsResponse, error) {
	// Validate pagination parameters
	if req.PageSize <= 0 || req.PageSize > 100 {
		req.PageSize = 20 // Default page size
	}
	if req.Page <= 0 {
		req.Page = 1
	}

	offset := (req.Page - 1) * req.PageSize

	// Get URLs from database with pagination
	dbURLs, err := s.db.GetURLsByUserID(req.UserID, int(req.PageSize+1), int(offset))
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve user URLs: %w", err)
	}

	// Check if there are more pages
	hasNext := len(dbURLs) > int(req.PageSize)
	if hasNext {
		dbURLs = dbURLs[:req.PageSize] // Remove the extra item
	}

	// Convert to domain models
	urls := make([]URL, len(dbURLs))
	for i, dbURL := range dbURLs {
		urls[i] = *s.dbToDomainURL(&dbURL)
	}

	return &GetUserURLsResponse{
		URLs:       urls,
		TotalCount: int32(len(urls)), // TODO: Get actual total count in production
		Page:       req.Page,
		PageSize:   req.PageSize,
		HasNext:    hasNext,
	}, nil
}

// UpdateURL updates an existing URL (from HLD design)
func (s *URLService) UpdateURL(req *UpdateURLRequest) (*URL, error) {
	// Get existing URL and check authorization
	existingURL, err := s.GetURL(req.ShortCode, req.UserID)
	if err != nil {
		return nil, err
	}

	// Update fields if provided
	updated := false
	if req.NewLongURL != "" {
		if err := s.validateURL(req.NewLongURL); err != nil {
			return nil, fmt.Errorf("invalid new URL: %w", err)
		}
		existingURL.LongURL = req.NewLongURL
		updated = true
	}

	if req.NewExpirationTime != nil {
		existingURL.ExpiresAt = req.NewExpirationTime
		updated = true
	}

	if req.Metadata != nil {
		existingURL.Metadata = req.Metadata
		updated = true
	}

	if !updated {
		return existingURL, nil
	}

	// TODO: Implement database update operation
	// For now, return the updated model

	// Invalidate cache
	cacheKey := cache.URLCacheKey(req.ShortCode)
	s.cache.Delete(cacheKey)

	return existingURL, nil
}

// DeleteURL soft deletes a URL (from HLD design)
func (s *URLService) DeleteURL(shortCode, userID string) error {
	// Check if URL exists and user has permission
	_, err := s.GetURL(shortCode, userID)
	if err != nil {
		return err
	}

	// Soft delete in database
	if err := s.db.DeleteURL(shortCode, userID); err != nil {
		return fmt.Errorf("failed to delete URL: %w", err)
	}

	// Remove from cache
	cacheKey := cache.URLCacheKey(shortCode)
	s.cache.Delete(cacheKey)

	return nil
}

// generateShortCode generates a unique short code using the algorithm from HLD
func (s *URLService) generateShortCode() (string, error) {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	const length = 7 // 62^7 = ~3.5 trillion combinations

	for attempts := 0; attempts < 10; attempts++ {
		// Generate random bytes
		bytes := make([]byte, length)
		if _, err := rand.Read(bytes); err != nil {
			return "", fmt.Errorf("failed to generate random bytes: %w", err)
		}

		// Convert to base62
		shortCode := make([]byte, length)
		for i, b := range bytes {
			shortCode[i] = charset[int(b)%len(charset)]
		}

		code := string(shortCode)

		// Check if code already exists
		existing, _ := s.db.GetURLByShortCode(code)
		if existing == nil {
			return code, nil
		}
	}

	return "", fmt.Errorf("failed to generate unique short code after 10 attempts")
}

// validateURL validates the long URL format (business rule from HLD)
func (s *URLService) validateURL(longURL string) error {
	if longURL == "" {
		return ErrInvalidURL
	}

	// Parse URL
	parsedURL, err := url.Parse(longURL)
	if err != nil {
		return ErrInvalidURL
	}

	// Check if scheme is present and valid
	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		return ErrInvalidURL
	}

	// Check if host is present
	if parsedURL.Host == "" {
		return ErrInvalidURL
	}

	return nil
}

// validateShortCode validates custom short code format (business rule from HLD)
func (s *URLService) validateShortCode(shortCode string) error {
	if len(shortCode) < 3 || len(shortCode) > 10 {
		return ErrInvalidShortCode
	}

	// Only allow alphanumeric characters
	matched, _ := regexp.MatchString("^[a-zA-Z0-9]+$", shortCode)
	if !matched {
		return ErrInvalidShortCode
	}

	return nil
}

// Helper functions for data conversion

func (s *URLService) dbToDomainURL(dbURL *database.URLMapping) *URL {
	var metadata map[string]string
	if dbURL.Metadata != "" && dbURL.Metadata != "{}" {
		json.Unmarshal([]byte(dbURL.Metadata), &metadata)
	}

	var expiresAt *time.Time
	if dbURL.ExpiresAt.Valid {
		expiresAt = &dbURL.ExpiresAt.Time
	}

	var lastAccessed *time.Time
	if dbURL.LastAccessed.Valid {
		lastAccessed = &dbURL.LastAccessed.Time
	}

	return &URL{
		ID:           dbURL.ID,
		ShortCode:    dbURL.ShortCode,
		LongURL:      dbURL.LongURL,
		UserID:       dbURL.UserID,
		CreatedAt:    dbURL.CreatedAt,
		ExpiresAt:    expiresAt,
		ClickCount:   dbURL.ClickCount,
		LastAccessed: lastAccessed,
		IsActive:     dbURL.IsActive,
		Metadata:     metadata,
	}
}

func (s *URLService) cacheToURL(shortCode string, data map[string]interface{}) *URL {
	// Enhanced implementation with proper user handling
	longURL, ok := data["long_url"].(string)
	if !ok {
		return nil
	}

	userID, _ := data["user_id"].(string)
	isActive, _ := data["is_active"].(bool)

	url := &URL{
		ShortCode: shortCode,
		LongURL:   longURL,
		UserID:    userID,
		IsActive:  isActive,
	}

	// Handle created_at if present
	if createdAtUnix, ok := data["created_at"].(float64); ok {
		url.CreatedAt = time.Unix(int64(createdAtUnix), 0)
	}

	// Handle expires_at if present
	if expiresAtUnix, ok := data["expires_at"].(float64); ok {
		expiresAt := time.Unix(int64(expiresAtUnix), 0)
		url.ExpiresAt = &expiresAt
	}

	return url
}

func (s *URLService) cacheURL(url *URL) {
	cacheKey := cache.URLCacheKey(url.ShortCode)
	urlData := map[string]interface{}{
		"long_url":   url.LongURL,
		"user_id":    url.UserID,
		"created_at": url.CreatedAt.Unix(),
		"is_active":  url.IsActive,
	}

	ttl := time.Hour * 24
	if url.ExpiresAt != nil {
		urlData["expires_at"] = url.ExpiresAt.Unix()
		if url.ExpiresAt.Before(time.Now().Add(ttl)) {
			ttl = time.Until(*url.ExpiresAt)
		}
	}

	s.cache.SetJSON(cacheKey, urlData, ttl)
}
