package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/redis/go-redis/v9"
	"github.com/sirupsen/logrus"

	"github.com/go-systems-lab/go-url-shortener/services/analytics-svc/domain"
)

// AnalyticsStoreImpl implements the AnalyticsStore interface
type AnalyticsStoreImpl struct {
	db    *sqlx.DB
	redis *redis.Client
	log   *logrus.Logger
}

// NewAnalyticsStore creates a new analytics store instance
func NewAnalyticsStore(db *sqlx.DB, redis *redis.Client, log *logrus.Logger) domain.AnalyticsStore {
	return &AnalyticsStoreImpl{
		db:    db,
		redis: redis,
		log:   log,
	}
}

// SaveClick saves a click record to the database
func (s *AnalyticsStoreImpl) SaveClick(ctx context.Context, click *domain.ClickRecord) error {
	query := `
		INSERT INTO click_analytics (
			short_code, long_url, client_ip, user_agent, referrer,
			country, city, device_type, browser, os,
			timestamp, session_id, is_unique, created_at
		) VALUES (
			:short_code, :long_url, :client_ip, :user_agent, :referrer,
			:country, :city, :device_type, :browser, :os,
			:timestamp, :session_id, :is_unique, :created_at
		)`

	_, err := s.db.NamedExecContext(ctx, query, click)
	if err != nil {
		s.log.WithError(err).Error("Failed to save click record")
		return fmt.Errorf("failed to save click: %w", err)
	}

	// Update cached aggregates asynchronously
	go s.updateCachedStats(click.ShortCode)

	return nil
}

// GetURLStats retrieves basic statistics for a URL
func (s *AnalyticsStoreImpl) GetURLStats(ctx context.Context, shortCode string, startTime, endTime time.Time) (*domain.URLStats, error) {
	query := `
		SELECT 
			short_code,
			COUNT(*) as total_clicks,
			COUNT(DISTINCT CASE WHEN is_unique = true THEN session_id END) as unique_clicks,
			MAX(timestamp) as last_clicked,
			MIN(created_at) as created_at
		FROM click_analytics 
		WHERE short_code = $1 
			AND timestamp BETWEEN $2 AND $3
		GROUP BY short_code`

	var stats domain.URLStats
	err := s.db.GetContext(ctx, &stats, query, shortCode, startTime, endTime)
	if err != nil {
		if err == sql.ErrNoRows {
			return &domain.URLStats{
				ShortCode:    shortCode,
				TotalClicks:  0,
				UniqueClicks: 0,
				LastClicked:  time.Time{},
				CreatedAt:    time.Time{},
			}, nil
		}
		return nil, fmt.Errorf("failed to get URL stats: %w", err)
	}

	return &stats, nil
}

// GetTimeSeriesData retrieves time-based analytics data
func (s *AnalyticsStoreImpl) GetTimeSeriesData(ctx context.Context, shortCode string, startTime, endTime time.Time, granularity string) ([]*domain.TimeSeriesData, error) {
	var intervalClause string
	switch granularity {
	case "hour":
		intervalClause = "DATE_TRUNC('hour', timestamp)"
	case "day":
		intervalClause = "DATE_TRUNC('day', timestamp)"
	case "week":
		intervalClause = "DATE_TRUNC('week', timestamp)"
	case "month":
		intervalClause = "DATE_TRUNC('month', timestamp)"
	default:
		intervalClause = "DATE_TRUNC('hour', timestamp)"
	}

	query := fmt.Sprintf(`
		SELECT 
			%s as timestamp,
			COUNT(*) as clicks,
			COUNT(DISTINCT CASE WHEN is_unique = true THEN session_id END) as unique_clicks
		FROM click_analytics 
		WHERE short_code = $1 
			AND timestamp BETWEEN $2 AND $3
		GROUP BY %s
		ORDER BY timestamp`, intervalClause, intervalClause)

	var timeSeries []*domain.TimeSeriesData
	err := s.db.SelectContext(ctx, &timeSeries, query, shortCode, startTime, endTime)
	if err != nil {
		return nil, fmt.Errorf("failed to get time series data: %w", err)
	}

	return timeSeries, nil
}

