package tracing

import (
	"context"
	"fmt"
	"net/http"
	"os"

	"go-micro.dev/v5/server"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.17.0"
	oteltrace "go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// TracingConfig holds configuration for Jaeger tracing
type TracingConfig struct {
	ServiceName    string
	ServiceVersion string
	Environment    string
	JaegerEndpoint string
	SamplingRatio  float64
}

// DefaultTracingConfig returns default tracing configuration
func DefaultTracingConfig(serviceName string) *TracingConfig {
	return &TracingConfig{
		ServiceName:    serviceName,
		ServiceVersion: getEnv("SERVICE_VERSION", "1.0.0"),
		Environment:    getEnv("ENVIRONMENT", "development"),
		JaegerEndpoint: getEnv("JAEGER_ENDPOINT", "localhost:4317"),
		SamplingRatio:  1.0, // Sample 100% of traces in development
	}
}

// InitJaeger initializes Jaeger tracing for the service using OTLP gRPC exporter
func InitJaeger(config *TracingConfig) (*trace.TracerProvider, error) {
	// Create OTLP gRPC exporter for Jaeger
	ctx := context.Background()

	fmt.Printf("🔍 Initializing Jaeger tracing for service: %s\n", config.ServiceName)
	fmt.Printf("📡 Jaeger endpoint: %s\n", config.JaegerEndpoint)

	conn, err := grpc.NewClient(config.JaegerEndpoint,
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("failed to create gRPC connection to Jaeger at %s: %w", config.JaegerEndpoint, err)
	}

	exporter, err := otlptracegrpc.New(ctx, otlptracegrpc.WithGRPCConn(conn))
	if err != nil {
		return nil, fmt.Errorf("failed to create OTLP gRPC exporter: %w", err)
	}

	fmt.Printf("✅ Successfully connected to Jaeger at %s\n", config.JaegerEndpoint)

	// Create resource with service information
	res, err := resource.New(
		context.Background(),
		resource.WithAttributes(
			semconv.ServiceName(config.ServiceName),
			semconv.ServiceVersion(config.ServiceVersion),
			semconv.DeploymentEnvironment(config.Environment),
			attribute.String("service.type", "microservice"),
			attribute.String("service.framework", "go-micro"),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create resource: %w", err)
	}

	// Create tracer provider with AlwaysSample for testing
	tp := trace.NewTracerProvider(
		trace.WithBatcher(exporter),
		trace.WithResource(res),
		trace.WithSampler(trace.AlwaysSample()), // Always sample for testing
	)

	// Set global tracer provider
	otel.SetTracerProvider(tp)

	// Set global propagator for distributed tracing
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	return tp, nil
}

// Tracer wraps OpenTelemetry tracer with convenience methods
type Tracer struct {
	tracer oteltrace.Tracer
}

// NewTracer creates a new tracer instance
func NewTracer(serviceName string) *Tracer {
	return &Tracer{
		tracer: otel.Tracer(serviceName),
	}
}

// StartSpan starts a new span with the given name and options
func (t *Tracer) StartSpan(ctx context.Context, spanName string, opts ...oteltrace.SpanStartOption) (context.Context, oteltrace.Span) {
	return t.tracer.Start(ctx, spanName, opts...)
}

// StartHTTPSpan starts a span for HTTP requests
func (t *Tracer) StartHTTPSpan(ctx context.Context, method, endpoint string) (context.Context, oteltrace.Span) {
	spanName := fmt.Sprintf("HTTP %s %s", method, endpoint)
	ctx, span := t.tracer.Start(ctx, spanName)

	span.SetAttributes(
		attribute.String("http.method", method),
		attribute.String("http.route", endpoint),
		attribute.String("span.kind", "server"),
	)

	return ctx, span
}

// StartGRPCSpan starts a span for gRPC requests
func (t *Tracer) StartGRPCSpan(ctx context.Context, service, method string) (context.Context, oteltrace.Span) {
	spanName := fmt.Sprintf("gRPC %s/%s", service, method)
	ctx, span := t.tracer.Start(ctx, spanName)

	span.SetAttributes(
		attribute.String("rpc.system", "grpc"),
		attribute.String("rpc.service", service),
		attribute.String("rpc.method", method),
		attribute.String("span.kind", "server"),
	)

	return ctx, span
}

// StartDatabaseSpan starts a span for database operations
func (t *Tracer) StartDatabaseSpan(ctx context.Context, operation, table string) (context.Context, oteltrace.Span) {
	spanName := fmt.Sprintf("DB %s %s", operation, table)
	ctx, span := t.tracer.Start(ctx, spanName)

	span.SetAttributes(
		attribute.String("db.system", "postgresql"),
		attribute.String("db.operation", operation),
		attribute.String("db.sql.table", table),
		attribute.String("span.kind", "client"),
	)

	return ctx, span
}

// StartCacheSpan starts a span for cache operations
func (t *Tracer) StartCacheSpan(ctx context.Context, operation, key string) (context.Context, oteltrace.Span) {
	spanName := fmt.Sprintf("Cache %s", operation)
	ctx, span := t.tracer.Start(ctx, spanName)

	span.SetAttributes(
		attribute.String("cache.system", "redis"),
		attribute.String("cache.operation", operation),
		attribute.String("cache.key", key),
		attribute.String("span.kind", "client"),
	)

	return ctx, span
}

// StartNATSSpan starts a span for NATS messaging
func (t *Tracer) StartNATSSpan(ctx context.Context, operation, subject string) (context.Context, oteltrace.Span) {
	spanName := fmt.Sprintf("NATS %s %s", operation, subject)
	ctx, span := t.tracer.Start(ctx, spanName)

	span.SetAttributes(
		attribute.String("messaging.system", "nats"),
		attribute.String("messaging.operation", operation),
		attribute.String("messaging.destination", subject),
		attribute.String("span.kind", "producer"),
	)

	return ctx, span
}

// RecordError records an error in the span
func RecordError(span oteltrace.Span, err error) {
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
}

// RecordSuccess marks the span as successful
func RecordSuccess(span oteltrace.Span) {
	span.SetStatus(codes.Ok, "")
}

// AddAttributes adds attributes to the span
func AddAttributes(span oteltrace.Span, attrs ...attribute.KeyValue) {
	span.SetAttributes(attrs...)
}

// TraceHTTPMiddleware provides tracing for HTTP requests
func TraceHTTPMiddleware(serviceName string) func(next http.Handler) http.Handler {
	tracer := NewTracer(serviceName)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Extract trace context from incoming request
			ctx := otel.GetTextMapPropagator().Extract(r.Context(), propagation.HeaderCarrier(r.Header))

			ctx, span := tracer.StartHTTPSpan(ctx, r.Method, r.URL.Path)
			defer span.End()

			// Add request attributes
			span.SetAttributes(
				attribute.String("http.url", r.URL.String()),
				attribute.String("http.user_agent", r.UserAgent()),
				attribute.String("http.remote_addr", r.RemoteAddr),
				attribute.String("service.name", serviceName),
			)

			// Create response writer wrapper to capture status code
			wrapped := &responseWriter{ResponseWriter: w, statusCode: 200}

			// Process request
			next.ServeHTTP(wrapped, r.WithContext(ctx))

			// Record response attributes
			span.SetAttributes(
				attribute.Int("http.status_code", wrapped.statusCode),
			)

			// Set span status based on HTTP status code
			if wrapped.statusCode >= 400 {
				span.SetStatus(codes.Error, fmt.Sprintf("HTTP %d", wrapped.statusCode))
			} else {
				span.SetStatus(codes.Ok, "")
			}
		})
	}
}

