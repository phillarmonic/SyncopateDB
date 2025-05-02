package api

import (
	"fmt"
	"github.com/phillarmonic/syncopate-db/internal/about"
	"github.com/phillarmonic/syncopate-db/internal/monitoring"
	"github.com/phillarmonic/syncopate-db/internal/settings"
	"net/http"
	"runtime"
	"strings"
	"time"
)

// handleDiagnostics provides comprehensive diagnostic information about the server
func (s *Server) handleDiagnostics(w http.ResponseWriter, r *http.Request) {
	// Get memory statistics
	memStats := s.memoryMonitor.GetMemoryStatsRaw()

	// Get runtime information
	var rtStats runtime.MemStats
	runtime.ReadMemStats(&rtStats)

	// Get entity statistics
	entityTypes := s.engine.ListEntityTypes()
	entityCounts := make(map[string]int)
	totalEntities := 0

	for _, typeName := range entityTypes {
		count, err := s.engine.GetEntityCount(typeName)
		if err == nil {
			entityCounts[typeName] = count
			totalEntities += count
		} else {
			entityCounts[typeName] = -1 // Error
		}
	}

	// Get the start time from the memory monitor
	var startedAt time.Time
	if stats := s.memoryMonitor.GetStats(); stats.StartedAt.IsZero() {
		// If start time is not available, use current time
		startedAt = time.Now()
	} else {
		startedAt = stats.StartedAt
	}

	// Safely extract the peak_bytes value
	var peakBytes uint64
	if peak, ok := memStats["peak_bytes"]; ok {
		peakBytes = safeUint64(peak)
	}

	// Determine if compression is available - remove the unused variable
	compressionAvailable := s.compressor != nil

	// Create a diagnostic response with safe conversions
	diagnostic := map[string]interface{}{
		"timestamp":   time.Now().Format(time.RFC3339),
		"uptime":      time.Since(startedAt).String(),
		"version":     about.About().Version,
		"environment": determineEnvironment(),
		"go_version":  runtime.Version(),
		"goroutines":  runtime.NumGoroutine(),
		"cpu_cores":   runtime.NumCPU(),
		"settings": map[string]interface{}{
			"debug":            settings.Config.Debug,
			"log_level":        settings.Config.LogLevel,
			"port":             settings.Config.Port,
			"enable_wal":       settings.Config.EnableWAL,
			"enable_zstd":      settings.Config.EnableZSTD,     // Database compression
			"enable_http_zstd": settings.Config.EnableHTTPZSTD, // HTTP compression
			"colorized_logs":   settings.Config.ColorizedLogs,
			"server_started":   settings.Config.ServerStarted,
		},
		// Update the compression section:
		"compression": map[string]interface{}{
			"enabled":          settings.Config.EnableHTTPZSTD, // Use HTTP-specific setting
			"available":        compressionAvailable,
			"type":             "zstd",
			"status":           compressionAvailable && settings.Config.EnableHTTPZSTD,
			"client_supported": strings.Contains(r.Header.Get("Accept-Encoding"), "zstd"),
			"response_compressed": strings.Contains(r.Header.Get("Accept-Encoding"), "zstd") &&
				compressionAvailable && settings.Config.EnableHTTPZSTD,
		},
		"memory": map[string]interface{}{
			"current":          monitoring.FormatBytes(rtStats.HeapAlloc),
			"current_bytes":    rtStats.HeapAlloc,
			"peak":             monitoring.FormatBytes(peakBytes),
			"peak_bytes":       peakBytes,
			"system":           monitoring.FormatBytes(rtStats.Sys),
			"system_bytes":     rtStats.Sys,
			"heap_objects":     rtStats.HeapObjects,
			"next_gc":          monitoring.FormatBytes(rtStats.NextGC),
			"next_gc_bytes":    rtStats.NextGC,
			"gc_cpu_fraction":  rtStats.GCCPUFraction,
			"gc_cycles":        rtStats.NumGC,
			"gc_forced_cycles": rtStats.NumForcedGC,
			"sample_count":     safeUint64(memStats["sample_count"]),
		},
		"entities": map[string]interface{}{
			"types":       entityTypes,
			"type_count":  len(entityTypes),
			"total_count": totalEntities,
			"counts":      entityCounts,
		},
	}

	// Format based on the query parameter
	format := r.URL.Query().Get("format")
	if format == "text" {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)

		// Output as plaintext
		s.writeDiagnosticsText(w, diagnostic)
	} else {
		// Default to JSON
		s.respondWithJSON(w, http.StatusOK, diagnostic, true)
	}
}

