package main

import (
	"flag"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/phillarmonic/syncopate-db/internal/api"
	"github.com/phillarmonic/syncopate-db/internal/datastore"
)

func main() {
	// Parse command-line flags
	port := flag.Int("port", 8080, "Port to listen on")
	logLevel := flag.String("log-level", "info", "Log level (debug, info, warn, error)")
	flag.Parse()

	// Set up logging
	logger := logrus.New()
	level, err := logrus.ParseLevel(*logLevel)
	if err != nil {
		level = logrus.InfoLevel
	}
	logger.SetLevel(level)
	logger.SetFormatter(&logrus.JSONFormatter{})

	// Initialize the data store
	logger.Info("Initializing data store...")
	engine := datastore.NewDataStoreEngine()
	queryService := datastore.NewQueryService(engine)

	// Configure and start the server
	serverConfig := api.ServerConfig{
		Port:         *port,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
		LogLevel:     level,
	}

	server := api.NewServer(engine, queryService, serverConfig)
	logger.Fatal(server.Start())
}
