package microservice

import (
	"context"
	"net/http"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/sirupsen/logrus"
	"go-micro.dev/v5"

	// Import NATS plugins
	natsBroker "github.com/micro/plugins/v5/broker/nats"
	natsRegistry "github.com/micro/plugins/v5/registry/nats"
	natsTransport "github.com/micro/plugins/v5/transport/nats"

	pb "github.com/go-systems-lab/go-url-shortener/proto/url"
	"github.com/go-systems-lab/go-url-shortener/services/url-shortener-svc/handler"
	"github.com/go-systems-lab/go-url-shortener/utils/cache"
	"github.com/go-systems-lab/go-url-shortener/utils/database"
	"github.com/go-systems-lab/go-url-shortener/utils/metrics"
	"github.com/go-systems-lab/go-url-shortener/utils/tracing"
)

// ClientOptions defines options for the microservice
type ClientOptions struct {
	Version string
	Log     *logrus.Logger
}

// Microservice represents the URL shortener microservice
type Microservice struct {
	service micro.Service
	log     *logrus.Logger
	tracer  *tracing.Tracer
}

// Init initializes the microservice with NATS plugins and observability
func Init(opts *ClientOptions) (*Microservice, error) {
	// Initialize tracing
	tracingConfig := tracing.DefaultTracingConfig("url.shortener.service")
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

	tracer := tracing.NewTracer("url.shortener.service")

	// Initialize metrics
	metricsRegistry := metrics.NewMetrics()

	// Initialize dependencies
	db := database.NewPostgreSQL()
	redisCache := cache.NewRedis()

	// Create handler with observability
	urlHandler := handler.NewURLHandler(db, redisCache)

	// Create Go Micro service with NATS plugins and observability middleware
	service := micro.NewService(
		micro.Name("url.shortener.service"),
		micro.Version(opts.Version),
		micro.Transport(natsTransport.NewTransport()),
		micro.Registry(natsRegistry.NewRegistry()),
		micro.Broker(natsBroker.NewBroker()),
		// Add observability middleware
		micro.WrapHandler(metrics.GoMicroMiddleware("url-shortener")),
		micro.WrapHandler(tracing.TraceGoMicroMiddleware("url-shortener")),
	)

	// Initialize service
	service.Init()

	// Register handler using the generated registration function
	err = pb.RegisterURLShortenerHandler(service.Server(), urlHandler)
	if err != nil {
		return nil, err
	}

	// Start metrics server in a separate goroutine
	go func() {
		http.Handle("/metrics", promhttp.HandlerFor(metricsRegistry.Registry, promhttp.HandlerOpts{}))
		opts.Log.Info("Metrics server starting on :8001/metrics")
		if err := http.ListenAndServe(":8001", nil); err != nil {
			opts.Log.WithError(err).Error("Failed to start metrics server")
		}
	}()

	opts.Log.Info("URL Shortener RPC service configured with NATS transport, registry, broker, and full observability stack")

	return &Microservice{
		service: service,
		log:     opts.Log,
		tracer:  tracer,
	}, nil
}

// Run starts the microservice
func (m *Microservice) Run() error {
	m.log.Info("Starting URL Shortener microservice with NATS and observability...")
	return m.service.Run()
}