// Helper function to safely convert interface{} to uint64
func safeUint64(value interface{}) uint64 {
	switch v := value.(type) {
	case uint64:
		return v
	case int64:
		return uint64(v)
	case float64:
		return uint64(v)
	case int:
		return uint64(v)
	case uint:
		return uint64(v)
	case int32:
		return uint64(v)
	case uint32:
		return uint64(v)
	default:
		return 0
	}
}

// Helper function to safely get timestamp from interface
func safeTime(value interface{}) time.Time {
	switch v := value.(type) {
	case time.Time:
		return v
	case string:
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			return time.Now() // Fallback to current time on error
		}
		return t
	default:
		return time.Now() // Default to current time
	}
}

// writeDiagnosticsText outputs the diagnostics as formatted text
func (s *Server) writeDiagnosticsText(w http.ResponseWriter, diag map[string]interface{}) {
	memory := diag["memory"].(map[string]interface{})
	entities := diag["entities"].(map[string]interface{})
	settings := diag["settings"].(map[string]interface{})
	compression := diag["compression"].(map[string]interface{})

	// Write header
	w.Write([]byte("SYNCOPATEDB DIAGNOSTIC INFORMATION\n"))
	w.Write([]byte("================================\n\n"))

	// General info
	w.Write([]byte("General Information:\n"))
	w.Write([]byte("-----------------\n"))
	w.Write([]byte("Timestamp:    " + diag["timestamp"].(string) + "\n"))
	w.Write([]byte("Uptime:       " + diag["uptime"].(string) + "\n"))
	w.Write([]byte("Version:      " + diag["version"].(string) + "\n"))
	w.Write([]byte("Environment:  " + diag["environment"].(string) + "\n"))
	w.Write([]byte("Go Version:   " + diag["go_version"].(string) + "\n"))
	w.Write([]byte("CPU Cores:    " + intToString(diag["cpu_cores"].(int)) + "\n"))
	w.Write([]byte("Goroutines:   " + intToString(diag["goroutines"].(int)) + "\n\n"))

	// Compression info
	w.Write([]byte("Compression Information:\n"))
	w.Write([]byte("-----------------------\n"))
	w.Write([]byte("Enabled:           " + boolToString(compression["enabled"].(bool)) + "\n"))
	w.Write([]byte("Available:         " + boolToString(compression["available"].(bool)) + "\n"))
	w.Write([]byte("Type:              " + compression["type"].(string) + "\n"))
	w.Write([]byte("Status:            " + boolToString(compression["status"].(bool)) + "\n"))
	w.Write([]byte("Client Supported:  " + boolToString(compression["client_supported"].(bool)) + "\n"))
	w.Write([]byte("Response Compressed: " + boolToString(compression["response_compressed"].(bool)) + "\n\n"))

	// Memory info
	w.Write([]byte("Memory Usage:\n"))
	w.Write([]byte("------------\n"))
	w.Write([]byte("Current:      " + memory["current"].(string) + "\n"))
	w.Write([]byte("Peak:         " + memory["peak"].(string) + "\n"))
	w.Write([]byte("System:       " + memory["system"].(string) + "\n"))
	w.Write([]byte("Heap Objects: " + uintToString(memory["heap_objects"].(uint64)) + "\n"))
	w.Write([]byte("Next GC:      " + memory["next_gc"].(string) + "\n"))
	w.Write([]byte("GC CPU %:     " + floatToString(memory["gc_cpu_fraction"].(float64)) + "\n"))
	w.Write([]byte("GC Cycles:    " + uintToString(memory["gc_cycles"].(uint64)) + "\n\n"))

	// Entity info
	w.Write([]byte("Entity Information:\n"))
	w.Write([]byte("------------------\n"))
	w.Write([]byte("Entity Types:  " + intToString(entities["type_count"].(int)) + "\n"))
	w.Write([]byte("Total Entities: " + intToString(entities["total_count"].(int)) + "\n"))

	// Write entity counts by type
	w.Write([]byte("\nEntity Counts by Type:\n"))
	entityTypes := entities["types"].([]string)
	entityCounts := entities["counts"].(map[string]int)

	for _, typeName := range entityTypes {
		count := entityCounts[typeName]
		w.Write([]byte(typeName + ": " + intToString(count) + "\n"))
	}
	w.Write([]byte("\n"))

	// Settings info
	w.Write([]byte("Settings:\n"))
	w.Write([]byte("---------\n"))
	w.Write([]byte("Debug Mode:       " + boolToString(settings["debug"].(bool)) + "\n"))
	w.Write([]byte("Log Level:        " + settings["log_level"].(string) + "\n"))
	w.Write([]byte("Port:             " + intToString(settings["port"].(int)) + "\n"))
	w.Write([]byte("WAL Enabled:      " + boolToString(settings["enable_wal"].(bool)) + "\n"))
	w.Write([]byte("ZSTD DB Enabled:    " + boolToString(settings["enable_zstd"].(bool)) + "\n"))
	w.Write([]byte("ZSTD HTTP Enabled:  " + boolToString(settings["enable_http_zstd"].(bool)) + "\n"))
	w.Write([]byte("Colorized Logs:   " + boolToString(settings["colorized_logs"].(bool)) + "\n"))
	w.Write([]byte("Server Started:   " + boolToString(settings["server_started"].(bool)) + "\n"))
}

