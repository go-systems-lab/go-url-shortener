package microservice

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	_ "github.com/lib/pq"
	natsBroker "github.com/micro/plugins/v5/broker/nats"
	natsRegistry "github.com/micro/plugins/v5/registry/nats"
	natsTransport "github.com/micro/plugins/v5/transport/nats"
	"github.com/redis/go-redis/v9"
	"github.com/sirupsen/logrus"
	"go-micro.dev/v5"
	"go-micro.dev/v5/broker"

	"encoding/base64"

	pb "github.com/go-systems-lab/go-url-shortener/proto/analytics"
	"github.com/go-systems-lab/go-url-shortener/services/analytics-svc/domain"
	"github.com/go-systems-lab/go-url-shortener/services/analytics-svc/handler"
	"github.com/go-systems-lab/go-url-shortener/services/analytics-svc/store"
	"github.com/go-systems-lab/go-url-shortener/utils/tracing"
)

// Microservice represents the analytics microservice
type Microservice struct {
	service          micro.Service
	log              *logrus.Logger
	analyticsService domain.AnalyticsService
}

// ClientOptions contains configuration for the analytics microservice
type ClientOptions struct {
	Version string
	Log     *logrus.Logger
}

// Init initializes the analytics microservice with ClickHouse and all dependencies
func Init(opts *ClientOptions) (*Microservice, error) {
	if opts.Log == nil {
		opts.Log = logrus.New()
		opts.Log.SetLevel(logrus.InfoLevel)
	}

	logger := opts.Log
	logger.Info("Initializing Analytics Microservice with ClickHouse and NATS...")

	// Initialize tracing
	tracingConfig := tracing.DefaultTracingConfig("url.shortener.analytics")
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

	// Initialize ClickHouse connection (as specified in HLD)
	clickhouseConn, err := initializeClickHouse(logger)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize ClickHouse: %w", err)
	}

	// Initialize Redis connection
	redisClient, err := initializeRedis(logger)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize Redis: %w", err)
	}

	// Create ClickHouse analytics store (following HLD architecture)
	analyticsStore := store.NewClickHouseStore(clickhouseConn, redisClient, logger)

	// Create analytics service (business logic)
	analyticsService := domain.NewAnalyticsService(analyticsStore, logger)

	// Create analytics handler
	analyticsHandler := handler.NewAnalyticsHandler(analyticsService, logger)

	// Create Go Micro service with NATS plugins (production-ready configuration)
	service := micro.NewService(
		micro.Name("url.shortener.analytics"),
		micro.Version(opts.Version),
		micro.Transport(natsTransport.NewTransport()), // NATS Transport
		micro.Registry(natsRegistry.NewRegistry()),    // NATS Registry
		micro.Broker(natsBroker.NewBroker()),          // NATS Broker
		// Add distributed tracing middleware
		micro.WrapHandler(tracing.TraceGoMicroMiddleware("analytics.service")),
		// Add metadata for service discovery
		micro.Metadata(map[string]string{
			"type":     "analytics",
			"database": "clickhouse",
			"cache":    "redis",
			"protocol": "grpc",
		}),
	)

	// Register analytics service handler
	if err := pb.RegisterAnalyticsServiceHandler(service.Server(), analyticsHandler); err != nil {
		return nil, fmt.Errorf("failed to register analytics handler: %w", err)
	}

	// Note: NATS subscription will be done after service initialization in Run()

	logger.Info("Analytics microservice initialized successfully with ClickHouse")

	return &Microservice{
		service:          service,
		log:              logger,
		analyticsService: analyticsService,
	}, nil
}

// Run starts the analytics microservice
func (m *Microservice) Run() error {
	m.log.Info("Starting Analytics Microservice...")

	// Initialize service (this will start the server and connect broker)
	m.service.Init()

	// Subscribe to click events in a goroutine after service starts
	go func() {
		// Wait a moment for the service to fully start
		time.Sleep(2 * time.Second)

		if err := subscribeToClickEvents(m.service, m.analyticsService, m.log); err != nil {
			m.log.WithError(err).Error("Failed to subscribe to click events")
		}
	}()

	// Run the service
	if err := m.service.Run(); err != nil {
		m.log.WithError(err).Fatal("Failed to run analytics service")
		return err
	}

	return nil
}

// Stop gracefully stops the analytics microservice
func (m *Microservice) Stop() error {
	m.log.Info("Stopping Analytics Microservice...")
	// Note: go-micro v5 handles graceful shutdown internally via signal handling
	return nil
}

