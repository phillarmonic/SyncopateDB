package api

import (
	"bytes"
	"fmt"
	"github.com/klauspost/compress/zstd"
	"github.com/phillarmonic/syncopate-db/internal/errors"
	"github.com/phillarmonic/syncopate-db/internal/settings"
	"github.com/sirupsen/logrus"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
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
		// Check if this is the visualization endpoint
		isVisualization := r.URL.Path == "/api/v1/memory/visualization"

		// Add security headers with appropriate CSP based on the endpoint
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("X-XSS-Protection", "1; mode=block")

		// Set Content-Security-Policy
		if isVisualization {
			// Relaxed CSP for visualization page to allow charts and styles
			w.Header().Set("Content-Security-Policy",
				"default-src 'self'; "+
					"script-src 'self' 'unsafe-inline' https://cdnjs.cloudflare.com; "+
					"style-src 'self' 'unsafe-inline'; "+
					"img-src 'self' data:; "+
					"font-src 'self'; "+
					"connect-src 'self'")
		} else {
			// Stricter CSP for other pages
			w.Header().Set("Content-Security-Policy", "default-src 'self'")
		}

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

				// Convert panic to error
				var errMsg string
				switch e := err.(type) {
				case error:
					errMsg = e.Error()
				case string:
					errMsg = e
				default:
					errMsg = fmt.Sprintf("%v", e)
				}

				s.respondWithError(w, http.StatusInternalServerError, "Internal server error",
					errors.NewError(errors.ErrCodeInternalServer, errMsg))
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// rateLimitMiddleware implements rate limiting with proper synchronization
func (s *Server) rateLimitMiddleware(next http.Handler) http.Handler {
	limiter := NewRateLimiter(1500, time.Second) // 1500 requests per second per IP

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := r.RemoteAddr

		// Check if the request exceeds the rate limit
		if limiter.Check(ip) {
			s.respondWithError(w, http.StatusTooManyRequests, "Rate limit exceeded",
				errors.NewError(errors.ErrCodeTooManyRequests, "Rate limit exceeded, try again later"))
			return
		}

		next.ServeHTTP(w, r)
	})
}

// compressionMiddleware compresses HTTP responses using zstd when appropriate
func (s *Server) compressionMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check if client accepts compression
		acceptEncoding := r.Header.Get("Accept-Encoding")
		supportsCompression := strings.Contains(acceptEncoding, "zstd")

		// Only use compression if:
		// 1. Client supports it
		// 2. HTTP compression is enabled in settings (using the new setting)
		// 3. We have a compressor initialized
		if supportsCompression && settings.Config.EnableHTTPZSTD && s.compressor != nil {
			// Create a response wrapper that compresses the output
			cw := &compressWriter{
				ResponseWriter: w,
				compressor:     s.compressor,
				statusCode:     http.StatusOK,
			}
			// Use the compressed writer instead
			w = cw
			w.Header().Set("Content-Encoding", "zstd")
		}

		// Call the next handler with potentially wrapped response writer
		next.ServeHTTP(w, r)
	})
}

// compressWriter is a ResponseWriter wrapper that compresses the response with zstd
type compressWriter struct {
	http.ResponseWriter
	compressor    *zstd.Encoder
	writer        io.Writer
	statusCode    int
	headerWritten bool
	buf           bytes.Buffer
}

// WriteHeader captures the status code and writes headers
func (cw *compressWriter) WriteHeader(code int) {
	if cw.headerWritten {
		return
	}
	cw.statusCode = code

	// Skip compression for certain status codes
	if code < 200 || code == 204 || code == 304 {
		cw.ResponseWriter.Header().Del("Content-Encoding")
		cw.ResponseWriter.WriteHeader(code)
		cw.headerWritten = true
		return
	}

	// Finalize headers and write them
	cw.ResponseWriter.Header().Add("Vary", "Accept-Encoding")
	cw.ResponseWriter.WriteHeader(code)
	cw.headerWritten = true
}

// Write compresses the data and writes it to the underlying ResponseWriter
func (cw *compressWriter) Write(p []byte) (int, error) {
	if !cw.headerWritten {
		cw.WriteHeader(http.StatusOK)
	}

	// For error responses, don't compress
	if cw.statusCode >= 400 {
		return cw.ResponseWriter.Write(p)
	}

	// Write to buffer first
	n, err := cw.buf.Write(p)
	if err != nil {
		return n, err
	}

	// Compress and write the buffer
	compressed := cw.compressor.EncodeAll(cw.buf.Bytes(), nil)
	cw.buf.Reset()

	_, err = cw.ResponseWriter.Write(compressed)
	return n, err
}