// Helper functions for text formatting
func intToString(i int) string {
	return fmt.Sprintf("%d", i)
}

func uintToString(u uint64) string {
	return fmt.Sprintf("%d", u)
}

func floatToString(f float64) string {
	return fmt.Sprintf("%.2f%%", f*100)
}

func boolToString(b bool) string {
	if b {
		return "Yes"
	}
	return "No"
}

func (s *Server) compressionInfoHandler(w http.ResponseWriter, r *http.Request) {
	// Check if client supports compression
	clientSupportsZstd := strings.Contains(r.Header.Get("Accept-Encoding"), "zstd")

	// Create test payloads of different types to demonstrate compression
	jsonSample := []byte(`{
        "entities": [
            {"id": 1, "name": "Example Entity 1", "description": "This is an example entity with some data"},
            {"id": 2, "name": "Example Entity 2", "description": "This is another example entity with some data"},
            {"id": 3, "name": "Example Entity 3", "description": "This is yet another example entity with different data"}
        ],
        "metadata": {
            "total": 3,
            "page": 1,
            "pageSize": 50,
            "serverInfo": "SyncopateDB v0.1.2"
        }
    }`)

	textSample := []byte(`
        SyncopateDB - A flexible, lightweight data store with advanced query capabilities
        
        SyncopateDB provides a simple yet powerful way to store and query data.
        It supports various entity types, each with its own fields and relationships.
        Data can be queried with a flexible query language, supporting filters, sorting, and pagination.
        SyncopateDB also supports transactions, ACID compliance, and more.
    `)

	repeatingSample := []byte(strings.Repeat("SyncopateDB is awesome! ", 100))

	// Calculate compression ratios
	jsonRatio := 1.0
	textRatio := 1.0
	repeatingRatio := 1.0

	if s.compressor != nil {
		jsonRatio = s.estimateCompressionRatio(jsonSample)
		textRatio = s.estimateCompressionRatio(textSample)
		repeatingRatio = s.estimateCompressionRatio(repeatingSample)
	}

	// Create response
	response := map[string]interface{}{
		"compression_status": map[string]interface{}{
			"db_compression_enabled":   settings.Config.EnableZSTD,
			"http_compression_enabled": settings.Config.EnableHTTPZSTD,
			"compressor_available":     s.compressor != nil,
			"client_supported":         clientSupportsZstd,
			"active_for_response":      clientSupportsZstd && s.compressor != nil && settings.Config.EnableHTTPZSTD,
		},
		"compression_efficiency": map[string]interface{}{
			"json_sample": map[string]interface{}{
				"original_size": len(jsonSample),
				"ratio":         jsonRatio,
				"formatted":     formatCompressionRatio(jsonRatio),
			},
			"text_sample": map[string]interface{}{
				"original_size": len(textSample),
				"ratio":         textRatio,
				"formatted":     formatCompressionRatio(textRatio),
			},
			"repeating_sample": map[string]interface{}{
				"original_size": len(repeatingSample),
				"ratio":         repeatingRatio,
				"formatted":     formatCompressionRatio(repeatingRatio),
			},
		},
		"server_info": map[string]interface{}{
			"timestamp":   time.Now().Format(time.RFC3339),
			"version":     about.About().Version,
			"go_version":  runtime.Version(),
			"environment": determineEnvironment(),
		},
	}

	s.respondWithJSON(w, http.StatusOK, response, true)
}