// GetCountryStats retrieves click statistics by country
func (s *AnalyticsStoreImpl) GetCountryStats(ctx context.Context, shortCode string, startTime, endTime time.Time) ([]*domain.CountryMetrics, error) {
	query := `
		WITH country_counts AS (
			SELECT 
				country,
				COUNT(*) as clicks
			FROM click_analytics 
			WHERE short_code = $1 
				AND timestamp BETWEEN $2 AND $3
			GROUP BY country
		),
		total_clicks AS (
			SELECT SUM(clicks) as total FROM country_counts
		)
		SELECT 
			cc.country,
			cc.clicks,
			CASE 
				WHEN tc.total > 0 THEN (cc.clicks::FLOAT / tc.total * 100)::FLOAT
				ELSE 0::FLOAT
			END as percentage
		FROM country_counts cc
		CROSS JOIN total_clicks tc
		ORDER BY cc.clicks DESC
		LIMIT 10`

	var countryStats []*domain.CountryMetrics
	err := s.db.SelectContext(ctx, &countryStats, query, shortCode, startTime, endTime)
	if err != nil {
		return nil, fmt.Errorf("failed to get country stats: %w", err)
	}

	return countryStats, nil
}

// GetDeviceStats retrieves click statistics by device type
func (s *AnalyticsStoreImpl) GetDeviceStats(ctx context.Context, shortCode string, startTime, endTime time.Time) ([]*domain.DeviceMetrics, error) {
	query := `
		WITH device_counts AS (
			SELECT 
				device_type,
				COUNT(*) as clicks
			FROM click_analytics 
			WHERE short_code = $1 
				AND timestamp BETWEEN $2 AND $3
			GROUP BY device_type
		),
		total_clicks AS (
			SELECT SUM(clicks) as total FROM device_counts
		)
		SELECT 
			dc.device_type,
			dc.clicks,
			CASE 
				WHEN tc.total > 0 THEN (dc.clicks::FLOAT / tc.total * 100)::FLOAT
				ELSE 0::FLOAT
			END as percentage
		FROM device_counts dc
		CROSS JOIN total_clicks tc
		ORDER BY dc.clicks DESC`

	var deviceStats []*domain.DeviceMetrics
	err := s.db.SelectContext(ctx, &deviceStats, query, shortCode, startTime, endTime)
	if err != nil {
		return nil, fmt.Errorf("failed to get device stats: %w", err)
	}

	return deviceStats, nil
}

// GetBrowserStats retrieves click statistics by browser
func (s *AnalyticsStoreImpl) GetBrowserStats(ctx context.Context, shortCode string, startTime, endTime time.Time) ([]*domain.BrowserMetrics, error) {
	query := `
		WITH browser_counts AS (
			SELECT 
				browser,
				COUNT(*) as clicks
			FROM click_analytics 
			WHERE short_code = $1 
				AND timestamp BETWEEN $2 AND $3
			GROUP BY browser
		),
		total_clicks AS (
			SELECT SUM(clicks) as total FROM browser_counts
		)
		SELECT 
			bc.browser,
			bc.clicks,
			CASE 
				WHEN tc.total > 0 THEN (bc.clicks::FLOAT / tc.total * 100)::FLOAT
				ELSE 0::FLOAT
			END as percentage
		FROM browser_counts bc
		CROSS JOIN total_clicks tc
		ORDER BY bc.clicks DESC
		LIMIT 10`

	var browserStats []*domain.BrowserMetrics
	err := s.db.SelectContext(ctx, &browserStats, query, shortCode, startTime, endTime)
	if err != nil {
		return nil, fmt.Errorf("failed to get browser stats: %w", err)
	}

	return browserStats, nil
}

