package monitoring

import (
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/sirupsen/logrus"
)

// TerminalMonitor displays memory usage information in the terminal
type TerminalMonitor struct {
	logger        *logrus.Logger
	memoryMonitor *MemoryMonitor
	stopChan      chan struct{}
	interval      time.Duration
	quietMode     bool // If true, only logs at start and shutdown, not periodically
}

// NewTerminalMonitor creates a new terminal monitor
func NewTerminalMonitor(memoryMonitor *MemoryMonitor, logger *logrus.Logger, interval time.Duration) *TerminalMonitor {
	return &TerminalMonitor{
		logger:        logger,
		memoryMonitor: memoryMonitor,
		stopChan:      make(chan struct{}),
		interval:      interval,
		quietMode:     false,
	}
}

// SetQuietMode sets whether the monitor should output periodically
func (tm *TerminalMonitor) SetQuietMode(quiet bool) {
	tm.quietMode = quiet
}

// Start begins terminal monitoring
func (tm *TerminalMonitor) Start() {
	// Start periodic monitoring
	go tm.monitorRoutine()

	// Output initial memory stats
	tm.logMemoryStats()

	// Also capture interrupt signal to show a final memory report
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-signalChan
		tm.logFinalReport()
	}()
}

// Stop stops terminal monitoring
func (tm *TerminalMonitor) Stop() {
	close(tm.stopChan)
}

// monitorRoutine periodically outputs memory statistics
func (tm *TerminalMonitor) monitorRoutine() {
	ticker := time.NewTicker(tm.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if !tm.quietMode {
				tm.logMemoryStats()
			}
		case <-tm.stopChan:
			// Final memory stats before stopping
			tm.logFinalReport()
			return
		}
	}
}

// logMemoryStats outputs current memory statistics to the logger
func (tm *TerminalMonitor) logMemoryStats() {
	stats := tm.memoryMonitor.GetMemoryStatsFormatted()

	tm.logger.WithFields(logrus.Fields{
		"current": stats["current"],
		"peak":    stats["peak"],
		"average": stats["average"],
	}).Info("Memory usage")
}

// logFinalReport outputs a comprehensive memory report
func (tm *TerminalMonitor) logFinalReport() {
	stats := tm.memoryMonitor.GetStats()

	// Get additional runtime stats for the final report
	var rtStats runtime.MemStats
	runtime.ReadMemStats(&rtStats)

	tm.logger.WithFields(logrus.Fields{
		"current_memory":   FormatBytes(stats.Current),
		"peak_memory":      FormatBytes(stats.Peak),
		"average_memory":   FormatBytes(stats.Average),
		"uptime":           time.Since(stats.StartedAt).String(),
		"samples_taken":    stats.Readings,
		"system_memory":    FormatBytes(rtStats.Sys),
		"heap_allocated":   FormatBytes(rtStats.HeapAlloc),
		"heap_objects":     rtStats.HeapObjects,
		"gc_cycles":        rtStats.NumGC,
		"forced_gc_cycles": rtStats.NumForcedGC,
		"gc_cpu_fraction":  fmt.Sprintf("%.2f%%", rtStats.GCCPUFraction*100),
		"goroutines":       runtime.NumGoroutine(),
		"next_gc_target":   FormatBytes(rtStats.NextGC),
	}).Info("Memory usage final report")
}

// GetRuntimeStats gets detailed runtime memory statistics
func (tm *TerminalMonitor) GetRuntimeStats() map[string]interface{} {
	var rtStats runtime.MemStats
	runtime.ReadMemStats(&rtStats)

	return map[string]interface{}{
		"mem_stats": map[string]interface{}{
			"alloc":           rtStats.Alloc,
			"total_alloc":     rtStats.TotalAlloc,
			"sys":             rtStats.Sys,
			"heap_alloc":      rtStats.HeapAlloc,
			"heap_sys":        rtStats.HeapSys,
			"heap_idle":       rtStats.HeapIdle,
			"heap_in_use":     rtStats.HeapInuse,
			"heap_released":   rtStats.HeapReleased,
			"heap_objects":    rtStats.HeapObjects,
			"stack_in_use":    rtStats.StackInuse,
			"stack_sys":       rtStats.StackSys,
			"next_gc":         rtStats.NextGC,
			"gc_cpu_fraction": rtStats.GCCPUFraction,
			"num_gc":          rtStats.NumGC,
			"num_forced_gc":   rtStats.NumForcedGC,
		},
		"goroutines":        runtime.NumGoroutine(),
		"gc_memory_percent": rtStats.GCCPUFraction * 100,
		"formatted": map[string]string{
			"alloc":         FormatBytes(rtStats.Alloc),
			"total_alloc":   FormatBytes(rtStats.TotalAlloc),
			"sys":           FormatBytes(rtStats.Sys),
			"heap_alloc":    FormatBytes(rtStats.HeapAlloc),
			"heap_sys":      FormatBytes(rtStats.HeapSys),
			"heap_idle":     FormatBytes(rtStats.HeapIdle),
			"heap_in_use":   FormatBytes(rtStats.HeapInuse),
			"heap_released": FormatBytes(rtStats.HeapReleased),
			"stack_in_use":  FormatBytes(rtStats.StackInuse),
			"stack_sys":     FormatBytes(rtStats.StackSys),
			"next_gc":       FormatBytes(rtStats.NextGC),
		},
	}
}
