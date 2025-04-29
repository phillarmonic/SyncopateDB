// This file should be located in: internal/monitoring/memory_stats.go
package monitoring

import (
	"fmt"
	"runtime"
	"sync"
	"time"
)

// MemoryStats holds memory usage statistics
type MemoryStats struct {
	Current       uint64    // Current memory usage in bytes
	Peak          uint64    // Peak memory usage recorded in bytes
	Average       uint64    // Average memory usage in bytes
	Readings      uint64    // Number of readings taken
	Total         uint64    // Total memory accumulated (for average calculation)
	LastUpdated   time.Time // Last time stats were updated
	StartedAt     time.Time // When monitoring started
	SampleCount   int       // Number of samples collected
	SampleHistory []Sample  // Limited history of samples for charts
}

// Sample represents a single memory reading
type Sample struct {
	Timestamp   time.Time // When the sample was taken
	MemoryUsage uint64    // Memory usage in bytes at this time
}

// MemoryMonitor manages memory statistics
type MemoryMonitor struct {
	stats      MemoryStats
	mu         sync.RWMutex
	stopChan   chan struct{}
	interval   time.Duration
	historyLen int          // Maximum number of samples to keep
	ticker     *time.Ticker // Reference to ticker for interval changes
}

// NewMemoryMonitor creates a new memory monitor
func NewMemoryMonitor(interval time.Duration, historyLen int) *MemoryMonitor {
	return &MemoryMonitor{
		stats: MemoryStats{
			LastUpdated:   time.Now(),
			StartedAt:     time.Now(),
			SampleHistory: make([]Sample, 0, historyLen),
		},
		stopChan:   make(chan struct{}),
		interval:   interval,
		historyLen: historyLen,
	}
}

// Start begins memory monitoring
func (mm *MemoryMonitor) Start() {
	// Initialize the ticker if not already set
	if mm.ticker == nil {
		mm.ticker = time.NewTicker(mm.interval)

		// Take an initial sample to avoid nil data
		mm.updateStats()

		go mm.monitorRoutine()
	}
}

// Stop stops memory monitoring
func (mm *MemoryMonitor) Stop() {
	if mm.ticker != nil {
		mm.ticker.Stop()
	}
	close(mm.stopChan)
}

// GetStats returns a copy of the current memory statistics
func (mm *MemoryMonitor) GetStats() MemoryStats {
	mm.mu.RLock()
	defer mm.mu.RUnlock()

	// Return a copy to prevent external modification
	statsCopy := mm.stats

	// Make a deep copy of the sample history
	statsCopy.SampleHistory = make([]Sample, len(mm.stats.SampleHistory))
	copy(statsCopy.SampleHistory, mm.stats.SampleHistory)

	return statsCopy
}

// ForceUpdate forces an immediate update of memory stats
func (mm *MemoryMonitor) ForceUpdate() {
	mm.updateStats()
}

// updateStats collects current memory usage and updates statistics
func (mm *MemoryMonitor) updateStats() {
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	// Get the current heap allocation (HeapAlloc represents bytes of allocated heap objects)
	current := memStats.HeapAlloc

	mm.mu.Lock()
	defer mm.mu.Unlock()

	// Update current usage
	mm.stats.Current = current

	// Update peak usage if current is higher
	if current > mm.stats.Peak {
		mm.stats.Peak = current
	}

	// Update total and readings for average calculation
	mm.stats.Total += current
	mm.stats.Readings++

	// Calculate average
	if mm.stats.Readings > 0 {
		mm.stats.Average = mm.stats.Total / mm.stats.Readings
	}

	// Update last updated time
	mm.stats.LastUpdated = time.Now()

	// Increment sample count
	mm.stats.SampleCount++

	// Add to sample history
	newSample := Sample{
		Timestamp:   time.Now(),
		MemoryUsage: current,
	}

	// Add to history, maintaining maximum length
	if len(mm.stats.SampleHistory) >= mm.historyLen {
		// Remove oldest (first) element
		mm.stats.SampleHistory = append(mm.stats.SampleHistory[1:], newSample)
	} else {
		mm.stats.SampleHistory = append(mm.stats.SampleHistory, newSample)
	}
}