// responseWriter wraps http.ResponseWriter to capture status code
type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

// TraceGoMicroMiddleware provides tracing for Go Micro gRPC requests
func TraceGoMicroMiddleware(serviceName string) server.HandlerWrapper {
	tracer := NewTracer(serviceName)

	return func(fn server.HandlerFunc) server.HandlerFunc {
		return func(ctx context.Context, req server.Request, rsp interface{}) error {
			ctx, span := tracer.StartGRPCSpan(ctx, req.Service(), req.Method())
			defer span.End()

			// Add request attributes
			span.SetAttributes(
				attribute.String("rpc.request.endpoint", req.Endpoint()),
				attribute.String("rpc.request.content_type", req.ContentType()),
			)

			// Process request
			err := fn(ctx, req, rsp)

			// Record result
			if err != nil {
				RecordError(span, err)
			} else {
				RecordSuccess(span)
			}

			return err
		}
	}
}

// Business Logic Tracing Helpers

// TraceURLShortening traces URL shortening operations
func TraceURLShortening(ctx context.Context, tracer *Tracer, longURL, userID string) (context.Context, oteltrace.Span) {
	ctx, span := tracer.StartSpan(ctx, "url.shorten")
	span.SetAttributes(
		attribute.String("url.long", longURL),
		attribute.String("user.id", userID),
		attribute.String("operation", "shorten"),
	)
	return ctx, span
}

// TraceURLRedirection traces URL redirection operations
func TraceURLRedirection(ctx context.Context, tracer *Tracer, shortCode string) (context.Context, oteltrace.Span) {
	ctx, span := tracer.StartSpan(ctx, "url.redirect")
	span.SetAttributes(
		attribute.String("url.short_code", shortCode),
		attribute.String("operation", "redirect"),
	)
	return ctx, span
}

// TraceAnalyticsEvent traces analytics event processing
func TraceAnalyticsEvent(ctx context.Context, tracer *Tracer, eventType, shortCode string) (context.Context, oteltrace.Span) {
	ctx, span := tracer.StartSpan(ctx, "analytics.process")
	span.SetAttributes(
		attribute.String("analytics.event_type", eventType),
		attribute.String("url.short_code", shortCode),
		attribute.String("operation", "analytics"),
	)
	return ctx, span
}

// Utility functions

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
