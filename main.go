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
)

func main() {
	// Parse command-line flags
	port := flag.Int("port", 8080, "Port to listen on")
	logLevel := flag.String("log-level", "info", "Log level (debug, info, warn, error)")
	dataDir := flag.String("data-dir", "./data", "Directory for data storage")
	memoryOnly := flag.Bool("memory-only", false, "Run in memory-only mode without persistence")
	cacheSize := flag.Int("cache-size", 10000, "Number of entities to cache in memory")
	snapshotInterval := flag.Int("snapshot-interval", 600, "Snapshot interval in seconds")
	syncWrites := flag.Bool("sync-writes", true, "Sync writes to disk immediately")
	flag.Parse()

	// Set up logging
	logger := logrus.New()
	level, err := logrus.ParseLevel(*logLevel)
	if err != nil {
		level = logrus.InfoLevel
	}
	logger.SetLevel(level)
	logger.SetFormatter(&logrus.JSONFormatter{})

	var engine *datastore.Engine
	var queryService *datastore.QueryService
	var persistenceManager *persistence.Manager

	if *memoryOnly {
		// Initialize in-memory data store
		logger.Info("Initializing in-memory data store (no persistence)...")
		engine = datastore.NewDataStoreEngine()
	} else {
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

		// Create persistence manager
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

		// Set up background garbage collection
		if persistenceManager != nil {
			go runGarbageCollection(persistenceManager, logger)
		}

		// Log successful initialization
		logger.Infof("Persistent data store initialized at %s", *dataDir)
	}

	// Initialize query service
	queryService = datastore.NewQueryService(engine)

	// Configure and start the server
	serverConfig := api.ServerConfig{
		Port:         *port,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
		LogLevel:     level,
	}

	server := api.NewServer(engine, queryService, serverConfig)

	// Add graceful shutdown for persistence
	if !*memoryOnly && persistenceManager != nil {
		// Force a snapshot before exiting
		fmt.Println("Press Ctrl+C to exit and save data")
	}

	logger.Fatal(server.Start())
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
