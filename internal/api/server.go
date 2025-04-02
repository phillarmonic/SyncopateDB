package api

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
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
	Port         int
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
	IdleTimeout  time.Duration
	LogLevel     logrus.Level
	RateLimit    int           // Requests per minute per IP
	RateWindow   time.Duration // Rate limit window (usually 1 minute)
}

// DefaultServerConfig returns a default server configuration
func DefaultServerConfig() ServerConfig {
	return ServerConfig{
		Port:         8080,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
		LogLevel:     logrus.InfoLevel,
		RateLimit:    100,         // 100 requests per minute per IP
		RateWindow:   time.Minute, // 1 minute window
	}
}

// Server represents the REST API server
type Server struct {
	router       *mux.Router
	config       ServerConfig
	server       *http.Server
	engine       common.DatastoreEngine
	queryService *datastore.QueryService
	logger       *logrus.Logger
	rateLimiter  *RateLimiter
	mu           sync.RWMutex // Protect response writers in concurrent handlers
}

// NewServer creates a new REST API server
func NewServer(engine common.DatastoreEngine, queryService *datastore.QueryService, config ServerConfig) *Server {
	logger := logrus.New()
	logger.SetLevel(config.LogLevel)
	logger.SetFormatter(&logrus.JSONFormatter{})

	server := &Server{
		router:       mux.NewRouter(),
		config:       config,
		engine:       engine,
		queryService: queryService,
		logger:       logger,
		rateLimiter:  NewRateLimiter(config.RateLimit, config.RateWindow),
	}

	server.setupRoutes()
	return server
}

// setupRoutes configures the API routes
func (s *Server) setupRoutes() {
	// Root path - SyncopateDB welcome
	s.router.HandleFunc("/", s.handleWelcome).Methods(http.MethodGet)

	// API version prefix
	api := s.router.PathPrefix("/api/v1").Subrouter()

	// Entity types
	api.HandleFunc("/entity-types", s.handleGetEntityTypes).Methods(http.MethodGet)
	api.HandleFunc("/entity-types", s.handleCreateEntityType).Methods(http.MethodPost)
	api.HandleFunc("/entity-types/{name}", s.handleGetEntityType).Methods(http.MethodGet)

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

	// Start server in a goroutine
	go func() {
		s.logger.Infof("Starting server on %s", addr)
		if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.logger.Fatalf("Could not start server: %v", err)
		}
	}()

	// Wait for interrupt signal to gracefully shut down the server
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt)
	<-quit

	s.logger.Info("Server is shutting down...")
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
