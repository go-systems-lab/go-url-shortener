package domain

import (
	"errors"
	"time"
)

// URL represents the core URL entity from HLD design
type URL struct {
	ID           int64             `json:"id" db:"id"`
	ShortCode    string            `json:"short_code" db:"short_code"`
	LongURL      string            `json:"long_url" db:"long_url"`
	UserID       string            `json:"user_id" db:"user_id"`
	CreatedAt    time.Time         `json:"created_at" db:"created_at"`
	ExpiresAt    *time.Time        `json:"expires_at" db:"expires_at"`
	ClickCount   int64             `json:"click_count" db:"click_count"`
	LastAccessed *time.Time        `json:"last_accessed" db:"last_accessed"`
	IsActive     bool              `json:"is_active" db:"is_active"`
	Metadata     map[string]string `json:"metadata" db:"metadata"`
}

// CreateURLRequest represents the business logic request for creating a short URL
type CreateURLRequest struct {
	LongURL        string            `json:"long_url"`
	CustomAlias    string            `json:"custom_alias,omitempty"`
	ExpirationTime *time.Time        `json:"expiration_time,omitempty"`
	UserID         string            `json:"user_id"`
	Metadata       map[string]string `json:"metadata,omitempty"`
}

// UpdateURLRequest represents the business logic request for updating a URL
type UpdateURLRequest struct {
	ShortCode         string            `json:"short_code"`
	UserID            string            `json:"user_id"`
	NewLongURL        string            `json:"new_long_url,omitempty"`
	NewExpirationTime *time.Time        `json:"new_expiration_time,omitempty"`
	Metadata          map[string]string `json:"metadata,omitempty"`
}

// GetUserURLsRequest represents pagination and filtering for user URLs
type GetUserURLsRequest struct {
	UserID    string `json:"user_id"`
	Page      int32  `json:"page"`
	PageSize  int32  `json:"page_size"`
	SortBy    string `json:"sort_by"`
	SortOrder string `json:"sort_order"`
}

// GetUserURLsResponse represents paginated user URLs response
type GetUserURLsResponse struct {
	URLs       []URL `json:"urls"`
	TotalCount int32 `json:"total_count"`
	Page       int32 `json:"page"`
	PageSize   int32 `json:"page_size"`
	HasNext    bool  `json:"has_next"`
}

// Domain-specific errors (from HLD design)
var (
	ErrInvalidURL       = errors.New("invalid URL format")
	ErrURLNotFound      = errors.New("URL not found")
	ErrURLExpired       = errors.New("URL has expired")
	ErrUnauthorized     = errors.New("unauthorized access to URL")
	ErrCustomAliasUsed  = errors.New("custom alias already exists")
	ErrInvalidShortCode = errors.New("invalid short code format")
)

// IsExpired checks if the URL has expired (business rule from HLD)
func (u *URL) IsExpired() bool {
	if u.ExpiresAt == nil {
		return false
	}
	return time.Now().After(*u.ExpiresAt)
}

// CanAccess checks if a user can access this URL (authorization business rule)
func (u *URL) CanAccess(userID string) bool {
	return u.UserID == userID
}

// IsValidForRedirect checks if URL is valid for redirect (business rule from HLD)
func (u *URL) IsValidForRedirect() error {
	if !u.IsActive {
		return ErrURLNotFound
	}
	if u.IsExpired() {
		return ErrURLExpired
	}
	return nil
}
