package main

import (
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/sirupsen/logrus"

	"github.com/go-systems-lab/go-url-shortener/services/redirect-svc/microservice"
)

// Version may be changed during build via --ldflags parameter
var Version = "latest"

func main() {
	logger := logrus.New()
	logger.SetFormatter(&logrus.JSONFormatter{})
	logger.Info("Starting Redirect RPC Service with NATS...")

	// Initialize microservice with NATS plugins
	microService, err := microservice.Init(&microservice.ClientOptions{
		Version: Version,
		Log:     logger,
	})
	if err != nil {
		logger.WithError(err).Fatal("Failed to initialize redirect micro-service")
	}

	// Run microservice
	if err := microService.Run(); err != nil {
		logger.WithError(err).Fatal("Failed to run redirect micro-service")
	}
}
