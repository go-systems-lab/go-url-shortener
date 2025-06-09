package domain

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/go-systems-lab/go-url-shortener/services/url-shortener-svc/domain"
)

// RedirectService handles URL resolution and click tracking business logic
type RedirectService struct {
	store UrlStore
}

// UrlStore interface for data access (following hexagonal architecture)
type UrlStore interface {
	ResolveURL(ctx context.Context, shortCode string) (*domain.URL, error)
	IncrementClickCount(ctx context.Context, shortCode string) error
	GetClickCount(ctx context.Context, shortCode string) (int64, error)
	PrewarmCache(ctx context.Context, shortCodes []string) error
	InvalidateCache(ctx context.Context, shortCode string) error
	GetCacheStats(ctx context.Context) (map[string]interface{}, error)
}

// ClickInfo represents click analytics data
type ClickInfo struct {
	ShortCode  string
	LongURL    string
	ClientIP   string
	UserAgent  string
	Referrer   string
	Country    string
	City       string
	DeviceType string
	Browser    string
	OS         string
	Timestamp  time.Time
	SessionID  string
	IsUnique   bool
}

// RedirectResult represents the result of a URL resolution
type RedirectResult struct {
	LongURL    string
	Found      bool
	Expired    bool
	CreatedAt  time.Time
	ExpiresAt  *time.Time
	ClickCount int64
	Error      string
}

// NewRedirectService creates a new redirect service
func NewRedirectService(store UrlStore) *RedirectService {
	return &RedirectService{
		store: store,
	}
}

// ResolveURL resolves a short code to long URL with business logic validation
func (s *RedirectService) ResolveURL(ctx context.Context, shortCode string, clientInfo ClientInfo) (*RedirectResult, error) {
	fmt.Printf("🔍 [DEBUG] Domain ResolveURL called with shortCode: %s (length: %d)\n", shortCode, len(shortCode))

	// 1. Validate short code format (business rule)
	if err := s.validateShortCode(shortCode); err != nil {
		fmt.Printf("❌ [DEBUG] Short code validation failed: %v\n", err)
		return &RedirectResult{
			Found: false,
			Error: fmt.Sprintf("invalid short code: %v", err),
		}, nil
	}

	fmt.Printf("✅ [DEBUG] Short code validation passed\n")

	// 2. Resolve URL from store (cache-first strategy)
	fmt.Printf("🔍 [DEBUG] Calling store.ResolveURL for shortCode: %s\n", shortCode)
	urlEntity, err := s.store.ResolveURL(ctx, shortCode)
	if err != nil {
		fmt.Printf("❌ [DEBUG] store.ResolveURL failed: %v\n", err)
		if strings.Contains(err.Error(), "not found") {
			return &RedirectResult{
				Found: false,
				Error: "Short URL not found",
			}, nil
		}
		if strings.Contains(err.Error(), "expired") {
			return &RedirectResult{
				Found:   true,
				Expired: true,
				Error:   "Short URL has expired",
			}, nil
		}
		return nil, fmt.Errorf("failed to resolve URL: %w", err)
	}

	fmt.Printf("✅ [DEBUG] store.ResolveURL success: %s -> %s\n", shortCode, urlEntity.LongURL)

	// 3. Apply business rules (from HLD design)
	if err := urlEntity.IsValidForRedirect(); err != nil {
		fmt.Printf("❌ [DEBUG] URL validation failed: %v\n", err)
		if err == domain.ErrURLExpired {
			return &RedirectResult{
				Found:     true,
				Expired:   true,
				LongURL:   urlEntity.LongURL,
				CreatedAt: urlEntity.CreatedAt,
				ExpiresAt: urlEntity.ExpiresAt,
				Error:     "URL has expired",
			}, nil
		}
		return &RedirectResult{
			Found: false,
			Error: err.Error(),
		}, nil
	}

	// 4. Validate destination URL (security check)
	if err := s.validateDestinationURL(urlEntity.LongURL); err != nil {
		fmt.Printf("❌ [DEBUG] Destination URL validation failed: %v\n", err)
		return &RedirectResult{
			Found: false,
			Error: fmt.Sprintf("invalid destination URL: %v", err),
		}, nil
	}

	fmt.Printf("✅ [DEBUG] All validations passed, returning success\n")

	// 5. Increment click count (async for performance)
	go func() {
		if err := s.store.IncrementClickCount(context.Background(), shortCode); err != nil {
			// Log error but don't fail the redirect
			fmt.Printf("Failed to increment click count for %s: %v\n", shortCode, err)
		}
	}()

	return &RedirectResult{
		LongURL:    urlEntity.LongURL,
		Found:      true,
		Expired:    false,
		CreatedAt:  urlEntity.CreatedAt,
		ExpiresAt:  urlEntity.ExpiresAt,
		ClickCount: urlEntity.ClickCount,
	}, nil
}