// GetReferrerStats retrieves click statistics by referrer
func (s *AnalyticsStoreImpl) GetReferrerStats(ctx context.Context, shortCode string, startTime, endTime time.Time) ([]*domain.ReferrerMetrics, error) {
	query := `
		WITH referrer_counts AS (
			SELECT 
				CASE 
					WHEN referrer = '' OR referrer IS NULL THEN 'Direct'
					ELSE referrer
				END as referrer,
				COUNT(*) as clicks
			FROM click_analytics 
			WHERE short_code = $1 
				AND timestamp BETWEEN $2 AND $3
			GROUP BY referrer
		),
		total_clicks AS (
			SELECT SUM(clicks) as total FROM referrer_counts
		)
		SELECT 
			rc.referrer,
			rc.clicks,
			CASE 
				WHEN tc.total > 0 THEN (rc.clicks::FLOAT / tc.total * 100)::FLOAT
				ELSE 0::FLOAT
			END as percentage
		FROM referrer_counts rc
		CROSS JOIN total_clicks tc
		ORDER BY rc.clicks DESC
		LIMIT 10`

	var referrerStats []*domain.ReferrerMetrics
	err := s.db.SelectContext(ctx, &referrerStats, query, shortCode, startTime, endTime)
	if err != nil {
		return nil, fmt.Errorf("failed to get referrer stats: %w", err)
	}

	return referrerStats, nil
}

// GetTopURLs retrieves the top performing URLs
func (s *AnalyticsStoreImpl) GetTopURLs(ctx context.Context, limit int, startTime, endTime time.Time, sortBy string) ([]*domain.URLStats, error) {
	var orderClause string
	switch strings.ToLower(sortBy) {
	case "unique_clicks":
		orderClause = "unique_clicks DESC"
	case "created_at":
		orderClause = "created_at DESC"
	default:
		orderClause = "total_clicks DESC"
	}

	query := fmt.Sprintf(`
		SELECT 
			short_code,
			COUNT(*) as total_clicks,
			COUNT(DISTINCT CASE WHEN is_unique = true THEN session_id END) as unique_clicks,
			MAX(timestamp) as last_clicked,
			MIN(created_at) as created_at
		FROM click_analytics 
		WHERE timestamp BETWEEN $1 AND $2
		GROUP BY short_code
		ORDER BY %s
		LIMIT $3`, orderClause)

	var topURLs []*domain.URLStats
	err := s.db.SelectContext(ctx, &topURLs, query, startTime, endTime, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get top URLs: %w", err)
	}

	return topURLs, nil
}

