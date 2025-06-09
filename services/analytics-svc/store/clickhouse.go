package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/redis/go-redis/v9"
	"github.com/sirupsen/logrus"

	"github.com/go-systems-lab/go-url-shortener/services/analytics-svc/domain"
)

// ClickHouseStoreImpl implements the AnalyticsStore interface using ClickHouse
type ClickHouseStoreImpl struct {
	db    clickhouse.Conn
	redis *redis.Client
	log   *logrus.Logger
}

// NewClickHouseStore creates a new ClickHouse analytics store instance
func NewClickHouseStore(db clickhouse.Conn, redis *redis.Client, log *logrus.Logger) domain.AnalyticsStore {
	return &ClickHouseStoreImpl{
		db:    db,
		redis: redis,
		log:   log,
	}
}

// SaveClick saves a click record to ClickHouse (optimized for time-series data)
func (s *ClickHouseStoreImpl) SaveClick(ctx context.Context, click *domain.ClickRecord) error {
	query := `
		INSERT INTO click_analytics (
			short_code, long_url, client_ip, user_agent, referrer,
			country, city, device_type, browser, os,
			timestamp, session_id, is_unique, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	err := s.db.Exec(ctx, query,
		click.ShortCode,
		click.LongURL,
		click.ClientIP,
		click.UserAgent,
		click.Referrer,
		click.Country,
		click.City,
		click.DeviceType,
		click.Browser,
		click.OS,
		click.Timestamp,
		click.SessionID,
		click.IsUnique,
		click.CreatedAt,
	)

	if err != nil {
		s.log.WithError(err).Error("Failed to save click record to ClickHouse")
		return fmt.Errorf("failed to save click: %w", err)
	}

	// Update cached aggregates asynchronously
	go s.updateCachedStats(click.ShortCode)

	return nil
}

// GetURLStats retrieves analytics statistics for a specific URL using ClickHouse aggregations
func (s *ClickHouseStoreImpl) GetURLStats(ctx context.Context, shortCode string, startTime, endTime time.Time) (*domain.URLStats, error) {
	query := `
		SELECT 
			short_code,
			toInt64(count()) as total_clicks,
			toInt64(uniq(session_id)) as unique_clicks,
			max(timestamp) as last_clicked,
			min(created_at) as created_at
		FROM click_analytics 
		WHERE short_code = ? 
			AND timestamp BETWEEN ? AND ?
		GROUP BY short_code`

	var stats domain.URLStats
	row := s.db.QueryRow(ctx, query, shortCode, startTime, endTime)

	err := row.Scan(
		&stats.ShortCode,
		&stats.TotalClicks,
		&stats.UniqueClicks,
		&stats.LastClicked,
		&stats.CreatedAt,
	)

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
		return nil, fmt.Errorf("failed to get URL stats from ClickHouse: %w", err)
	}

	return &stats, nil
}

// GetTimeSeriesData retrieves time-based analytics data using ClickHouse time functions
func (s *ClickHouseStoreImpl) GetTimeSeriesData(ctx context.Context, shortCode string, startTime, endTime time.Time, granularity string) ([]*domain.TimeSeriesData, error) {
	var intervalFunc string
	switch granularity {
	case "hour":
		intervalFunc = "toStartOfHour(timestamp)"
	case "day":
		intervalFunc = "toStartOfDay(timestamp)"
	case "week":
		intervalFunc = "toStartOfWeek(timestamp)"
	case "month":
		intervalFunc = "toStartOfMonth(timestamp)"
	default:
		intervalFunc = "toStartOfHour(timestamp)"
	}

	query := fmt.Sprintf(`
		SELECT 
			%s as timestamp,
			toInt64(count()) as clicks,
			toInt64(uniq(session_id)) as unique_clicks
		FROM click_analytics 
		WHERE short_code = ? 
			AND timestamp BETWEEN ? AND ?
		GROUP BY timestamp
		ORDER BY timestamp`, intervalFunc)

	rows, err := s.db.Query(ctx, query, shortCode, startTime, endTime)
	if err != nil {
		return nil, fmt.Errorf("failed to get time series data from ClickHouse: %w", err)
	}
	defer rows.Close()

	var timeSeries []*domain.TimeSeriesData
	for rows.Next() {
		var ts domain.TimeSeriesData
		if err := rows.Scan(&ts.Timestamp, &ts.Clicks, &ts.UniqueClicks); err != nil {
			return nil, fmt.Errorf("failed to scan time series row: %w", err)
		}
		timeSeries = append(timeSeries, &ts)
	}

	return timeSeries, nil
}

// GetCountryStats retrieves click statistics by country using ClickHouse aggregations
func (s *ClickHouseStoreImpl) GetCountryStats(ctx context.Context, shortCode string, startTime, endTime time.Time) ([]*domain.CountryMetrics, error) {
	query := `
		SELECT 
			country,
			toInt64(count()) as clicks,
			toFloat64(count() * 100.0 / sum(count()) OVER ()) as percentage
		FROM click_analytics 
		WHERE short_code = ? 
			AND timestamp BETWEEN ? AND ?
			AND country != ''
		GROUP BY country
		ORDER BY clicks DESC
		LIMIT 10`

	rows, err := s.db.Query(ctx, query, shortCode, startTime, endTime)
	if err != nil {
		return nil, fmt.Errorf("failed to get country stats from ClickHouse: %w", err)
	}
	defer rows.Close()

	var countryStats []*domain.CountryMetrics
	for rows.Next() {
		var cs domain.CountryMetrics
		if err := rows.Scan(&cs.Country, &cs.Clicks, &cs.Percentage); err != nil {
			return nil, fmt.Errorf("failed to scan country stats row: %w", err)
		}
		countryStats = append(countryStats, &cs)
	}

	return countryStats, nil
}

// GetDeviceStats retrieves click statistics by device type
func (s *ClickHouseStoreImpl) GetDeviceStats(ctx context.Context, shortCode string, startTime, endTime time.Time) ([]*domain.DeviceMetrics, error) {
	query := `
		SELECT 
			device_type,
			toInt64(count()) as clicks,
			toFloat64(count() * 100.0 / sum(count()) OVER ()) as percentage
		FROM click_analytics 
		WHERE short_code = ? 
			AND timestamp BETWEEN ? AND ?
			AND device_type != ''
		GROUP BY device_type
		ORDER BY clicks DESC`

	rows, err := s.db.Query(ctx, query, shortCode, startTime, endTime)
	if err != nil {
		return nil, fmt.Errorf("failed to get device stats from ClickHouse: %w", err)
	}
	defer rows.Close()

	var deviceStats []*domain.DeviceMetrics
	for rows.Next() {
		var ds domain.DeviceMetrics
		if err := rows.Scan(&ds.DeviceType, &ds.Clicks, &ds.Percentage); err != nil {
			return nil, fmt.Errorf("failed to scan device stats row: %w", err)
		}
		deviceStats = append(deviceStats, &ds)
	}

	return deviceStats, nil
}

// GetBrowserStats retrieves click statistics by browser
func (s *ClickHouseStoreImpl) GetBrowserStats(ctx context.Context, shortCode string, startTime, endTime time.Time) ([]*domain.BrowserMetrics, error) {
	query := `
		SELECT 
			browser,
			toInt64(count()) as clicks,
			toFloat64(count() * 100.0 / sum(count()) OVER ()) as percentage
		FROM click_analytics 
		WHERE short_code = ? 
			AND timestamp BETWEEN ? AND ?
			AND browser != ''
		GROUP BY browser
		ORDER BY clicks DESC
		LIMIT 10`

	rows, err := s.db.Query(ctx, query, shortCode, startTime, endTime)
	if err != nil {
		return nil, fmt.Errorf("failed to get browser stats from ClickHouse: %w", err)
	}
	defer rows.Close()

	var browserStats []*domain.BrowserMetrics
	for rows.Next() {
		var bs domain.BrowserMetrics
		if err := rows.Scan(&bs.Browser, &bs.Clicks, &bs.Percentage); err != nil {
			return nil, fmt.Errorf("failed to scan browser stats row: %w", err)
		}
		browserStats = append(browserStats, &bs)
	}

	return browserStats, nil
}

// GetReferrerStats retrieves click statistics by referrer
func (s *ClickHouseStoreImpl) GetReferrerStats(ctx context.Context, shortCode string, startTime, endTime time.Time) ([]*domain.ReferrerMetrics, error) {
	query := `
		SELECT 
			CASE 
				WHEN referrer = '' OR referrer IS NULL THEN 'Direct'
				ELSE referrer
			END as referrer,
			toInt64(count()) as clicks,
			toFloat64(count() * 100.0 / sum(count()) OVER ()) as percentage
		FROM click_analytics 
		WHERE short_code = ? 
			AND timestamp BETWEEN ? AND ?
		GROUP BY referrer
		ORDER BY clicks DESC
		LIMIT 10`

	rows, err := s.db.Query(ctx, query, shortCode, startTime, endTime)
	if err != nil {
		return nil, fmt.Errorf("failed to get referrer stats from ClickHouse: %w", err)
	}
	defer rows.Close()

	var referrerStats []*domain.ReferrerMetrics
	for rows.Next() {
		var rs domain.ReferrerMetrics
		if err := rows.Scan(&rs.Referrer, &rs.Clicks, &rs.Percentage); err != nil {
			return nil, fmt.Errorf("failed to scan referrer stats row: %w", err)
		}
		referrerStats = append(referrerStats, &rs)
	}

	return referrerStats, nil
}

// GetTopURLs retrieves the top performing URLs using ClickHouse aggregations
func (s *ClickHouseStoreImpl) GetTopURLs(ctx context.Context, limit int, startTime, endTime time.Time, sortBy string) ([]*domain.URLStats, error) {
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
			toInt64(count()) as total_clicks,
			toInt64(uniq(session_id)) as unique_clicks,
			max(timestamp) as last_clicked,
			min(created_at) as created_at
		FROM click_analytics 
		WHERE timestamp BETWEEN ? AND ?
		GROUP BY short_code
		ORDER BY %s
		LIMIT ?`, orderClause)

	rows, err := s.db.Query(ctx, query, startTime, endTime, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get top URLs from ClickHouse: %w", err)
	}
	defer rows.Close()

	var topURLs []*domain.URLStats
	for rows.Next() {
		var stats domain.URLStats
		if err := rows.Scan(&stats.ShortCode, &stats.TotalClicks, &stats.UniqueClicks, &stats.LastClicked, &stats.CreatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan top URLs row: %w", err)
		}
		topURLs = append(topURLs, &stats)
	}

	return topURLs, nil
}