// initializeClickHouse creates and configures ClickHouse connection
func initializeClickHouse(log *logrus.Logger) (clickhouse.Conn, error) {
	// Get ClickHouse configuration from environment variables
	clickhouseUser := os.Getenv("CLICKHOUSE_USER")
	if clickhouseUser == "" {
		clickhouseUser = "default"
	}

	clickhousePassword := os.Getenv("CLICKHOUSE_PASSWORD")
	if clickhousePassword == "" {
		clickhousePassword = ""
	}

	clickhouseDatabase := os.Getenv("CLICKHOUSE_DATABASE")
	if clickhouseDatabase == "" {
		clickhouseDatabase = "analytics"
	}

	clickhouseHost := os.Getenv("CLICKHOUSE_HOST")
	if clickhouseHost == "" {
		clickhouseHost = "localhost:9001"
	}

	log.WithFields(logrus.Fields{
		"host":     clickhouseHost,
		"database": clickhouseDatabase,
		"user":     clickhouseUser,
	}).Info("Connecting to ClickHouse...")

	// Create ClickHouse connection with production settings
	conn, err := clickhouse.Open(&clickhouse.Options{
		Addr: []string{clickhouseHost},
		Auth: clickhouse.Auth{
			Database: clickhouseDatabase,
			Username: clickhouseUser,
			Password: clickhousePassword,
		},
		Settings: clickhouse.Settings{
			"max_execution_time": 60,
		},
		DialTimeout:      time.Second * 30,
		MaxOpenConns:     10,
		MaxIdleConns:     5,
		ConnMaxLifetime:  time.Hour,
		ConnOpenStrategy: clickhouse.ConnOpenInOrder,
	})

	if err != nil {
		return nil, fmt.Errorf("failed to connect to ClickHouse: %w", err)
	}

	// Test the connection
	if err := conn.Ping(context.Background()); err != nil {
		return nil, fmt.Errorf("failed to ping ClickHouse: %w", err)
	}

	// Create analytics table and indexes
	if err := createClickHouseSchema(conn, log); err != nil {
		return nil, fmt.Errorf("failed to create ClickHouse schema: %w", err)
	}

	log.Info("ClickHouse connection established successfully")
	return conn, nil
}

// initializeRedis creates and configures Redis connection
func initializeRedis(log *logrus.Logger) (*redis.Client, error) {
	redisURL := os.Getenv("REDIS_URL")
	if redisURL == "" {
		redisURL = "redis://localhost:6379/0"
	}

	log.WithField("url", redisURL).Info("Connecting to Redis...")

	// Parse Redis URL
	opt, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse Redis URL: %w", err)
	}

	// Create Redis client with production settings
	client := redis.NewClient(&redis.Options{
		Addr:         opt.Addr,
		Password:     opt.Password,
		DB:           opt.DB,
		PoolSize:     10,
		MinIdleConns: 5,
		MaxRetries:   3,
		DialTimeout:  time.Second * 5,
		ReadTimeout:  time.Second * 3,
		WriteTimeout: time.Second * 3,
	})

	// Test the connection
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}

	log.Info("Redis connection established successfully")
	return client, nil
}

// createClickHouseSchema creates the analytics table optimized for time-series data
func createClickHouseSchema(conn clickhouse.Conn, log *logrus.Logger) error {
	// ClickHouse schema optimized for analytics (as specified in HLD)
	createTableQuery := `
	CREATE TABLE IF NOT EXISTS click_analytics (
		short_code String,
		long_url String,
		client_ip String,
		user_agent String,
		referrer String,
		country String,
		city String,
		device_type String,
		browser String,
		os String,
		timestamp DateTime64(3),
		session_id String,
		is_unique UInt8,
		created_at DateTime64(3) DEFAULT now()
	) ENGINE = MergeTree()
	PARTITION BY toYYYYMM(timestamp)
	ORDER BY (short_code, timestamp)
	SETTINGS index_granularity = 8192`

	log.Info("Creating ClickHouse analytics table with time-series optimization...")

	if err := conn.Exec(context.Background(), createTableQuery); err != nil {
		return fmt.Errorf("failed to create analytics table: %w", err)
	}

	// Create materialized views for real-time aggregations (production optimization)
	createAggregateViews := `
	CREATE MATERIALIZED VIEW IF NOT EXISTS click_analytics_hourly_mv
	ENGINE = SummingMergeTree()
	PARTITION BY toYYYYMM(hour)
	ORDER BY (short_code, hour)
	AS SELECT
		short_code,
		toStartOfHour(timestamp) as hour,
		count() as total_clicks,
		uniq(session_id) as unique_visitors
	FROM click_analytics
	GROUP BY short_code, hour;

	CREATE MATERIALIZED VIEW IF NOT EXISTS click_analytics_daily_mv
	ENGINE = SummingMergeTree()
	PARTITION BY toYYYYMM(day)
	ORDER BY (short_code, day)
	AS SELECT
		short_code,
		toStartOfDay(timestamp) as day,
		count() as total_clicks,
		uniq(session_id) as unique_visitors,
		uniq(country) as countries_count
	FROM click_analytics
	GROUP BY short_code, day`

	if err := conn.Exec(context.Background(), createAggregateViews); err != nil {
		log.WithError(err).Warn("Failed to create materialized views (optional optimization)")
	}

	log.Info("ClickHouse schema created successfully with analytics optimizations")
	return nil
}

