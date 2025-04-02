package api

import (
	"net/http"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

// RateLimiter provides thread-safe rate limiting functionality
type RateLimiter struct {
	requestCounts map[string]int
	lastResetTime time.Time
	resetInterval time.Duration
	requestLimit  int
	mu            sync.Mutex
}

// NewRateLimiter creates a new rate limiter
func NewRateLimiter(limit int, interval time.Duration) *RateLimiter {
	return &RateLimiter{
		requestCounts: make(map[string]int),
		lastResetTime: time.Now(),
		resetInterval: interval,
		requestLimit:  limit,
	}
}

// Check checks if a request from the given IP exceeds the rate limit
func (rl *RateLimiter) Check(ip string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	// Reset counters if interval has passed
	now := time.Now()
	if now.Sub(rl.lastResetTime) > rl.resetInterval {
		rl.requestCounts = make(map[string]int)
		rl.lastResetTime = now
	}

	// Increment request count for IP
	rl.requestCounts[ip]++

	// Return true if limit is exceeded
	return rl.requestCounts[ip] > rl.requestLimit
}

// generateRequestID creates a unique request ID
func generateRequestID() string {
	// A more robust implementation would use UUID
	return time.Now().Format("20060102150405.000000")
}

// authMiddleware handles authentication (if implemented)
func (s *Server) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// This is where you would add authentication logic
		// For now, pass through all requests
		next.ServeHTTP(w, r)
	})
}

// requestIDMiddleware adds a unique request ID to each request
func (s *Server) requestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Add request ID header if not present
		if r.Header.Get("X-Request-ID") == "" {
			requestID := generateRequestID()
			r.Header.Set("X-Request-ID", requestID)
			w.Header().Set("X-Request-ID", requestID)
		}
		next.ServeHTTP(w, r)
	})
}

// securityHeadersMiddleware adds security-related headers to responses
func (s *Server) securityHeadersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Add security headers
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Content-Security-Policy", "default-src 'self'")
		w.Header().Set("X-XSS-Protection", "1; mode=block")
		next.ServeHTTP(w, r)
	})
}

// recoveryMiddleware catches panics and returns a 500 error
func (s *Server) recoveryMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				s.logger.WithFields(logrus.Fields{
					"error": err,
					"path":  r.URL.Path,
				}).Error("Recovered from panic")
				s.respondWithError(w, http.StatusInternalServerError, "Internal server error")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// rateLimitMiddleware implements rate limiting with proper synchronization
func (s *Server) rateLimitMiddleware(next http.Handler) http.Handler {
	limiter := NewRateLimiter(100, time.Minute) // 100 requests per minute per IP

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := r.RemoteAddr

		// Check if the request exceeds the rate limit
		if limiter.Check(ip) {
			s.respondWithError(w, http.StatusTooManyRequests, "Rate limit exceeded")
			return
		}

		next.ServeHTTP(w, r)
	})
}
