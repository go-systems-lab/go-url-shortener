package main

import (
	"context"
	"fmt"
	"os"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/sirupsen/logrus"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
	"go-micro.dev/v5"
	"go-micro.dev/v5/registry"
	"go-micro.dev/v5/transport"

	// Import NATS plugins
	natsBroker "github.com/micro/plugins/v5/broker/nats"
	natsRegistry "github.com/micro/plugins/v5/registry/nats"
	natsTransport "github.com/micro/plugins/v5/transport/nats"

	_ "github.com/go-systems-lab/go-url-shortener/services/rest-api-svc/docs" // Import generated docs
	"github.com/go-systems-lab/go-url-shortener/services/rest-api-svc/handler"
	"github.com/go-systems-lab/go-url-shortener/utils/metrics"
	"github.com/go-systems-lab/go-url-shortener/utils/tracing"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
)

// Version may be changed during build via --ldflags parameter
var Version = "latest"

// HealthResponse represents the health check response
type HealthResponse struct {
	Status    string `json:"status" example:"ok"`
	Service   string `json:"service" example:"rest-api-svc"`
	Transport string `json:"transport" example:"NATS"`
	Version   string `json:"version" example:"1.0"`
}

// @title			URL Shortener REST API
// @version		1.0
// @description	A production-ready URL shortener microservice built with Go Micro v5, NATS, PostgreSQL, and Redis.
// @description	This REST API provides HTTP endpoints for end users to interact with the URL shortening service.
//
// @contact.name	URL Shortener API Support
// @contact.email	support@urlshortener.com
//
// @license.name	MIT
// @license.url	https://opensource.org/licenses/MIT
//
// @host		localhost:8082
// @BasePath	/api/v1
func main() {
	logger := logrus.New()
	logger.SetFormatter(&logrus.JSONFormatter{})
	logger.Info("Starting REST API Service with NATS, Swagger documentation, and observability...")

	// Initialize tracing
	tracingConfig := tracing.DefaultTracingConfig("rest.api.service")
	tp, err := tracing.InitJaeger(tracingConfig)
	if err != nil {
		logger.WithError(err).Warn("Failed to initialize Jaeger tracing, continuing without tracing")
	} else {
		logger.Info("Jaeger tracing initialized successfully")
		defer func() {
			if err := tp.Shutdown(context.Background()); err != nil {
				logger.WithError(err).Error("Failed to shutdown tracer provider")
			}
		}()
	}

	// Initialize metrics
	metricsRegistry := metrics.NewMetrics()

	// Get port from environment variable, default to 8080 if not set
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	// Create Go Micro service for client with NATS plugins
	natsURL := os.Getenv("NATS_URL")
	if natsURL == "" {
		natsURL = "nats://localhost:4222"
	}

	service := micro.NewService(
		micro.Name("rest.api.service"),
		micro.Version(Version),
		micro.Transport(natsTransport.NewTransport(transport.Addrs(natsURL))),
		micro.Registry(natsRegistry.NewRegistry(registry.Addrs(natsURL))),
		micro.Broker(natsBroker.NewBroker()),
	)

	// Initialize service
	service.Init()

	// Start the Go Micro service in a goroutine to enable service discovery
	go func() {
		if err := service.Run(); err != nil {
			logger.WithError(err).Error("Failed to run Go Micro service")
		}
	}()

	logger.Info("REST API service configured with NATS transport, registry, and broker")

	// Create REST API handler
	urlHandler := handler.NewURLHandler(service)

	// Setup Gin router
	router := gin.Default()

	// Add observability middleware
	router.Use(metrics.GinMiddleware("rest-api"))

	// Add distributed tracing middleware
	if tp != nil {
		logger.Info("🔍 Registering distributed tracing middleware")
		router.Use(func(c *gin.Context) {
			// Extract trace context from incoming request headers
			ctx := otel.GetTextMapPropagator().Extract(c.Request.Context(), propagation.HeaderCarrier(c.Request.Header))

			// Create tracer and start span
			tracer := tracing.NewTracer("rest.api.service")
			ctx, span := tracer.StartHTTPSpan(ctx, c.Request.Method, c.Request.URL.Path)
			defer span.End()

			// Add HTTP attributes
			tracing.AddAttributes(span,
				attribute.String("http.url", c.Request.URL.String()),
				attribute.String("http.user_agent", c.Request.UserAgent()),
				attribute.String("http.remote_addr", c.ClientIP()),
				attribute.String("service.name", "rest.api.service"),
			)

			// Set context for downstream services
			c.Request = c.Request.WithContext(ctx)

			c.Next()

			// Record response status
			statusCode := c.Writer.Status()
			tracing.AddAttributes(span, attribute.Int("http.status_code", statusCode))

			if statusCode >= 400 {
				tracing.RecordError(span, fmt.Errorf("HTTP %d", statusCode))
			} else {
				tracing.RecordSuccess(span)
			}
		})
	} else {
		logger.Warn("🔍 Tracing provider is nil, skipping tracing middleware")
	}

	// Add CORS middleware
	router.Use(func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	})

	// Metrics endpoint
	router.GET("/metrics", gin.WrapH(promhttp.HandlerFor(metricsRegistry.Registry, promhttp.HandlerOpts{})))

	// API Documentation routes
	router.GET("/docs/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))
	router.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler)) // Alternative path

	// Documentation landing page
	router.GET("/", func(c *gin.Context) {
		c.Header("Content-Type", "text/html")
		c.String(200, `
<!DOCTYPE html>
<html>
<head>
    <title>URL Shortener API</title>
    <meta charset="utf-8">
    <style>
        body { font-family: Arial, sans-serif; margin: 40px; background: #f5f5f5; }
        .container { background: white; padding: 30px; border-radius: 8px; box-shadow: 0 2px 10px rgba(0,0,0,0.1); }
        h1 { color: #2c3e50; }
        .endpoint { background: #ecf0f1; padding: 15px; margin: 10px 0; border-radius: 5px; }
        .method { font-weight: bold; padding: 4px 8px; border-radius: 3px; color: white; margin-right: 10px; }
        .get { background: #27ae60; }
        .post { background: #e74c3c; }
        .delete { background: #e67e22; }
        a { color: #3498db; text-decoration: none; }
        a:hover { text-decoration: underline; }
    </style>
</head>
<body>
    <div class="container">
        <h1>🚀 URL Shortener REST API</h1>
        <p>Welcome to the URL Shortener microservice REST API. This service provides HTTP endpoints for creating and managing short URLs.</p>
        
        <h2>📖 API Documentation</h2>
        <p><strong><a href="/docs/index.html">📋 Swagger UI Documentation</a></strong> - Interactive API documentation with try-it-out functionality</p>
        <p><strong><a href="/swagger/doc.json">📄 OpenAPI JSON Specification</a></strong> - Machine-readable API specification</p>
        
        <h2>🏗️ Architecture</h2>
        <p>This REST API service communicates with an internal Go Micro RPC service via NATS transport for business logic processing.</p>
        
        <h2>📱 Available Endpoints</h2>
        <div class="endpoint">
            <span class="method post">POST</span> <strong>/api/v1/shorten</strong> - Create a short URL
        </div>
        <div class="endpoint">
            <span class="method get">GET</span> <strong>/api/v1/urls/{shortCode}</strong> - Get URL information
        </div>
        <div class="endpoint">
            <span class="method delete">DELETE</span> <strong>/api/v1/urls/{shortCode}</strong> - Delete a URL
        </div>
        <div class="endpoint">
            <span class="method get">GET</span> <strong>/api/v1/users/{userID}/urls</strong> - List user URLs
        </div>
        <div class="endpoint">
            <span class="method get">GET</span> <strong>/api/v1/analytics/urls/{shortCode}</strong> - Get URL analytics
        </div>
        <div class="endpoint">
            <span class="method get">GET</span> <strong>/api/v1/analytics/top-urls</strong> - Get top performing URLs
        </div>
        <div class="endpoint">
            <span class="method get">GET</span> <strong>/api/v1/analytics/dashboard</strong> - Get analytics dashboard
        </div>
        <div class="endpoint">
            <span class="method get">GET</span> <strong>/health</strong> - Service health check
        </div>
        
        <h2>🧪 Quick Test</h2>
        <p>Try the health check: <a href="/health">/health</a></p>
        
        <p><em>For comprehensive API testing and examples, visit the <a href="/docs/index.html">Swagger UI</a>.</em></p>
    </div>
</body>
</html>`)
	})

	// Setup API routes
	api := router.Group("/api/v1")
	{
		// URL Management endpoints
		api.POST("/shorten", urlHandler.ShortenURL)
		api.GET("/urls/:shortCode", urlHandler.GetURLInfo)
		api.DELETE("/urls/:shortCode", urlHandler.DeleteURL)
		api.GET("/users/:userID/urls", urlHandler.GetUserURLs)

		// Analytics endpoints
		api.GET("/analytics/urls/:shortCode", urlHandler.GetURLStats)
		api.GET("/analytics/top-urls", urlHandler.GetTopURLs)
		api.GET("/analytics/dashboard", urlHandler.GetDashboard)
	}

	// Add redirect route (must be after API routes to avoid conflicts)
	// This handles GET /:shortCode for actual URL redirection
	router.GET("/:shortCode", urlHandler.RedirectURL)

	// Health check with Swagger annotation
	//
	//	@Summary		Health check
	//	@Description	Check the health and status of the REST API service
	//	@Tags			Health
	//	@Accept			json
	//	@Produce		json
	//	@Success		200	{object}	HealthResponse	"Service is healthy"
	//	@Router			/health [get]
	router.GET("/health", func(c *gin.Context) {
		c.JSON(200, HealthResponse{
			Status:    "ok",
			Service:   "rest-api-svc",
			Transport: "NATS",
			Version:   Version,
		})
	})

	logger.WithFields(logrus.Fields{
		"port":    port,
		"swagger": "http://localhost:" + port + "/docs/index.html",
		"docs":    "http://localhost:" + port + "/",
	}).Info("REST API Service with Swagger documentation ready")

	if err := router.Run(":" + port); err != nil {
		logger.WithError(err).Fatal("Failed to start REST API service")
	}
}
