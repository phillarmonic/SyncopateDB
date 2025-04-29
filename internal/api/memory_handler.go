// This file should be located in: internal/api/memory_handler.go
package api

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/phillarmonic/syncopate-db/internal/monitoring"
)

// MemoryStatsResponse represents the response for memory statistics
type MemoryStatsResponse struct {
	Current     string    `json:"current"`
	Peak        string    `json:"peak"`
	Average     string    `json:"average"`
	Readings    uint64    `json:"readings"`
	LastUpdated time.Time `json:"last_updated"`
	Uptime      string    `json:"uptime"`
}

// TimeSeriesResponse represents a collection of memory samples for time series display
type TimeSeriesResponse struct {
	Samples []monitoring.Sample `json:"samples"`
}

// handleMemoryStats handles requests for memory statistics
func (s *Server) handleMemoryStats(w http.ResponseWriter, r *http.Request) {
	format := r.URL.Query().Get("format")

	switch format {
	case "raw":
		// Return raw bytes values for programmatic use
		s.respondWithJSON(w, http.StatusOK, s.memoryMonitor.GetMemoryStatsRaw())
	case "timeseries":
		// Return time series data for graphs
		s.respondWithJSON(w, http.StatusOK, s.memoryMonitor.GetTimeSeriesData())
	case "text":
		// Return as plain text for terminal display
		stats := s.memoryMonitor.GetMemoryStatsFormatted()

		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)

		fmt.Fprintf(w, "SYNCOPATEDB MEMORY STATISTICS\n")
		fmt.Fprintf(w, "==============================\n")
		fmt.Fprintf(w, "Current: %s\n", stats["current"])
		fmt.Fprintf(w, "Peak:    %s\n", stats["peak"])
		fmt.Fprintf(w, "Average: %s\n", stats["average"])
		fmt.Fprintf(w, "Samples: %s\n", stats["readings"])
		fmt.Fprintf(w, "Uptime:  %s\n", stats["uptime"])
		fmt.Fprintf(w, "Updated: %s\n", stats["last_updated"])
	default:
		// Default to formatted values in JSON
		s.respondWithJSON(w, http.StatusOK, s.memoryMonitor.GetMemoryStatsFormatted())
	}
}

