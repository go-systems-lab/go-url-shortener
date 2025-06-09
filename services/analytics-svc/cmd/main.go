package main

import (
	"github.com/sirupsen/logrus"
	"github.com/go-systems-lab/go-url-shortener/services/analytics-svc/microservice"
)

func main() {
	// Initialize logger
	logger := logrus.New()
	logger.SetLevel(logrus.InfoLevel)
	logger.Info("Starting Analytics Service with NATS...")

	// Initialize microservice using the wrapper pattern
	analyticsService, err := microservice.Init(&microservice.ClientOptions{
		Version: "latest",
		Log:     logger,
	})
	if err != nil {
		logger.WithError(err).Fatal("Failed to initialize analytics microservice")
	}

	// Run the service
	if err := analyticsService.Run(); err != nil {
		logger.WithError(err).Fatal("Failed to run analytics microservice")
	}
}
