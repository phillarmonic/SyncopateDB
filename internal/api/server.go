package api

import (
	"context"
	"fmt"
	"github.com/phillarmonic/syncopate-db/internal/monitoring"
	"github.com/phillarmonic/syncopate-db/internal/settings"
	"net/http"
	"os"
	"os/signal"
	"sort"
	"sync"
	"time"

	"github.com/gorilla/mux"
	"github.com/rs/cors"
	"github.com/sirupsen/logrus"

	"github.com/phillarmonic/syncopate-db/internal/common"
	"github.com/phillarmonic/syncopate-db/internal/datastore"
)

// ServerConfig holds configuration for the REST API server
type ServerConfig struct {
	Port          int
	ReadTimeout   time.Duration
	WriteTimeout  time.Duration
	IdleTimeout   time.Duration
	LogLevel      logrus.Level
	RateLimit     int           // Requests per minute per IP
	RateWindow    time.Duration // Rate limit window (usually 1 minute)
	DebugMode     bool          // Flag to enable debug mode (disables goroutines)
	ColorizedLogs bool          // Flag to enable colorized log output
}

// DefaultServerConfig returns a default server configuration
func DefaultServerConfig() ServerConfig {
	logLevel, err := logrus.ParseLevel(string(settings.Config.LogLevel))
	if err != nil {
		logLevel = logrus.InfoLevel
	}

	return ServerConfig{
		Port:          settings.Config.Port,
		ReadTimeout:   15 * time.Second,
		WriteTimeout:  15 * time.Second,
		IdleTimeout:   60 * time.Second,
		LogLevel:      logLevel,
		RateLimit:     100,                           // 100 requests per minute per IP
		RateWindow:    time.Minute,                   // 1 minute window
		DebugMode:     settings.Config.Debug,         // Debug mode from settings
		ColorizedLogs: settings.Config.ColorizedLogs, // Colorized logs from settings
	}
}

// Server represents the REST API server
type Server struct {
	router        *mux.Router
	config        ServerConfig
	server        *http.Server
	engine        common.DatastoreEngine
	queryService  *datastore.QueryService
	logger        *logrus.Logger
	rateLimiter   *RateLimiter
	memoryMonitor *monitoring.MemoryMonitor
	mu            sync.RWMutex // Protect response writers in concurrent handlers
}

// NewServer creates a new REST API server
func NewServer(engine common.DatastoreEngine, queryService *datastore.QueryService, config ServerConfig) *Server {
	logger := logrus.New()
	logger.SetLevel(config.LogLevel)
	logger.SetFormatter(&logrus.JSONFormatter{})

	// Create a memory monitor with 30-second interval and store up to 100 samples
	memoryMonitor := monitoring.NewMemoryMonitor(30*time.Second, 100)

	// Start the memory monitor immediately
	memoryMonitor.Start()

	server := &Server{
		router:        mux.NewRouter(),
		config:        config,
		engine:        engine,
		queryService:  queryService,
		logger:        logger,
		rateLimiter:   NewRateLimiter(config.RateLimit, config.RateWindow),
		memoryMonitor: memoryMonitor, // Initialize the memory monitor
	}

	server.setupRoutes()
	return server
}

// setupRoutes configures the API routes
func (s *Server) setupRoutes() {
	// Root path - SyncopateDB welcome
	s.router.HandleFunc("/", s.handleWelcome).Methods(http.MethodGet)
	s.router.HandleFunc("/settings", s.handleSettings).Methods(http.MethodGet)

	// API version prefix
	api := s.router.PathPrefix("/api/v1").Subrouter()

	// Entity types
	api.HandleFunc("/entity-types", s.handleGetEntityTypes).Methods(http.MethodGet)
	api.HandleFunc("/entity-types", s.handleCreateEntityType).Methods(http.MethodPost)
	api.HandleFunc("/entity-types/{name}", s.handleGetEntityType).Methods(http.MethodGet)
	api.HandleFunc("/entity-types/{name}", s.handleUpdateEntityType).Methods(http.MethodPut) // New endpoint

	// Entities
	api.HandleFunc("/entities/{type}", s.handleListEntities).Methods(http.MethodGet)
	api.HandleFunc("/entities/{type}", s.handleCreateEntity).Methods(http.MethodPost)
	api.HandleFunc("/entities/{type}/{id}", s.handleGetEntity).Methods(http.MethodGet)
	api.HandleFunc("/entities/{type}/{id}", s.handleUpdateEntity).Methods(http.MethodPut)
	api.HandleFunc("/entities/{type}/{id}", s.handleDeleteEntity).Methods(http.MethodDelete)

	// Query
	api.HandleFunc("/query", s.handleQuery).Methods(http.MethodPost)

	// Health check
	s.router.HandleFunc("/health", s.handleHealthCheck).Methods(http.MethodGet)

	// Add a debug endpoint if in debug mode
	if s.config.DebugMode {
		s.router.HandleFunc("/debug", s.handleDebug).Methods(http.MethodGet)
		s.router.HandleFunc("/debug/entities", s.handleDebugEntities).Methods(http.MethodGet)
		// Add a new debug endpoint for schema migrations
		s.router.HandleFunc("/debug/schema", s.handleDebugSchema).Methods(http.MethodGet)
	}

	api.HandleFunc("/memory", s.handleMemoryStats).Methods(http.MethodGet)
	api.HandleFunc("/memory/visualization", s.handleVisualizationHTML).Methods(http.MethodGet)
	api.HandleFunc("/memory/sample", s.handleForceSample).Methods(http.MethodPost)
	api.HandleFunc("/memory/config", s.handleMemoryConfig).Methods(http.MethodGet, http.MethodPost)

	// Diagnostics route
	api.HandleFunc("/diagnostics", s.handleDiagnostics).Methods(http.MethodGet)
}