// GetDashboardMetrics retrieves comprehensive dashboard metrics
func (s *ClickHouseStoreImpl) GetDashboardMetrics(ctx context.Context, startTime, endTime time.Time) (*domain.DashboardMetrics, error) {
	// Get basic metrics using ClickHouse aggregations
	basicQuery := `
		SELECT 
			toInt64(uniq(short_code)) as total_urls,
			toInt64(count()) as total_clicks,
			toInt64(uniq(session_id)) as unique_clicks,
			toInt64(uniqIf(short_code, timestamp >= now() - INTERVAL 7 DAY)) as active_urls
		FROM click_analytics 
		WHERE timestamp BETWEEN ? AND ?`

	var metrics domain.DashboardMetrics
	row := s.db.QueryRow(ctx, basicQuery, startTime, endTime)
	err := row.Scan(&metrics.TotalURLs, &metrics.TotalClicks, &metrics.UniqueClicks, &metrics.ActiveURLs)
	if err != nil {
		return nil, fmt.Errorf("failed to get basic dashboard metrics from ClickHouse: %w", err)
	}

	// Get click timeline (daily aggregation)
	timelineQuery := `
		SELECT 
			toStartOfDay(timestamp) as timestamp,
			toInt64(count()) as clicks,
			toInt64(uniq(session_id)) as unique_clicks
		FROM click_analytics 
		WHERE timestamp BETWEEN ? AND ?
		GROUP BY timestamp
		ORDER BY timestamp`

	rows, err := s.db.Query(ctx, timelineQuery, startTime, endTime)
	if err != nil {
		return nil, fmt.Errorf("failed to get click timeline from ClickHouse: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var ts domain.TimeSeriesData
		if err := rows.Scan(&ts.Timestamp, &ts.Clicks, &ts.UniqueClicks); err != nil {
			return nil, fmt.Errorf("failed to scan timeline row: %w", err)
		}
		metrics.ClickTimeline = append(metrics.ClickTimeline, ts)
	}

	// Get top countries (reuse existing method)
	topCountries, err := s.GetCountryStats(ctx, "", startTime, endTime) // Empty shortCode for all URLs
	if err != nil {
		return nil, fmt.Errorf("failed to get top countries: %w", err)
	}
	if len(topCountries) > 5 {
		topCountries = topCountries[:5] // Limit to top 5
	}
	for _, country := range topCountries {
		metrics.TopCountries = append(metrics.TopCountries, *country)
	}

	// Get device breakdown
	deviceBreakdown, err := s.GetDeviceStats(ctx, "", startTime, endTime) // Empty shortCode for all URLs
	if err != nil {
		return nil, fmt.Errorf("failed to get device breakdown: %w", err)
	}
	for _, device := range deviceBreakdown {
		metrics.DeviceBreakdown = append(metrics.DeviceBreakdown, *device)
	}

	return &metrics, nil
}

// IsUniqueVisitor checks if this is a unique visitor for the URL
func (s *ClickHouseStoreImpl) IsUniqueVisitor(ctx context.Context, shortCode, sessionID string) (bool, error) {
	// Check Redis cache first for recent sessions
	cacheKey := fmt.Sprintf("session:%s:%s", shortCode, sessionID)
	exists, err := s.redis.Exists(ctx, cacheKey).Result()
	if err == nil && exists > 0 {
		return false, nil // Not unique, session exists in cache
	}

	// Check ClickHouse for session existence
	query := `
		SELECT count() > 0
		FROM click_analytics 
		WHERE short_code = ? AND session_id = ?
		LIMIT 1`

	var existsInDB bool
	row := s.db.QueryRow(ctx, query, shortCode, sessionID)
	err = row.Scan(&existsInDB)
	if err != nil {
		s.log.WithError(err).Error("Failed to check unique visitor in ClickHouse")
		return false, err
	}

	// If unique, cache the session for 24 hours
	if !existsInDB {
		s.redis.Set(ctx, cacheKey, "1", 24*time.Hour)
		return true, nil
	}

	return false, nil
}

// updateCachedStats updates cached statistics (async)
func (s *ClickHouseStoreImpl) updateCachedStats(shortCode string) {
	ctx := context.Background()

	// Update cached total clicks count
	cacheKey := fmt.Sprintf("stats:%s:total_clicks", shortCode)
	s.redis.Incr(ctx, cacheKey)
	s.redis.Expire(ctx, cacheKey, time.Hour)

	// Log successful cache update
	s.log.WithField("short_code", shortCode).Debug("Updated cached stats")
}
