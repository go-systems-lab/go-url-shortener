package metrics

import (
	"context"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go-micro.dev/v5/server"
)

// Prometheus metrics for URL shortener system
var (
	// HTTP Request metrics
	HTTPRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "Total number of HTTP requests",
		},
		[]string{"service", "method", "endpoint", "status"},
	)

	HTTPRequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "http_request_duration_seconds",
			Help:    "Duration of HTTP requests in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"service", "method", "endpoint", "status"},
	)

	// gRPC Request metrics
	GRPCRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "grpc_requests_total",
			Help: "Total number of gRPC requests",
		},
		[]string{"service", "method", "status"},
	)

	GRPCRequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "grpc_request_duration_seconds",
			Help:    "Duration of gRPC requests in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"service", "method", "status"},
	)

	// Business Logic Metrics
	URLsCreatedTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "urls_created_total",
			Help: "Total number of URLs created",
		},
		[]string{"service", "user_type"},
	)

	RedirectionsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "redirections_total",
			Help: "Total number of URL redirections",
		},
		[]string{"service", "country", "device_type"},
	)

	RedirectRequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "redirect_request_duration_seconds",
			Help:    "Duration of redirect requests in seconds",
			Buckets: []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10},
		},
		[]string{"service", "cache_hit"},
	)

	URLClickCount = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "url_click_count",
			Help: "Click count per URL",
		},
		[]string{"short_code", "country"},
	)

	// Cache Metrics
	RedisCacheHitRatio = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "redis_cache_hit_ratio",
			Help: "Redis cache hit ratio",
		},
		[]string{"service"},
	)

	CacheOperationsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "cache_operations_total",
			Help: "Total cache operations",
		},
		[]string{"service", "operation", "result"},
	)

	// Database Metrics
	DatabaseConnectionsActive = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "postgres_active_connections",
			Help: "Number of active PostgreSQL connections",
		},
		[]string{"database"},
	)

	DatabaseOperationsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "database_operations_total",
			Help: "Total database operations",
		},
		[]string{"service", "operation", "table"},
	)

	DatabaseOperationDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "database_operation_duration_seconds",
			Help:    "Duration of database operations",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"service", "operation", "table"},
	)

	// NATS Metrics
	NATSMessagesPublished = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "nats_messages_published_total",
			Help: "Total NATS messages published",
		},
		[]string{"service", "subject"},
	)

	NATSMessagesReceived = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "nats_messages_received_total",
			Help: "Total NATS messages received",
		},
		[]string{"service", "subject"},
	)

	// Analytics Metrics
	AnalyticsEventsProcessed = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "analytics_events_processed_total",
			Help: "Total analytics events processed",
		},
		[]string{"service", "event_type"},
	)

	RedirectionsByCountry = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "redirections_by_country",
			Help: "Redirections grouped by country",
		},
		[]string{"country"},
	)
)

// Metrics contains all prometheus metrics
type Metrics struct {
	Registry *prometheus.Registry
}

// NewMetrics creates a new metrics instance
func NewMetrics() *Metrics {
	registry := prometheus.NewRegistry()

	// Register Go runtime metrics (CRITICAL for dashboard panels!)
	registry.MustRegister(collectors.NewGoCollector())
	registry.MustRegister(collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))

	// Register all custom metrics
	registry.MustRegister(
		HTTPRequestsTotal,
		HTTPRequestDuration,
		GRPCRequestsTotal,
		GRPCRequestDuration,
		URLsCreatedTotal,
		RedirectionsTotal,
		RedirectRequestDuration,
		URLClickCount,
		RedisCacheHitRatio,
		CacheOperationsTotal,
		DatabaseConnectionsActive,
		DatabaseOperationsTotal,
		DatabaseOperationDuration,
		NATSMessagesPublished,
		NATSMessagesReceived,
		AnalyticsEventsProcessed,
		RedirectionsByCountry,
	)

	return &Metrics{
		Registry: registry,
	}
}

// PrometheusHandler returns the HTTP handler for metrics endpoint
func (m *Metrics) PrometheusHandler() gin.HandlerFunc {
	h := promhttp.HandlerFor(m.Registry, promhttp.HandlerOpts{})
	return gin.WrapH(h)
}