// handleDebug provides a debug endpoint for testing when in debug mode
func (s *Server) handleDebug(w http.ResponseWriter, r *http.Request) {
	// Example debug info - you can modify this to include whatever diagnostics you need
	debugInfo := map[string]interface{}{
		"serverTime":   time.Now().Format(time.RFC3339),
		"entityTypes":  s.engine.ListEntityTypes(),
		"debugMode":    s.config.DebugMode,
		"goroutines":   "main thread only", // Since we're in debug mode
		"requestPath":  r.URL.Path,
		"requestQuery": r.URL.RawQuery,
		"requestHeaders": func() map[string]string {
			headers := make(map[string]string)
			for name, values := range r.Header {
				if len(values) > 0 {
					headers[name] = values[0]
				}
			}
			return headers
		}(),
	}

	s.respondWithJSON(w, http.StatusOK, debugInfo, true)
}

// Start initializes and starts the server
func (s *Server) Start() error {
	// Configure CORS
	c := cors.New(cors.Options{
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Content-Type", "Content-Length", "Accept-Encoding", "X-CSRF-Token", "Authorization"},
		AllowCredentials: true,
		MaxAge:           86400, // 24 hours
	})

	// Create a chain of middleware
	handler := c.Handler(s.router)
	handler = s.logMiddleware(handler)
	handler = s.rateLimitMiddleware(handler)
	handler = s.securityHeadersMiddleware(handler)
	handler = s.requestIDMiddleware(handler)
	handler = s.recoveryMiddleware(handler)

	// Create HTTP server
	addr := fmt.Sprintf(":%d", s.config.Port)
	s.server = &http.Server{
		Addr:         addr,
		Handler:      handler,
		ReadTimeout:  s.config.ReadTimeout,
		WriteTimeout: s.config.WriteTimeout,
		IdleTimeout:  s.config.IdleTimeout,
	}

	settings.SetServerStarted(true)

	s.logger.Info("SyncopateDB server is starting on " + addr)

	// Start the server in a goroutine
	go func() {
		if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.logger.Fatalf("Could not start server: %v", err)
		}
	}()

	// Wait for the interrupt signal to gracefully shut down the server
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt)
	<-quit

	s.logger.Info("SyncopateDB Server is shutting down...")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := s.server.Shutdown(ctx); err != nil {
		s.logger.Fatalf("Server shutdown error: %v", err)
		return err
	}

	s.logger.Info("Server gracefully stopped")
	return nil
}

// logMiddleware logs information about each request
func (s *Server) logMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Create a response wrapper to capture the status code
		rw := &responseWriter{
			ResponseWriter: w,
			statusCode:     http.StatusOK,
		}

		// Call the next handler
		next.ServeHTTP(rw, r)

		// Log the request details
		duration := time.Since(start)
		s.logger.WithFields(logrus.Fields{
			"method":   r.Method,
			"path":     r.URL.Path,
			"status":   rw.statusCode,
			"duration": duration.String(),
			"ip":       r.RemoteAddr,
		}).Info("Request completed")
	})
}

// handleDebugEntities provides a debug endpoint for inspecting entity storage
func (s *Server) handleDebugEntities(w http.ResponseWriter, r *http.Request) {
	// Get entity type from query parameter
	entityType := r.URL.Query().Get("type")

	// Create response structure
	type EntityDebugInfo struct {
		ID          string                 `json:"id"`
		StorageKey  string                 `json:"storage_key"`
		Type        string                 `json:"type"`
		IDGenerator string                 `json:"id_generator"`
		Fields      map[string]interface{} `json:"fields"`
	}

	debug := struct {
		EntityType string            `json:"entity_type"`
		Entities   []EntityDebugInfo `json:"entities"`
		AllMapKeys []string          `json:"all_map_keys"`
	}{
		EntityType: entityType,
		Entities:   []EntityDebugInfo{},
		AllMapKeys: []string{},
	}

	// Use reflection to access the engine's entities map
	if engine, ok := s.engine.(*datastore.Engine); ok {
		// Get direct access to the entities map for debugging
		engine.DebugInspectEntities(func(entities map[string]common.Entity) {
			// Store all keys from the map for analysis
			for key := range entities {
				debug.AllMapKeys = append(debug.AllMapKeys, key)
			}

			// If a specific entity type is requested, filter for that type
			if entityType != "" {
				for key, entity := range entities {
					if entity.Type == entityType {
						// Get the ID generator type
						idGenType := "unknown"
						if def, err := s.engine.GetEntityDefinition(entity.Type); err == nil {
							idGenType = string(def.IDGenerator)
						}

						debug.Entities = append(debug.Entities, EntityDebugInfo{
							ID:          entity.ID,
							StorageKey:  key, // This is the actual key used in the map
							Type:        entity.Type,
							IDGenerator: idGenType,
							Fields:      entity.Fields,
						})
					}
				}
			}
		})
	}

	// Sort the keys for easier analysis
	sort.Strings(debug.AllMapKeys)

	// Return the debug information
	s.respondWithJSON(w, http.StatusOK, debug, true)
}

// responseWriter is a wrapper around http.ResponseWriter that captures the status code
type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

// WriteHeader captures the status code before writing it
func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

// GetMemoryMonitor returns the server's memory monitor instance
func (s *Server) GetMemoryMonitor() *monitoring.MemoryMonitor {
	return s.memoryMonitor
}