// GetDashboardMetrics retrieves comprehensive dashboard metrics
func (s *AnalyticsStoreImpl) GetDashboardMetrics(ctx context.Context, startTime, endTime time.Time) (*domain.DashboardMetrics, error) {
	// Get basic metrics
	basicQuery := `
		SELECT 
			COUNT(DISTINCT short_code) as total_urls,
			COUNT(*) as total_clicks,
			COUNT(DISTINCT CASE WHEN is_unique = true THEN session_id END) as unique_clicks,
			COUNT(DISTINCT CASE WHEN timestamp >= NOW() - INTERVAL '7 days' THEN short_code END) as active_urls
		FROM click_analytics 
		WHERE timestamp BETWEEN $1 AND $2`

	var metrics domain.DashboardMetrics
	err := s.db.GetContext(ctx, &metrics, basicQuery, startTime, endTime)
	if err != nil {
		return nil, fmt.Errorf("failed to get basic dashboard metrics: %w", err)
	}

	// Get click timeline (daily aggregation)
	timelineQuery := `
		SELECT 
			DATE_TRUNC('day', timestamp) as timestamp,
			COUNT(*) as clicks,
			COUNT(DISTINCT CASE WHEN is_unique = true THEN session_id END) as unique_clicks
		FROM click_analytics 
		WHERE timestamp BETWEEN $1 AND $2
		GROUP BY DATE_TRUNC('day', timestamp)
		ORDER BY timestamp`

	err = s.db.SelectContext(ctx, &metrics.ClickTimeline, timelineQuery, startTime, endTime)
	if err != nil {
		return nil, fmt.Errorf("failed to get click timeline: %w", err)
	}

	// Get top countries
	countryQuery := `
		WITH country_counts AS (
			SELECT 
				country,
				COUNT(*) as clicks
			FROM click_analytics 
			WHERE timestamp BETWEEN $1 AND $2
			GROUP BY country
		),
		total_clicks AS (
			SELECT SUM(clicks) as total FROM country_counts
		)
		SELECT 
			cc.country,
			cc.clicks,
			CASE 
				WHEN tc.total > 0 THEN (cc.clicks::FLOAT / tc.total * 100)::FLOAT
				ELSE 0::FLOAT
			END as percentage
		FROM country_counts cc
		CROSS JOIN total_clicks tc
		ORDER BY cc.clicks DESC
		LIMIT 5`

	err = s.db.SelectContext(ctx, &metrics.TopCountries, countryQuery, startTime, endTime)
	if err != nil {
		return nil, fmt.Errorf("failed to get top countries: %w", err)
	}

	// Get device breakdown
	deviceQuery := `
		WITH device_counts AS (
			SELECT 
				device_type,
				COUNT(*) as clicks
			FROM click_analytics 
			WHERE timestamp BETWEEN $1 AND $2
			GROUP BY device_type
		),
		total_clicks AS (
			SELECT SUM(clicks) as total FROM device_counts
		)
		SELECT 
			dc.device_type,
			dc.clicks,
			CASE 
				WHEN tc.total > 0 THEN (dc.clicks::FLOAT / tc.total * 100)::FLOAT
				ELSE 0::FLOAT
			END as percentage
		FROM device_counts dc
		CROSS JOIN total_clicks tc
		ORDER BY dc.clicks DESC`

	err = s.db.SelectContext(ctx, &metrics.DeviceBreakdown, deviceQuery, startTime, endTime)
	if err != nil {
		return nil, fmt.Errorf("failed to get device breakdown: %w", err)
	}

	return &metrics, nil
}

// IsUniqueVisitor checks if this is a unique visitor for the URL
func (s *AnalyticsStoreImpl) IsUniqueVisitor(ctx context.Context, shortCode, sessionID string) (bool, error) {
	// Check Redis cache first for recent sessions
	cacheKey := fmt.Sprintf("session:%s:%s", shortCode, sessionID)
	exists, err := s.redis.Exists(ctx, cacheKey).Result()
	if err == nil && exists > 0 {
		return false, nil // Not unique, session exists in cache
	}

	// Check database for session existence
	query := `
		SELECT EXISTS(
			SELECT 1 FROM click_analytics 
			WHERE short_code = $1 AND session_id = $2
		)`

	var exists_db bool
	err = s.db.GetContext(ctx, &exists_db, query, shortCode, sessionID)
	if err != nil {
		s.log.WithError(err).Error("Failed to check unique visitor")
		return false, err
	}

	// If unique, cache the session for 24 hours
	if !exists_db {
		s.redis.Set(ctx, cacheKey, "1", 24*time.Hour)
		return true, nil
	}

	return false, nil
}

// updateCachedStats updates cached statistics (async)
func (s *AnalyticsStoreImpl) updateCachedStats(shortCode string) {
	ctx := context.Background()

	// Update cached total clicks count
	cacheKey := fmt.Sprintf("stats:%s:total_clicks", shortCode)
	s.redis.Incr(ctx, cacheKey)
	s.redis.Expire(ctx, cacheKey, time.Hour)

	// Log successful cache update
	s.log.WithField("short_code", shortCode).Debug("Updated cached stats")
}
