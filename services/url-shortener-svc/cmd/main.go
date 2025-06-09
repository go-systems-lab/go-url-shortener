package main

import (
	"github.com/sirupsen/logrus"

	// Import NATS plugins

	"github.com/go-systems-lab/go-url-shortener/services/url-shortener-svc/microservice"
)

// Version may be changed during build via --ldflags parameter
var Version = "latest"

func main() {
	logger := logrus.New()
	logger.SetFormatter(&logrus.JSONFormatter{})
	logger.Info("Starting URL Shortener RPC Service with NATS...")

	// Initialize microservice with NATS plugins
	microService, err := microservice.Init(&microservice.ClientOptions{
		Version: Version,
		Log:     logger,
	})
	if err != nil {
		logger.WithError(err).Fatal("Failed to initialize micro-service")
	}

	// Run microservice
	if err := microService.Run(); err != nil {
		logger.WithError(err).Fatal("Failed to run micro-service")
	}
}
