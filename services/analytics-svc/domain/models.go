package domain

import (
	"time"
)

// ClickRecord represents a single click event in our domain
type ClickRecord struct {
	ID         int64     `json:"id" db:"id"`
	ShortCode  string    `json:"short_code" db:"short_code"`
	LongURL    string    `json:"long_url" db:"long_url"`
	ClientIP   string    `json:"client_ip" db:"client_ip"`
	UserAgent  string    `json:"user_agent" db:"user_agent"`
	Referrer   string    `json:"referrer" db:"referrer"`
	Country    string    `json:"country" db:"country"`
	City       string    `json:"city" db:"city"`
	DeviceType string    `json:"device_type" db:"device_type"`
	Browser    string    `json:"browser" db:"browser"`
	OS         string    `json:"os" db:"os"`
	Timestamp  time.Time `json:"timestamp" db:"timestamp"`
	SessionID  string    `json:"session_id" db:"session_id"`
	IsUnique   bool      `json:"is_unique" db:"is_unique"`
	CreatedAt  time.Time `json:"created_at" db:"created_at"`
}

// URLStats represents aggregated statistics for a URL
type URLStats struct {
	ShortCode    string    `json:"short_code" db:"short_code"`
	TotalClicks  int64     `json:"total_clicks" db:"total_clicks"`
	UniqueClicks int64     `json:"unique_clicks" db:"unique_clicks"`
	LastClicked  time.Time `json:"last_clicked" db:"last_clicked"`
	CreatedAt    time.Time `json:"created_at" db:"created_at"`
}

// CountryMetrics represents click metrics by country
type CountryMetrics struct {
	Country    string  `json:"country" db:"country"`
	Clicks     int64   `json:"clicks" db:"clicks"`
	Percentage float64 `json:"percentage" db:"percentage"`
}

// DeviceMetrics represents click metrics by device type
type DeviceMetrics struct {
	DeviceType string  `json:"device_type" db:"device_type"`
	Clicks     int64   `json:"clicks" db:"clicks"`
	Percentage float64 `json:"percentage" db:"percentage"`
}

// BrowserMetrics represents click metrics by browser
type BrowserMetrics struct {
	Browser    string  `json:"browser" db:"browser"`
	Clicks     int64   `json:"clicks" db:"clicks"`
	Percentage float64 `json:"percentage" db:"percentage"`
}

// ReferrerMetrics represents click metrics by referrer
type ReferrerMetrics struct {
	Referrer   string  `json:"referrer" db:"referrer"`
	Clicks     int64   `json:"clicks" db:"clicks"`
	Percentage float64 `json:"percentage" db:"percentage"`
}

// TimeSeriesData represents time-based analytics data
type TimeSeriesData struct {
	Timestamp    time.Time `json:"timestamp" db:"timestamp"`
	Clicks       int64     `json:"clicks" db:"clicks"`
	UniqueClicks int64     `json:"unique_clicks" db:"unique_clicks"`
}

// DashboardMetrics represents overall system metrics
type DashboardMetrics struct {
	TotalURLs       int64            `json:"total_urls" db:"total_urls"`
	TotalClicks     int64            `json:"total_clicks" db:"total_clicks"`
	UniqueClicks    int64            `json:"unique_clicks" db:"unique_clicks"`
	ActiveURLs      int64            `json:"active_urls" db:"active_urls"`
	ClickTimeline   []TimeSeriesData `json:"click_timeline"`
	TopCountries    []CountryMetrics `json:"top_countries"`
	DeviceBreakdown []DeviceMetrics  `json:"device_breakdown"`
}
