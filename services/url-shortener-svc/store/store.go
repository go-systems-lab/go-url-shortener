package store

import (
	"time"

	"github.com/go-systems-lab/go-url-shortener/services/url-shortener-svc/domain"
	"github.com/go-systems-lab/go-url-shortener/utils/cache"
	"github.com/go-systems-lab/go-url-shortener/utils/database"
)

// URLStore provides a clean interface for URL operations
type URLStore struct {
	service *domain.URLService
}

// NewURLStore creates a new URL store instance
func NewURLStore(db *database.PostgreSQL, cache *cache.Redis) *URLStore {
	service := domain.NewURLService(db, cache)
	return &URLStore{
		service: service,
	}
}

// ShortenURLRequest represents the store-level request for URL shortening
type ShortenURLRequest struct {
	LongURL        string            `json:"long_url"`
	CustomAlias    string            `json:"custom_alias,omitempty"`
	ExpirationTime *time.Time        `json:"expiration_time,omitempty"`
	UserID         string            `json:"user_id"`
	Metadata       map[string]string `json:"metadata,omitempty"`
}

// URLResponse represents the store-level response for URL operations
type URLResponse struct {
	ID           int64             `json:"id"`
	ShortCode    string            `json:"short_code"`
	LongURL      string            `json:"long_url"`
	UserID       string            `json:"user_id"`
	CreatedAt    time.Time         `json:"created_at"`
	ExpiresAt    *time.Time        `json:"expires_at"`
	ClickCount   int64             `json:"click_count"`
	LastAccessed *time.Time        `json:"last_accessed"`
	IsActive     bool              `json:"is_active"`
	Metadata     map[string]string `json:"metadata"`
}

// GetUserURLsRequest represents pagination request for user URLs
type GetUserURLsRequest struct {
	UserID    string `json:"user_id"`
	Page      int32  `json:"page"`
	PageSize  int32  `json:"page_size"`
	SortBy    string `json:"sort_by"`
	SortOrder string `json:"sort_order"`
}

// GetUserURLsResponse represents paginated response for user URLs
type GetUserURLsResponse struct {
	URLs       []URLResponse `json:"urls"`
	TotalCount int32         `json:"total_count"`
	Page       int32         `json:"page"`
	PageSize   int32         `json:"page_size"`
	HasNext    bool          `json:"has_next"`
}

// UpdateURLRequest represents the store-level request for URL updates
type UpdateURLRequest struct {
	ShortCode         string            `json:"short_code"`
	UserID            string            `json:"user_id"`
	NewLongURL        string            `json:"new_long_url,omitempty"`
	NewExpirationTime *time.Time        `json:"new_expiration_time,omitempty"`
	Metadata          map[string]string `json:"metadata,omitempty"`
}

// ShortenURL creates a new short URL
func (s *URLStore) ShortenURL(req *ShortenURLRequest) (*URLResponse, error) {
	domainReq := &domain.CreateURLRequest{
		LongURL:        req.LongURL,
		CustomAlias:    req.CustomAlias,
		ExpirationTime: req.ExpirationTime,
		UserID:         req.UserID,
		Metadata:       req.Metadata,
	}

	url, err := s.service.ShortenURL(domainReq)
	if err != nil {
		return nil, err
	}

	return s.domainToStoreURL(url), nil
}

// GetURL retrieves URL information by short code
func (s *URLStore) GetURL(shortCode, userID string) (*URLResponse, error) {
	url, err := s.service.GetURL(shortCode, userID)
	if err != nil {
		return nil, err
	}

	return s.domainToStoreURL(url), nil
}

// GetUserURLs retrieves URLs for a user with pagination
func (s *URLStore) GetUserURLs(req *GetUserURLsRequest) (*GetUserURLsResponse, error) {
	domainReq := &domain.GetUserURLsRequest{
		UserID:    req.UserID,
		Page:      req.Page,
		PageSize:  req.PageSize,
		SortBy:    req.SortBy,
		SortOrder: req.SortOrder,
	}

	response, err := s.service.GetUserURLs(domainReq)
	if err != nil {
		return nil, err
	}

	// Convert domain URLs to store URLs
	urls := make([]URLResponse, len(response.URLs))
	for i, url := range response.URLs {
		urls[i] = *s.domainToStoreURL(&url)
	}

	return &GetUserURLsResponse{
		URLs:       urls,
		TotalCount: response.TotalCount,
		Page:       response.Page,
		PageSize:   response.PageSize,
		HasNext:    response.HasNext,
	}, nil
}

// UpdateURL updates an existing URL
func (s *URLStore) UpdateURL(req *UpdateURLRequest) (*URLResponse, error) {
	domainReq := &domain.UpdateURLRequest{
		ShortCode:         req.ShortCode,
		UserID:            req.UserID,
		NewLongURL:        req.NewLongURL,
		NewExpirationTime: req.NewExpirationTime,
		Metadata:          req.Metadata,
	}

	url, err := s.service.UpdateURL(domainReq)
	if err != nil {
		return nil, err
	}

	return s.domainToStoreURL(url), nil
}

// DeleteURL soft deletes a URL
func (s *URLStore) DeleteURL(shortCode, userID string) error {
	return s.service.DeleteURL(shortCode, userID)
}

// Helper function to convert domain URL to store URL
func (s *URLStore) domainToStoreURL(url *domain.URL) *URLResponse {
	return &URLResponse{
		ID:           url.ID,
		ShortCode:    url.ShortCode,
		LongURL:      url.LongURL,
		UserID:       url.UserID,
		CreatedAt:    url.CreatedAt,
		ExpiresAt:    url.ExpiresAt,
		ClickCount:   url.ClickCount,
		LastAccessed: url.LastAccessed,
		IsActive:     url.IsActive,
		Metadata:     url.Metadata,
	}
}
