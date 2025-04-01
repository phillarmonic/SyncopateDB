package api

import (
	"net/http"
	"time"

	"github.com/sirupsen/logrus"
)

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

// rateLimitMiddleware implements rate limiting (simple implementation)
func (s *Server) rateLimitMiddleware(next http.Handler) http.Handler {
	// A more robust implementation would use a token bucket or similar
	var requestCounts = make(map[string]int)
	var lastResetTime = time.Now()
	const requestLimit = 100 // Requests per minute
	const resetInterval = time.Minute

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		now := time.Now()
		ip := r.RemoteAddr

		// Reset counters if interval has passed
		if now.Sub(lastResetTime) > resetInterval {
			requestCounts = make(map[string]int)
			lastResetTime = now
		}

		// Increment request count for IP
		requestCounts[ip]++

		// Check if limit exceeded
		if requestCounts[ip] > requestLimit {
			s.respondWithError(w, http.StatusTooManyRequests, "Rate limit exceeded")
			return
		}

		next.ServeHTTP(w, r)
	})
}

// generateRequestID creates a unique request ID
func generateRequestID() string {
	// A more robust implementation would use UUID
	return time.Now().Format("20060102150405.000000")
}
