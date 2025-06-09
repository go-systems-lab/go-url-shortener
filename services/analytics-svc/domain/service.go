package domain

import (
	"context"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/ua-parser/uap-go/uaparser"
)

// AnalyticsService interface defines the business logic operations
type AnalyticsService interface {
	ProcessClick(ctx context.Context, event *ClickEvent) error
	GetURLStats(ctx context.Context, shortCode string, startTime, endTime time.Time, granularity string) (*URLStatsReport, error)
	GetTopURLs(ctx context.Context, limit int, startTime, endTime time.Time, sortBy string) ([]*URLStats, error)
	GetDashboard(ctx context.Context, startTime, endTime time.Time) (*DashboardMetrics, error)
}

// AnalyticsStore interface defines data access operations
type AnalyticsStore interface {
	SaveClick(ctx context.Context, click *ClickRecord) error
	GetURLStats(ctx context.Context, shortCode string, startTime, endTime time.Time) (*URLStats, error)
	GetTimeSeriesData(ctx context.Context, shortCode string, startTime, endTime time.Time, granularity string) ([]*TimeSeriesData, error)
	GetCountryStats(ctx context.Context, shortCode string, startTime, endTime time.Time) ([]*CountryMetrics, error)
	GetDeviceStats(ctx context.Context, shortCode string, startTime, endTime time.Time) ([]*DeviceMetrics, error)
	GetBrowserStats(ctx context.Context, shortCode string, startTime, endTime time.Time) ([]*BrowserMetrics, error)
	GetReferrerStats(ctx context.Context, shortCode string, startTime, endTime time.Time) ([]*ReferrerMetrics, error)
	GetTopURLs(ctx context.Context, limit int, startTime, endTime time.Time, sortBy string) ([]*URLStats, error)
	GetDashboardMetrics(ctx context.Context, startTime, endTime time.Time) (*DashboardMetrics, error)
	IsUniqueVisitor(ctx context.Context, shortCode, sessionID string) (bool, error)
}

// ClickEvent represents an incoming click event
type ClickEvent struct {
	ShortCode string
	LongURL   string
	ClientIP  string
	UserAgent string
	Referrer  string
	Timestamp time.Time
	SessionID string
}

// URLStatsReport represents comprehensive URL statistics
type URLStatsReport struct {
	URLStats      *URLStats
	TimeSeries    []*TimeSeriesData
	CountryStats  []*CountryMetrics
	DeviceStats   []*DeviceMetrics
	BrowserStats  []*BrowserMetrics
	ReferrerStats []*ReferrerMetrics
}

// AnalyticsServiceImpl implements the AnalyticsService interface
type AnalyticsServiceImpl struct {
	store  AnalyticsStore
	parser *uaparser.Parser
	log    *logrus.Logger
}

// NewAnalyticsService creates a new analytics service instance
func NewAnalyticsService(store AnalyticsStore, log *logrus.Logger) AnalyticsService {
	parser := uaparser.NewFromSaved()
	return &AnalyticsServiceImpl{
		store:  store,
		parser: parser,
		log:    log,
	}
}

// ProcessClick processes an incoming click event with enrichment
func (s *AnalyticsServiceImpl) ProcessClick(ctx context.Context, event *ClickEvent) error {
	s.log.WithFields(logrus.Fields{
		"short_code": event.ShortCode,
		"client_ip":  event.ClientIP,
		"timestamp":  event.Timestamp,
	}).Info("Processing click event")

	// Parse user agent for device/browser info
	client := s.parser.Parse(event.UserAgent)

	// Determine if this is a unique visitor
	isUnique, err := s.store.IsUniqueVisitor(ctx, event.ShortCode, event.SessionID)
	if err != nil {
		s.log.WithError(err).Error("Failed to check unique visitor")
		isUnique = false // Default to false on error
	}

	// Create enriched click record
	clickRecord := &ClickRecord{
		ShortCode:  event.ShortCode,
		LongURL:    event.LongURL,
		ClientIP:   event.ClientIP,
		UserAgent:  event.UserAgent,
		Referrer:   event.Referrer,
		Country:    s.getCountryFromIP(event.ClientIP),
		City:       s.getCityFromIP(event.ClientIP),
		DeviceType: s.getDeviceType(client),
		Browser:    fmt.Sprintf("%s %s", client.UserAgent.Family, client.UserAgent.Major),
		OS:         fmt.Sprintf("%s %s", client.Os.Family, client.Os.Major),
		Timestamp:  event.Timestamp,
		SessionID:  event.SessionID,
		IsUnique:   isUnique,
		CreatedAt:  time.Now(),
	}

	// Save the click record
	if err := s.store.SaveClick(ctx, clickRecord); err != nil {
		s.log.WithError(err).Error("Failed to save click record")
		return fmt.Errorf("failed to save click: %w", err)
	}

	s.log.WithFields(logrus.Fields{
		"short_code": event.ShortCode,
		"is_unique":  isUnique,
		"country":    clickRecord.Country,
		"device":     clickRecord.DeviceType,
	}).Info("Click processed successfully")

	return nil
}