// GinMiddleware provides Prometheus metrics for Gin HTTP endpoints
func GinMiddleware(serviceName string) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()

		c.Next()

		duration := time.Since(start)
		status := strconv.Itoa(c.Writer.Status())

		HTTPRequestsTotal.WithLabelValues(
			serviceName,
			c.Request.Method,
			c.FullPath(),
			status,
		).Inc()

		HTTPRequestDuration.WithLabelValues(
			serviceName,
			c.Request.Method,
			c.FullPath(),
			status,
		).Observe(duration.Seconds())
	}
}

// GoMicroMiddleware provides Prometheus metrics for Go Micro gRPC endpoints
func GoMicroMiddleware(serviceName string) server.HandlerWrapper {
	return func(fn server.HandlerFunc) server.HandlerFunc {
		return func(ctx context.Context, req server.Request, rsp interface{}) error {
			start := time.Now()

			err := fn(ctx, req, rsp)

			duration := time.Since(start)
			status := "success"
			if err != nil {
				status = "error"
			}

			GRPCRequestsTotal.WithLabelValues(
				serviceName,
				req.Method(),
				status,
			).Inc()

			GRPCRequestDuration.WithLabelValues(
				serviceName,
				req.Method(),
				status,
			).Observe(duration.Seconds())

			return err
		}
	}
}

// Business Logic Metric Helpers

// RecordURLCreated records a URL creation event
func RecordURLCreated(service, userType string) {
	URLsCreatedTotal.WithLabelValues(service, userType).Inc()
}

// RecordRedirection records a URL redirection event
func RecordRedirection(service, country, deviceType string, cacheHit bool, duration time.Duration) {
	RedirectionsTotal.WithLabelValues(service, country, deviceType).Inc()

	cacheHitStr := "false"
	if cacheHit {
		cacheHitStr = "true"
	}

	RedirectRequestDuration.WithLabelValues(service, cacheHitStr).Observe(duration.Seconds())
	RedirectionsByCountry.WithLabelValues(country).Inc()
}

// RecordURLClick records a click on a specific URL
func RecordURLClick(shortCode, country string) {
	URLClickCount.WithLabelValues(shortCode, country).Inc()
}

// RecordCacheOperation records cache operations
func RecordCacheOperation(service, operation, result string) {
	CacheOperationsTotal.WithLabelValues(service, operation, result).Inc()
}

// UpdateCacheHitRatio updates cache hit ratio
func UpdateCacheHitRatio(service string, ratio float64) {
	RedisCacheHitRatio.WithLabelValues(service).Set(ratio)
}

// RecordDatabaseOperation records database operations
func RecordDatabaseOperation(service, operation, table string, duration time.Duration) {
	DatabaseOperationsTotal.WithLabelValues(service, operation, table).Inc()
	DatabaseOperationDuration.WithLabelValues(service, operation, table).Observe(duration.Seconds())
}

// UpdateDatabaseConnections updates active database connections
func UpdateDatabaseConnections(database string, count float64) {
	DatabaseConnectionsActive.WithLabelValues(database).Set(count)
}

// RecordNATSMessage records NATS message events
func RecordNATSMessagePublished(service, subject string) {
	NATSMessagesPublished.WithLabelValues(service, subject).Inc()
}

func RecordNATSMessageReceived(service, subject string) {
	NATSMessagesReceived.WithLabelValues(service, subject).Inc()
}

// RecordAnalyticsEvent records analytics event processing
func RecordAnalyticsEvent(service, eventType string) {
	AnalyticsEventsProcessed.WithLabelValues(service, eventType).Inc()
}

// Helper functions for instrumentation

// MeasureTime measures execution time and calls the provided function with duration
func MeasureTime(fn func(duration time.Duration)) func() {
	start := time.Now()
	return func() {
		fn(time.Since(start))
	}
}

// WrapDatabaseOperation wraps database operations with metrics
func WrapDatabaseOperation(service, operation, table string, fn func() error) error {
	defer MeasureTime(func(duration time.Duration) {
		RecordDatabaseOperation(service, operation, table, duration)
	})()

	return fn()
}

// WrapCacheOperation wraps cache operations with metrics
func WrapCacheOperation(service, operation string, fn func() (bool, error)) (bool, error) {
	hit, err := fn()

	result := "success"
	if err != nil {
		result = "error"
	}

	RecordCacheOperation(service, operation, result)
	return hit, err
}
