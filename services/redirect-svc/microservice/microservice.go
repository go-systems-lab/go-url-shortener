package microservice

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/nats-io/nats.go"
	"github.com/redis/go-redis/v9"
	"github.com/sirupsen/logrus"
	"go-micro.dev/v5"

	// Import NATS plugins
	natsBroker "github.com/micro/plugins/v5/broker/nats"
	natsRegistry "github.com/micro/plugins/v5/registry/nats"
	natsTransport "github.com/micro/plugins/v5/transport/nats"

	pb "github.com/go-systems-lab/go-url-shortener/proto/redirect"
	"github.com/go-systems-lab/go-url-shortener/services/redirect-svc/domain"
	"github.com/go-systems-lab/go-url-shortener/services/redirect-svc/handler"
	"github.com/go-systems-lab/go-url-shortener/services/redirect-svc/store"
	"github.com/go-systems-lab/go-url-shortener/utils/tracing"
)

// ClientOptions defines options for the redirect microservice
type ClientOptions struct {
	Version string
	Log     *logrus.Logger
}

// Microservice represents the redirect microservice
type Microservice struct {
	service micro.Service
	log     *logrus.Logger
}

// Init initializes the microservice with NATS plugins
func Init(opts *ClientOptions) (*Microservice, error) {
	// Initialize tracing
	tracingConfig := tracing.DefaultTracingConfig("redirect.service")
	tp, err := tracing.InitJaeger(tracingConfig)
	if err != nil {
		opts.Log.WithError(err).Warn("Failed to initialize Jaeger tracing, continuing without tracing")
	} else {
		opts.Log.Info("Jaeger tracing initialized successfully")
		defer func() {
			if err := tp.Shutdown(context.Background()); err != nil {
				opts.Log.WithError(err).Error("Failed to shutdown tracer provider")
			}
		}()
	}

	// Initialize dependencies
	db, err := initializePostgreSQL(opts.Log)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize PostgreSQL: %w", err)
	}

	redisClient, err := initializeRedis(opts.Log)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize Redis: %w", err)
	}

	natsConn, err := initializeNATS(opts.Log)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize NATS: %w", err)
	}

	// Create service layers
	redirectStore := store.NewRedirectStore(db, redisClient)
	redirectService := domain.NewRedirectService(redirectStore)

	// Create Go Micro service with NATS plugins
	service := micro.NewService(
		micro.Name("redirect.service"),
		micro.Version(opts.Version),
		micro.Transport(natsTransport.NewTransport()),
		micro.Registry(natsRegistry.NewRegistry()),
		micro.Broker(natsBroker.NewBroker()),
		micro.WrapHandler(tracing.TraceGoMicroMiddleware("redirect.service")),
	)

	// Initialize service
	service.Init()

	// Create handler with service for broker access
	redirectHandler := handler.NewRedirectHandler(redirectService, natsConn, service)

	// Register handler using the generated registration function
	err = pb.RegisterRedirectServiceHandler(service.Server(), redirectHandler)
	if err != nil {
		return nil, fmt.Errorf("failed to register redirect handler: %w", err)
	}

	opts.Log.Info("Redirect RPC service configured with NATS transport, registry, and broker")

	return &Microservice{
		service: service,
		log:     opts.Log,
	}, nil
}

// Run starts the microservice
func (m *Microservice) Run() error {
	m.log.Info("Starting Redirect microservice with NATS...")
	return m.service.Run()
}

// initializePostgreSQL connects to PostgreSQL database
func initializePostgreSQL(log *logrus.Logger) (*sqlx.DB, error) {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		databaseURL = "postgres://postgres:password@localhost:5432/url_shortener_db?sslmode=disable"
	}

	log.Info("🔌 Connecting to PostgreSQL...")
	db, err := sqlx.Connect("pgx", databaseURL)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	// Configure connection pool
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)

	// Test connection
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	log.Info("✅ Connected to PostgreSQL")
	return db, nil
}

// initializeRedis connects to Redis cache
func initializeRedis(log *logrus.Logger) (*redis.Client, error) {
	redisURL := os.Getenv("REDIS_URL")
	if redisURL == "" {
		redisURL = "redis://localhost:6379/0"
	}

	log.Info("🔌 Connecting to Redis...")

	// Parse Redis URL
	opt, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse Redis URL: %w", err)
	}

	// Production-optimized configuration
	opt.MaxRetries = 3
	opt.MinRetryBackoff = 8 * time.Millisecond
	opt.MaxRetryBackoff = 512 * time.Millisecond
	opt.PoolSize = 30
	opt.MinIdleConns = 10
	opt.PoolTimeout = 30 * time.Second

	client := redis.NewClient(opt)

	// Test connection
	if err := client.Ping(context.Background()).Err(); err != nil {
		client.Close()
		return nil, fmt.Errorf("failed to ping Redis: %w", err)
	}

	log.Info("✅ Connected to Redis")
	return client, nil
}

// initializeNATS connects to NATS server
func initializeNATS(log *logrus.Logger) (*nats.Conn, error) {
	natsURL := os.Getenv("NATS_URL")
	if natsURL == "" {
		natsURL = "nats://localhost:4222"
	}

	log.Infof("🔌 Connecting to NATS at %s...", natsURL)

	// NATS connection options
	opts := []nats.Option{
		nats.Name("redirect-service"),
		nats.MaxReconnects(-1), // Unlimited reconnects
		nats.ReconnectWait(2 * time.Second),
		nats.DisconnectErrHandler(func(nc *nats.Conn, err error) {
			log.Warnf("⚠️ NATS disconnected: %v", err)
		}),
		nats.ReconnectHandler(func(nc *nats.Conn) {
			log.Infof("✅ NATS reconnected: %s", nc.ConnectedUrl())
		}),
	}

	conn, err := nats.Connect(natsURL, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to NATS: %w", err)
	}

	log.Info("✅ Connected to NATS")
	return conn, nil
}