// GetURLStats retrieves comprehensive statistics for a URL
func (s *AnalyticsServiceImpl) GetURLStats(ctx context.Context, shortCode string, startTime, endTime time.Time, granularity string) (*URLStatsReport, error) {
	s.log.WithFields(logrus.Fields{
		"short_code":  shortCode,
		"start_time":  startTime,
		"end_time":    endTime,
		"granularity": granularity,
	}).Info("Getting URL statistics")

	// Get basic URL stats
	urlStats, err := s.store.GetURLStats(ctx, shortCode, startTime, endTime)
	if err != nil {
		return nil, fmt.Errorf("failed to get URL stats: %w", err)
	}

	// Get time series data
	timeSeries, err := s.store.GetTimeSeriesData(ctx, shortCode, startTime, endTime, granularity)
	if err != nil {
		return nil, fmt.Errorf("failed to get time series data: %w", err)
	}

	// Get country stats
	countryStats, err := s.store.GetCountryStats(ctx, shortCode, startTime, endTime)
	if err != nil {
		return nil, fmt.Errorf("failed to get country stats: %w", err)
	}

	// Get device stats
	deviceStats, err := s.store.GetDeviceStats(ctx, shortCode, startTime, endTime)
	if err != nil {
		return nil, fmt.Errorf("failed to get device stats: %w", err)
	}

	// Get browser stats
	browserStats, err := s.store.GetBrowserStats(ctx, shortCode, startTime, endTime)
	if err != nil {
		return nil, fmt.Errorf("failed to get browser stats: %w", err)
	}

	// Get referrer stats
	referrerStats, err := s.store.GetReferrerStats(ctx, shortCode, startTime, endTime)
	if err != nil {
		return nil, fmt.Errorf("failed to get referrer stats: %w", err)
	}

	return &URLStatsReport{
		URLStats:      urlStats,
		TimeSeries:    timeSeries,
		CountryStats:  countryStats,
		DeviceStats:   deviceStats,
		BrowserStats:  browserStats,
		ReferrerStats: referrerStats,
	}, nil
}

// GetTopURLs retrieves the top performing URLs
func (s *AnalyticsServiceImpl) GetTopURLs(ctx context.Context, limit int, startTime, endTime time.Time, sortBy string) ([]*URLStats, error) {
	s.log.WithFields(logrus.Fields{
		"limit":      limit,
		"start_time": startTime,
		"end_time":   endTime,
		"sort_by":    sortBy,
	}).Info("Getting top URLs")

	return s.store.GetTopURLs(ctx, limit, startTime, endTime, sortBy)
}

// GetDashboard retrieves comprehensive dashboard metrics
func (s *AnalyticsServiceImpl) GetDashboard(ctx context.Context, startTime, endTime time.Time) (*DashboardMetrics, error) {
	s.log.WithFields(logrus.Fields{
		"start_time": startTime,
		"end_time":   endTime,
	}).Info("Getting dashboard metrics")

	return s.store.GetDashboardMetrics(ctx, startTime, endTime)
}

// Helper methods for data enrichment

func (s *AnalyticsServiceImpl) getCountryFromIP(ip string) string {
	// Simple country detection based on IP
	// In production, use a proper GeoIP service like MaxMind
	if strings.HasPrefix(ip, "192.168.") || strings.HasPrefix(ip, "10.") || strings.HasPrefix(ip, "172.") {
		return "Local"
	}

	// Parse IP to check if it's valid
	parsedIP := net.ParseIP(ip)
	if parsedIP == nil {
		return "Unknown"
	}

	// Simple mock geo-location - in production use proper GeoIP
	switch {
	case strings.Contains(ip, ".1."):
		return "US"
	case strings.Contains(ip, ".2."):
		return "CA"
	case strings.Contains(ip, ".3."):
		return "UK"
	case strings.Contains(ip, ".4."):
		return "DE"
	case strings.Contains(ip, ".5."):
		return "JP"
	default:
		return "Unknown"
	}
}

func (s *AnalyticsServiceImpl) getCityFromIP(ip string) string {
	// Simple city detection - in production use proper GeoIP service
	country := s.getCountryFromIP(ip)
	switch country {
	case "US":
		return "New York"
	case "CA":
		return "Toronto"
	case "UK":
		return "London"
	case "DE":
		return "Berlin"
	case "JP":
		return "Tokyo"
	default:
		return "Unknown"
	}
}

func (s *AnalyticsServiceImpl) getDeviceType(client *uaparser.Client) string {
	// Determine device type from user agent
	if client.Device.Family != "" && client.Device.Family != "Other" {
		return "Mobile"
	}

	userAgent := strings.ToLower(client.UserAgent.Family)
	if strings.Contains(userAgent, "mobile") || strings.Contains(userAgent, "android") || strings.Contains(userAgent, "iphone") {
		return "Mobile"
	}

	if strings.Contains(userAgent, "tablet") || strings.Contains(userAgent, "ipad") {
		return "Tablet"
	}

	return "Desktop"
}
