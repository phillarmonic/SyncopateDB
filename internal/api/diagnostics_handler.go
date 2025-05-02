package api

import (
	"fmt"
	"github.com/phillarmonic/syncopate-db/internal/about"
	"github.com/phillarmonic/syncopate-db/internal/monitoring"
	"github.com/phillarmonic/syncopate-db/internal/settings"
	"net/http"
	"runtime"
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

	// Create diagnostic response with safe conversions
	diagnostic := map[string]interface{}{
		"timestamp":   time.Now().Format(time.RFC3339),
		"uptime":      time.Since(startedAt).String(),
		"version":     about.About().Version,
		"environment": determineEnvironment(),
		"go_version":  runtime.Version(),
		"goroutines":  runtime.NumGoroutine(),
		"cpu_cores":   runtime.NumCPU(),
		"settings": map[string]interface{}{
			"debug":          settings.Config.Debug,
			"log_level":      settings.Config.LogLevel,
			"port":           settings.Config.Port,
			"enable_wal":     settings.Config.EnableWAL,
			"enable_zstd":    settings.Config.EnableZSTD,
			"colorized_logs": settings.Config.ColorizedLogs,
			"server_started": settings.Config.ServerStarted,
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
	w.Write([]byte("ZSTD Enabled:     " + boolToString(settings["enable_zstd"].(bool)) + "\n"))
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