// TrackClick creates click analytics data for NATS publishing
func (s *RedirectService) TrackClick(ctx context.Context, shortCode string, longURL string, clientInfo ClientInfo) (*ClickInfo, error) {
	// Parse user agent for device/browser information
	deviceInfo := s.parseUserAgent(clientInfo.UserAgent)

	// Determine if this is a unique click (simplified logic)
	isUnique := s.isUniqueClick(ctx, shortCode, clientInfo.ClientIP)

	clickInfo := &ClickInfo{
		ShortCode:  shortCode,
		LongURL:    longURL,
		ClientIP:   clientInfo.ClientIP,
		UserAgent:  clientInfo.UserAgent,
		Referrer:   clientInfo.Referrer,
		Country:    clientInfo.Country,
		DeviceType: deviceInfo.DeviceType,
		Browser:    deviceInfo.Browser,
		OS:         deviceInfo.OS,
		Timestamp:  time.Now(),
		SessionID:  s.generateSessionID(clientInfo),
		IsUnique:   isUnique,
	}

	return clickInfo, nil
}

// ClientInfo represents client request information
type ClientInfo struct {
	ClientIP   string
	UserAgent  string
	Referrer   string
	Country    string
	DeviceType string
}

// DeviceInfo represents parsed device information
type DeviceInfo struct {
	DeviceType string // mobile, desktop, tablet
	Browser    string // Chrome, Firefox, Safari, etc.
	OS         string // Windows, macOS, Linux, iOS, Android
}

// validateShortCode validates the format of a short code
func (s *RedirectService) validateShortCode(shortCode string) error {
	if shortCode == "" {
		return fmt.Errorf("short code is empty")
	}

	// Check length (updated to be more flexible: 3-10 characters)
	if len(shortCode) < 3 || len(shortCode) > 10 {
		return fmt.Errorf("short code must be 3-10 characters long")
	}

	// Check characters (alphanumeric only)
	matched, _ := regexp.MatchString("^[a-zA-Z0-9]+$", shortCode)
	if !matched {
		return fmt.Errorf("short code must contain only alphanumeric characters")
	}

	return nil
}

// validateDestinationURL validates the destination URL for security
func (s *RedirectService) validateDestinationURL(longURL string) error {
	parsedURL, err := url.Parse(longURL)
	if err != nil {
		return fmt.Errorf("invalid URL format: %v", err)
	}

	// Must have valid scheme
	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		return fmt.Errorf("URL must use http or https scheme")
	}

	// Must have valid host
	if parsedURL.Host == "" {
		return fmt.Errorf("URL must have a valid host")
	}

	// Security check: block private IP ranges
	if s.isPrivateIP(parsedURL.Host) {
		return fmt.Errorf("cannot redirect to private IP addresses")
	}

	// Security check: block localhost
	if strings.Contains(parsedURL.Host, "localhost") || strings.Contains(parsedURL.Host, "127.0.0.1") {
		return fmt.Errorf("cannot redirect to localhost")
	}

	return nil
}

