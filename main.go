package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/phillarmonic/syncopate-db/internal/api"
	"github.com/phillarmonic/syncopate-db/internal/datastore"
	"github.com/phillarmonic/syncopate-db/internal/persistence"
	"github.com/phillarmonic/syncopate-db/internal/settings"
)

func main() {
	// Parse command-line flags (will override environment variables)
	port := flag.Int("port", settings.Config.Port, "Port to listen on")
	logLevel := flag.String("log-level", string(settings.Config.LogLevel), "Log level (debug, info, warn, error)")
	dataDir := flag.String("data-dir", "./data", "Directory for data storage")
	cacheSize := flag.Int("cache-size", 10000, "Number of entities to cache in memory")
	snapshotInterval := flag.Int("snapshot-interval", 600, "Snapshot interval in seconds")
	syncWrites := flag.Bool("sync-writes", true, "Sync writes to disk immediately")
	debugMode := flag.Bool("debug", settings.Config.Debug, "Enable debug mode (disables goroutines for easier debugging)")
	colorLogs := flag.Bool("color-logs", settings.Config.ColorizedLogs, "Enable colorized log output")
	flag.Parse()

	// Update settings from flags (flags have higher priority)
	settings.Config.Port = *port
	settings.Config.LogLevel = settings.LogLevel(*logLevel)
	settings.Config.Debug = *debugMode
	settings.Config.ColorizedLogs = *colorLogs

	// Set up logging
	logger := logrus.New()
	level, err := logrus.ParseLevel(string(settings.Config.LogLevel))
	if err != nil {
		level = logrus.InfoLevel
	}
	logger.SetLevel(level)

	// Choose formatter based on colorized setting
	if settings.Config.ColorizedLogs {
		// Configure text formatter with colors
		textFormatter := &logrus.TextFormatter{
			ForceColors:            true,
			DisableTimestamp:       false,
			FullTimestamp:          true,
			DisableLevelTruncation: false,
			PadLevelText:           true,
		}
		logger.SetFormatter(textFormatter)
		logger.WithField("colorized", true).Debug("Colorized logging enabled")
	} else {
		// Use JSON formatter for structured logging
		logger.SetFormatter(&logrus.JSONFormatter{})
		logger.WithField("colorized", false).Debug("JSON logging enabled")
	}

	if settings.Config.Debug {
		logger.Info("Debug mode enabled - server will run synchronously")
	}

	var engine *datastore.Engine
	var queryService *datastore.QueryService
	var persistenceManager *persistence.Manager

	// Ensure data directory exists
	if err := os.MkdirAll(*dataDir, 0755); err != nil {
		logger.Fatalf("Failed to create data directory: %v", err)
	}

	// Initialize persistent data store
	logger.Info("Initializing persistent data store...")

	// Configure persistence
	persistenceConfig := persistence.Config{
		Path:             filepath.Clean(*dataDir),
		CacheSize:        *cacheSize,
		SyncWrites:       *syncWrites,
		SnapshotInterval: time.Duration(*snapshotInterval) * time.Second,
		Logger:           logger,
	}

	// Create a persistence manager
	persistenceManager, err = persistence.NewManager(persistenceConfig)
	if err != nil {
		logger.Fatalf("Failed to initialize persistence: %v", err)
	}
	defer func() {
		if err := persistenceManager.Close(); err != nil {
			logger.Errorf("Error closing persistence manager: %v", err)
		}
	}()

	// Create the datastore engine with persistence
	engine = datastore.NewDataStoreEngine(datastore.EngineConfig{
		Persistence:       persistenceManager.GetPersistenceProvider(),
		EnablePersistence: true,
	})

	// Set the engine in the persistence manager
	persistenceManager.SetEngine(engine)

	// Set up a background garbage collection
	if persistenceManager != nil && !settings.Config.Debug {
		go runGarbageCollection(persistenceManager, logger)
	}

	// Log successful initialization
	logger.Infof("Persistent data store initialized at %s", *dataDir)

	// Initialize query service
	queryService = datastore.NewQueryService(engine)

	// Configure and start the server
	serverConfig := api.ServerConfig{
		Port:         settings.Config.Port,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
		LogLevel:     level,
		RateLimit:    100,                   // 100 requests per minute per IP
		RateWindow:   time.Minute,           // 1 minute window
		DebugMode:    settings.Config.Debug, // Set debug mode from settings
	}

	server := api.NewServer(engine, queryService, serverConfig)

	// Add graceful shutdown for persistence
	if persistenceManager != nil {
		// Force a snapshot before exiting
		fmt.Println("Press Ctrl+C to exit and save data")
	}

	logger.Info(server.Start())
}

// runGarbageCollection periodically runs Badger garbage collection
func runGarbageCollection(manager *persistence.Manager, logger *logrus.Logger) {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		logger.Debug("Running Badger value log garbage collection")
		err := manager.RunValueLogGC(0.7) // 0.7 is the discard ratio
		if err == nil {
			// If GC succeeded, run it again to collect more garbage
			logger.Debug("Value log GC successful, running again")
			// Add a small delay to give other processes a chance to run
			time.Sleep(500 * time.Millisecond)
			manager.RunValueLogGC(0.7)
		} else if err != nil && err.Error() != "Nothing to discard" {
			logger.Warnf("Error during value log GC: %v", err)
		}
	}
}