// handleVisualizationHTML serves an HTML page with memory usage visualization
func (s *Server) handleVisualizationHTML(w http.ResponseWriter, r *http.Request) {
	// Return a self-contained HTML page with charts
	htmlContent := `<!DOCTYPE html>
<html>
<head>
    <title>SyncopateDB Memory Usage</title>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <script src="https://cdnjs.cloudflare.com/ajax/libs/Chart.js/3.9.1/chart.min.js" integrity="sha512-ElRFoEQdI5Ht6kZvyzXhYG9NqjtkmlkfYk0wr6wHxU9JEHakS7UJZNeml5ALk+8IKlU6jDgMabC3vkumRokgJA==" crossorigin="anonymous" referrerpolicy="no-referrer"></script>
    <style>
        body { font-family: Arial, sans-serif; margin: 20px; background-color: #f5f5f5; }
        .container { max-width: 1200px; margin: 0 auto; background-color: white; padding: 20px; border-radius: 8px; box-shadow: 0 2px 4px rgba(0,0,0,0.1); }
        h1 { color: #333; text-align: center; }
        .stats-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(300px, 1fr)); gap: 20px; margin-bottom: 30px; }
        .stat-card { background-color: #f9f9f9; border-radius: 8px; padding: 20px; box-shadow: 0 1px 3px rgba(0,0,0,0.1); text-align: center; }
        .stat-value { font-size: 24px; font-weight: bold; color: #2a6bb1; margin: 10px 0; }
        .stat-label { font-size: 16px; color: #666; }
        .chart-container { height: 400px; margin-top: 30px; }
        .controls { margin: 20px 0; text-align: center; }
        button { padding: 8px 16px; margin: 0 10px; background-color: #2a6bb1; color: white; border: none; border-radius: 4px; cursor: pointer; }
        button:hover { background-color: #1e5290; }
        .auto-refresh { display: flex; align-items: center; justify-content: center; margin-top: 10px; }
        .refresh-label { margin-right: 10px; }
    </style>
</head>
<body>
    <div class="container">
        <h1>SyncopateDB Memory Usage Dashboard</h1>
        
        <div class="stats-grid">
            <div class="stat-card">
                <div class="stat-label">Current Memory Usage</div>
                <div class="stat-value" id="current-memory">Loading...</div>
            </div>
            <div class="stat-card">
                <div class="stat-label">Peak Memory Usage</div>
                <div class="stat-value" id="peak-memory">Loading...</div>
            </div>
            <div class="stat-card">
                <div class="stat-label">Average Memory Usage</div>
                <div class="stat-value" id="average-memory">Loading...</div>
            </div>
            <div class="stat-card">
                <div class="stat-label">Uptime</div>
                <div class="stat-value" id="uptime">Loading...</div>
            </div>
        </div>
        
        <div class="controls">
            <button id="refresh-btn">Refresh Data</button>
            <div class="auto-refresh">
                <span class="refresh-label">Auto-refresh:</span>
                <select id="refresh-interval">
                    <option value="0">Off</option>
                    <option value="5" selected>5 seconds</option>
                    <option value="15">15 seconds</option>
                    <option value="30">30 seconds</option>
                    <option value="60">1 minute</option>
                </select>
            </div>
        </div>
        
        <div class="chart-container">
            <canvas id="memory-chart"></canvas>
        </div>
    </div>

    <script>
        // Global variables
        let memoryChart;
        let refreshInterval;
        let timeSeriesData = [];
        
        // Initialize the dashboard
        document.addEventListener('DOMContentLoaded', function() {
            // Initial data load
            loadMemoryStats();
            loadTimeSeriesData();
            
            // Set up refresh button
            document.getElementById('refresh-btn').addEventListener('click', function() {
                loadMemoryStats();
                loadTimeSeriesData();
            });
            
            // Set up auto-refresh
            document.getElementById('refresh-interval').addEventListener('change', setupAutoRefresh);
            setupAutoRefresh();
        });
        
        // Set up auto-refresh based on dropdown selection
        function setupAutoRefresh() {
            const intervalSelect = document.getElementById('refresh-interval');
            const seconds = parseInt(intervalSelect.value);
            
            // Clear existing interval
            if (refreshInterval) {
                clearInterval(refreshInterval);
                refreshInterval = null;
            }
            
            // Set new interval if not disabled
            if (seconds > 0) {
                refreshInterval = setInterval(function() {
                    loadMemoryStats();
                    loadTimeSeriesData();
                }, seconds * 1000);
            }
        }
        
        // Load memory statistics
        function loadMemoryStats() {
            fetch('/api/v1/memory')
                .then(response => response.json())
                .then(data => {
                    document.getElementById('current-memory').textContent = data.current;
                    document.getElementById('peak-memory').textContent = data.peak;
                    document.getElementById('average-memory').textContent = data.average;
                    document.getElementById('uptime').textContent = data.uptime;
                })
                .catch(error => console.error('Error fetching memory stats:', error));
        }
        
        // Load time series data and update chart
        function loadTimeSeriesData() {
            fetch('/api/v1/memory?format=timeseries')
                .then(response => response.json())
                .then(data => {
                    timeSeriesData = data;
                    updateChart();
                })
                .catch(error => console.error('Error fetching time series data:', error));
        }
        
        // Update or initialize the chart
        function updateChart() {
            const ctx = document.getElementById('memory-chart').getContext('2d');
            
            // Extract data points
            const labels = timeSeriesData.map(item => {
                const date = new Date(item.timestamp);
                return date.toLocaleTimeString();
            });
            
            const memoryData = timeSeriesData.map(item => item.memory_mb);
            
            // If chart already exists, update it
            if (memoryChart) {
                memoryChart.data.labels = labels;
                memoryChart.data.datasets[0].data = memoryData;
                memoryChart.update();
            } else {
                // Create new chart
                memoryChart = new Chart(ctx, {
                    type: 'line',
                    data: {
                        labels: labels,
                        datasets: [{
                            label: 'Memory Usage (MB)',
                            data: memoryData,
                            fill: true,
                            borderColor: '#2a6bb1',
                            backgroundColor: 'rgba(42, 107, 177, 0.1)',
                            tension: 0.3,
                            pointRadius: 3,
                            pointBackgroundColor: '#2a6bb1'
                        }]
                    },
                    options: {
                        responsive: true,
                        maintainAspectRatio: false,
                        plugins: {
                            legend: {
                                position: 'top',
                            },
                            tooltip: {
                                callbacks: {
                                    label: function(context) {
                                        return 'Memory: ' + context.parsed.y.toFixed(2) + ' MB';
                                    }
                                }
                            }
                        },
                        scales: {
                            x: {
                                title: {
                                    display: true,
                                    text: 'Time'
                                }
                            },
                            y: {
                                beginAtZero: true,
                                title: {
                                    display: true,
                                    text: 'Memory (MB)'
                                }
                            }
                        }
                    }
                });
            }
        }
    </script>
</body>
</html>`

	w.Header().Set("Content-Type", "text/html")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(htmlContent))
}

// handleForceSample forces an immediate memory sample
func (s *Server) handleForceSample(w http.ResponseWriter, r *http.Request) {
	s.memoryMonitor.ForceUpdate()
	s.respondWithJSON(w, http.StatusOK, map[string]string{
		"status":  "ok",
		"message": "Memory sample taken successfully",
	})
}

// handleMemoryConfig allows configuration of memory monitor parameters
func (s *Server) handleMemoryConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		// Return current configuration
		s.respondWithJSON(w, http.StatusOK, map[string]interface{}{
			"interval_seconds": s.memoryMonitor.GetInterval().Seconds(),
			"history_length":   s.memoryMonitor.GetHistoryLength(),
		})
		return
	}

	// Must be POST for configuration changes
	if r.Method != http.MethodPost {
		s.respondWithError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	// Process interval changes
	intervalStr := r.URL.Query().Get("interval")
	if intervalStr != "" {
		interval, err := strconv.Atoi(intervalStr)
		if err != nil || interval < 1 {
			s.respondWithError(w, http.StatusBadRequest, "Invalid interval value")
			return
		}

		s.memoryMonitor.SetInterval(time.Duration(interval) * time.Second)
	}

	// Process history length changes
	historyStr := r.URL.Query().Get("history_length")
	if historyStr != "" {
		historyLen, err := strconv.Atoi(historyStr)
		if err != nil || historyLen < 1 {
			s.respondWithError(w, http.StatusBadRequest, "Invalid history length")
			return
		}

		s.memoryMonitor.SetHistoryLength(historyLen)
	}

	s.respondWithJSON(w, http.StatusOK, map[string]string{
		"status":  "ok",
		"message": "Memory monitor configuration updated",
	})
}