// isPrivateIP checks if the host is a private IP address
func (s *RedirectService) isPrivateIP(host string) bool {
	// Extract IP from host (could be host:port)
	hostIP := host
	if strings.Contains(host, ":") {
		hostIP, _, _ = net.SplitHostPort(host)
	}

	ip := net.ParseIP(hostIP)
	if ip == nil {
		return false // Not an IP, probably a domain
	}

	// Check private IP ranges
	private10 := net.ParseIP("10.0.0.0")
	private172 := net.ParseIP("172.16.0.0")
	private192 := net.ParseIP("192.168.0.0")

	return ip.IsPrivate() ||
		(ip.Mask(net.CIDRMask(8, 32)).Equal(private10)) ||
		(ip.Mask(net.CIDRMask(12, 32)).Equal(private172)) ||
		(ip.Mask(net.CIDRMask(16, 32)).Equal(private192))
}

// parseUserAgent extracts device and browser information from user agent
func (s *RedirectService) parseUserAgent(userAgent string) DeviceInfo {
	ua := strings.ToLower(userAgent)

	// Determine device type
	deviceType := "desktop"
	if strings.Contains(ua, "mobile") || strings.Contains(ua, "android") || strings.Contains(ua, "iphone") {
		deviceType = "mobile"
	} else if strings.Contains(ua, "tablet") || strings.Contains(ua, "ipad") {
		deviceType = "tablet"
	}

	// Determine browser
	browser := "unknown"
	if strings.Contains(ua, "chrome") && !strings.Contains(ua, "edge") {
		browser = "Chrome"
	} else if strings.Contains(ua, "firefox") {
		browser = "Firefox"
	} else if strings.Contains(ua, "safari") && !strings.Contains(ua, "chrome") {
		browser = "Safari"
	} else if strings.Contains(ua, "edge") {
		browser = "Edge"
	} else if strings.Contains(ua, "opera") {
		browser = "Opera"
	}

	// Determine OS
	os := "unknown"
	if strings.Contains(ua, "windows") {
		os = "Windows"
	} else if strings.Contains(ua, "macintosh") || strings.Contains(ua, "mac os x") {
		os = "macOS"
	} else if strings.Contains(ua, "linux") {
		os = "Linux"
	} else if strings.Contains(ua, "android") {
		os = "Android"
	} else if strings.Contains(ua, "iphone") || strings.Contains(ua, "ipad") {
		os = "iOS"
	}

	return DeviceInfo{
		DeviceType: deviceType,
		Browser:    browser,
		OS:         os,
	}
}

// isUniqueClick determines if this is a unique click from this IP (simplified)
func (s *RedirectService) isUniqueClick(ctx context.Context, shortCode string, clientIP string) bool {
	// In a real implementation, you'd check Redis or database for recent clicks
	// For now, we'll use a simple heuristic
	return true // Simplified for demo
}

// generateSessionID generates a session ID for analytics
func (s *RedirectService) generateSessionID(clientInfo ClientInfo) string {
	// Simple session ID based on IP and timestamp
	timestamp := time.Now().Unix() / 300 // 5-minute buckets
	return fmt.Sprintf("%s-%d", clientInfo.ClientIP, timestamp)
}

// GetURLStats retrieves URL statistics for analytics
func (s *RedirectService) GetURLStats(ctx context.Context, shortCode string) (map[string]interface{}, error) {
	clickCount, err := s.store.GetClickCount(ctx, shortCode)
	if err != nil {
		return nil, fmt.Errorf("failed to get click count: %w", err)
	}

	stats := map[string]interface{}{
		"short_code":  shortCode,
		"click_count": clickCount,
		"timestamp":   time.Now(),
	}

	return stats, nil
}

// PrewarmPopularURLs preloads popular URLs into cache
func (s *RedirectService) PrewarmPopularURLs(ctx context.Context, shortCodes []string) error {
	return s.store.PrewarmCache(ctx, shortCodes)
}

// InvalidateURL removes a URL from cache
func (s *RedirectService) InvalidateURL(ctx context.Context, shortCode string) error {
	return s.store.InvalidateCache(ctx, shortCode)
}