// monitorRoutine periodically updates memory statistics
func (mm *MemoryMonitor) monitorRoutine() {
	defer func() {
		if mm.ticker != nil {
			mm.ticker.Stop()
		}
	}()

	for {
		select {
		case <-mm.ticker.C:
			mm.updateStats()
		case <-mm.stopChan:
			return
		}
	}
}

// FormatBytes formats bytes into a human-readable string
func FormatBytes(bytes uint64) string {
	const (
		KB = 1 << 10
		MB = 1 << 20
		GB = 1 << 30
	)

	switch {
	case bytes >= GB:
		return fmt.Sprintf("%.2f GB", float64(bytes)/float64(GB))
	case bytes >= MB:
		return fmt.Sprintf("%.2f MB", float64(bytes)/float64(MB))
	case bytes >= KB:
		return fmt.Sprintf("%.2f KB", float64(bytes)/float64(KB))
	default:
		return fmt.Sprintf("%d bytes", bytes)
	}
}

// GetMemoryStatsFormatted returns memory statistics in a human-readable format
func (mm *MemoryMonitor) GetMemoryStatsFormatted() map[string]string {
	stats := mm.GetStats()

	return map[string]string{
		"current":      FormatBytes(stats.Current),
		"peak":         FormatBytes(stats.Peak),
		"average":      FormatBytes(stats.Average),
		"readings":     fmt.Sprintf("%d", stats.Readings),
		"uptime":       time.Since(stats.StartedAt).String(),
		"last_updated": stats.LastUpdated.Format(time.RFC3339),
	}
}

// GetMemoryStatsRaw returns memory statistics in raw byte formats
func (mm *MemoryMonitor) GetMemoryStatsRaw() map[string]interface{} {
	stats := mm.GetStats()

	return map[string]interface{}{
		"current_bytes":  stats.Current,
		"peak_bytes":     stats.Peak,
		"average_bytes":  stats.Average,
		"readings":       stats.Readings,
		"uptime_seconds": time.Since(stats.StartedAt).Seconds(),
		"last_updated":   stats.LastUpdated.Format(time.RFC3339),
		"started_at":     stats.StartedAt.Format(time.RFC3339),
		"sample_count":   stats.SampleCount,
	}
}

// GetTimeSeriesData returns time series data for graphing
func (mm *MemoryMonitor) GetTimeSeriesData() []map[string]interface{} {
	mm.mu.RLock()
	defer mm.mu.RUnlock()

	result := make([]map[string]interface{}, len(mm.stats.SampleHistory))

	for i, sample := range mm.stats.SampleHistory {
		result[i] = map[string]interface{}{
			"timestamp":    sample.Timestamp.Format(time.RFC3339),
			"unix_time":    sample.Timestamp.Unix(),
			"memory_bytes": sample.MemoryUsage,
			"memory_mb":    float64(sample.MemoryUsage) / (1024 * 1024),
		}
	}

	return result
}

// GetInterval returns the current monitoring interval
func (mm *MemoryMonitor) GetInterval() time.Duration {
	mm.mu.RLock()
	defer mm.mu.RUnlock()
	return mm.interval
}

// SetInterval changes the monitoring interval
func (mm *MemoryMonitor) SetInterval(newInterval time.Duration) {
	mm.mu.Lock()
	defer mm.mu.Unlock()

	mm.interval = newInterval

	// Update the ticker
	if mm.ticker != nil {
		mm.ticker.Stop()
		mm.ticker = time.NewTicker(newInterval)
	}
}

// GetHistoryLength returns the current history length
func (mm *MemoryMonitor) GetHistoryLength() int {
	mm.mu.RLock()
	defer mm.mu.RUnlock()
	return mm.historyLen
}

// SetHistoryLength changes the number of samples kept
func (mm *MemoryMonitor) SetHistoryLength(length int) {
	mm.mu.Lock()
	defer mm.mu.Unlock()

	mm.historyLen = length

	// Resize the sample history if needed
	if len(mm.stats.SampleHistory) > length {
		// Keep the most recent samples
		mm.stats.SampleHistory = mm.stats.SampleHistory[len(mm.stats.SampleHistory)-length:]
	}
}