// subscribeToClickEvents subscribes to click events from NATS for real-time processing
func subscribeToClickEvents(service micro.Service, analyticsService domain.AnalyticsService, log *logrus.Logger) error {
	log.Info("Subscribing to click events from NATS...")

	// Subscribe to url.clicked events using Go Micro broker
	_, err := service.Options().Broker.Subscribe("url.clicked", func(event broker.Event) error {
		log.WithFields(logrus.Fields{
			"topic": event.Topic(),
		}).Debug("Received click event")

		// Get message body
		messageBody := event.Message().Body

		// Check if the message is base64 encoded (Go Micro broker behavior)
		var jsonData []byte
		if len(messageBody) > 0 && messageBody[0] == '"' {
			// Message is JSON-encoded string, need to unmarshal and then base64 decode
			var encodedString string
			if err := json.Unmarshal(messageBody, &encodedString); err != nil {
				log.WithError(err).WithField("body", string(messageBody)).Error("Failed to unmarshal encoded string")
				return nil
			}

			// Decode base64
			decoded, err := base64.StdEncoding.DecodeString(encodedString)
			if err != nil {
				log.WithError(err).WithField("encoded", encodedString).Error("Failed to decode base64")
				return nil
			}
			jsonData = decoded
		} else {
			// Message is already JSON
			jsonData = messageBody
		}

		// Parse click event from JSON
		var clickData map[string]interface{}
		if err := json.Unmarshal(jsonData, &clickData); err != nil {
			log.WithError(err).WithField("body", string(jsonData)).Error("Failed to parse click event JSON")
			return nil // Don't return error to avoid reprocessing
		}

		// Create domain click event with safe type conversions
		clickEvent := &domain.ClickEvent{
			ShortCode: getStringFromMap(clickData, "short_code"),
			LongURL:   getStringFromMap(clickData, "long_url"),
			ClientIP:  getStringFromMap(clickData, "client_ip"),
			UserAgent: getStringFromMap(clickData, "user_agent"),
			Referrer:  getStringFromMap(clickData, "referrer"),
			SessionID: getStringFromMap(clickData, "session_id"),
			Timestamp: time.Now(),
		}

		// Extract timestamp if provided
		if timestampVal, exists := clickData["timestamp"]; exists {
			if timestamp, ok := timestampVal.(float64); ok {
				clickEvent.Timestamp = time.Unix(int64(timestamp), 0)
			}
		}

		log.WithFields(logrus.Fields{
			"short_code": clickEvent.ShortCode,
			"long_url":   clickEvent.LongURL,
			"client_ip":  clickEvent.ClientIP,
			"session_id": clickEvent.SessionID,
		}).Info("Processing click event from NATS")

		// Process the click event
		if err := analyticsService.ProcessClick(context.Background(), clickEvent); err != nil {
			log.WithError(err).WithFields(logrus.Fields{
				"short_code": clickEvent.ShortCode,
				"client_ip":  clickEvent.ClientIP,
			}).Error("Failed to process click event")
		} else {
			log.WithField("short_code", clickEvent.ShortCode).Info("✅ Click event processed successfully and stored in ClickHouse")
		}

		return nil
	})

	if err != nil {
		return fmt.Errorf("failed to subscribe to click events: %w", err)
	}

	log.Info("Successfully subscribed to click events")
	return nil
}

// Helper function to safely extract string values from map
func getStringFromMap(data map[string]interface{}, key string) string {
	if val, exists := data[key]; exists {
		if str, ok := val.(string); ok {
			return str
		}
	}
	return ""
}
