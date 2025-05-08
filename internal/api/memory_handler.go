package api

import (
	"fmt"
	"github.com/phillarmonic/syncopate-db/internal/errors"
	"net/http"
	"strconv"
	"strings"
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

	// Get stats from the memory monitor
	stats := s.memoryMonitor.GetStats()

	// Format the uptime in a human-readable way
	formattedUptime := formatUptime(time.Since(stats.StartedAt))

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
		fmt.Fprintf(w, "Uptime:  %s\n", formattedUptime)
		fmt.Fprintf(w, "Updated: %s\n", stats["last_updated"])
	default:
		// Default to formatted values in JSON with human-readable uptime
		formattedStats := s.memoryMonitor.GetMemoryStatsFormatted()
		formattedStats["uptime"] = formattedUptime

		s.respondWithJSON(w, http.StatusOK, formattedStats)
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
        
        /* Connection alert styles */
        .connection-alert {
            background-color: #f8d7da;
            color: #721c24;
            padding: 15px;
            border-radius: 8px;
            margin-bottom: 20px;
            border: 1px solid #f5c6cb;
            display: none; /* Hidden by default */
            text-align: center;
            font-weight: bold;
        }
        .connection-alert.visible {
            display: block;
            animation: pulse 2s infinite;
        }
        @keyframes pulse {
            0% { opacity: 1; }
            50% { opacity: 0.8; }
            100% { opacity: 1; }
        }
    </style>
</head>
<body>
    <div class="container">
        <h1>SyncopateDB Memory Usage Dashboard</h1>
        
        <!-- Connection alert box -->
        <div id="connection-alert" class="connection-alert">
            <span>⚠️ Connection Error: Unable to connect to the server. Data may be stale. Retrying... ⚠️</span>
        </div>
        
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
					<option value="1" selected>1 second</option>
                    <option value="5">5 seconds</option>
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
        let connectionErrorCount = 0;
        let connectionAlert = document.getElementById('connection-alert');
        
        // Initialize the dashboard
        document.addEventListener('DOMContentLoaded', function() {
            // Get references
            connectionAlert = document.getElementById('connection-alert');
            
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
        
        // Handle connection errors
        function handleConnectionError(error) {
            console.error('Connection error:', error);
            connectionErrorCount++;
            
            // Show the alert after consecutive errors
            if (connectionErrorCount >= 2) {
                connectionAlert.classList.add('visible');
            }
        }
        
        // Reset connection errors
        function resetConnectionErrors() {
            connectionErrorCount = 0;
            connectionAlert.classList.remove('visible');
        }
        
        // Format uptime in a more readable format (months, weeks, days, hours, minutes, seconds)
        function formatUptime(uptimeString) {
            // Parse the uptime string which might be in format like "2160h41m15.123456s" or "90d5h30m15s"
            // First, let's extract all time components using regex
            const regex = /(\d+)([a-z]+)/g;
            let match;
            
            // Time values in seconds
            const second = 1;
            const minute = 60 * second;
            const hour = 60 * minute;
            const day = 24 * hour;
            const week = 7 * day;
            const month = 30 * day; // Approximation
            
            // Initialize accumulators
            let totalSeconds = 0;
            
            // Extract and convert all time components to seconds
            while ((match = regex.exec(uptimeString)) !== null) {
                const value = parseInt(match[1], 10);
                const unit = match[2];
                
                switch(unit) {
                    case 's':
                        totalSeconds += value;
                        break;
                    case 'm':
                    case 'min':
                        totalSeconds += value * minute;
                        break;
                    case 'h':
                        totalSeconds += value * hour;
                        break;
                    case 'd':
                        totalSeconds += value * day;
                        break;
                    case 'w':
                        totalSeconds += value * week;
                        break;
                    case 'mo':
                        totalSeconds += value * month;
                        break;
                }
            }
            
            // If we couldn't parse anything, return the original string
            if (totalSeconds === 0) {
                return uptimeString;
            }
            
            // Calculate the time components
            const months = Math.floor(totalSeconds / month);
            totalSeconds %= month;
            
            const weeks = Math.floor(totalSeconds / week);
            totalSeconds %= week;
            
            const days = Math.floor(totalSeconds / day);
            totalSeconds %= day;
            
            const hours = Math.floor(totalSeconds / hour);
            totalSeconds %= hour;
            
            const minutes = Math.floor(totalSeconds / minute);
            totalSeconds %= minute;
            
            const seconds = Math.floor(totalSeconds);
            
            // Build the formatted string, omitting zero values
            const parts = [];
            
            if (months > 0) {
                parts.push(months + (months === 1 ? ' month' : ' months'));
            }
            
            if (weeks > 0) {
                parts.push(weeks + (weeks === 1 ? ' week' : ' weeks'));
            }
            
            if (days > 0) {
                parts.push(days + (days === 1 ? ' day' : ' days'));
            }
            
            if (hours > 0) {
                parts.push(hours + (hours === 1 ? ' hour' : ' hours'));
            }
            
            if (minutes > 0) {
                parts.push(minutes + (minutes === 1 ? ' minute' : ' minutes'));
            }
            
            if (seconds > 0 || parts.length === 0) {
                parts.push(seconds + (seconds === 1 ? ' second' : ' seconds'));
            }
            
            return parts.join(', ');
        }
        
        // Load memory statistics
        function loadMemoryStats() {
            fetch('/api/v1/memory')
                .then(response => {
                    if (!response.ok) {
                        throw new Error('Network response was not ok: ' + response.status);
                    }
                    return response.json();
                })
                .then(data => {
                    // Reset connection errors on successful response
                    resetConnectionErrors();
                    
                    document.getElementById('current-memory').textContent = data.current;
                    document.getElementById('peak-memory').textContent = data.peak;
                    document.getElementById('average-memory').textContent = data.average;
                    
                    // Format the uptime
                    const formattedUptime = formatUptime(data.uptime);
                    document.getElementById('uptime').textContent = formattedUptime;
                })
                .catch(error => {
                    handleConnectionError(error);
                });
        }
        
        // Load time series data and update chart
        function loadTimeSeriesData() {
            fetch('/api/v1/memory?format=timeseries')
                .then(response => {
                    if (!response.ok) {
                        throw new Error('Network response was not ok: ' + response.status);
                    }
                    return response.json();
                })
                .then(data => {
                    // Reset connection errors on successful response
                    resetConnectionErrors();
                    
                    timeSeriesData = data;
                    updateChart();
                })
                .catch(error => {
                    handleConnectionError(error);
                });
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
		s.respondWithError(w, http.StatusMethodNotAllowed, "Method not allowed",
			errors.NewError(errors.ErrCodeInvalidRequest, "Only GET and POST methods are allowed for this endpoint"))
		return
	}

	// Process interval changes
	intervalStr := r.URL.Query().Get("interval")
	if intervalStr != "" {
		interval, err := strconv.Atoi(intervalStr)
		if err != nil || interval < 1 {
			s.respondWithError(w, http.StatusBadRequest, "Invalid interval value",
				errors.NewError(errors.ErrCodeInvalidRequest,
					fmt.Sprintf("Invalid interval value: %s. Must be a positive integer.", intervalStr)))
			return
		}

		s.memoryMonitor.SetInterval(time.Duration(interval) * time.Second)
	}

	// Process history length changes
	historyStr := r.URL.Query().Get("history_length")
	if historyStr != "" {
		historyLen, err := strconv.Atoi(historyStr)
		if err != nil || historyLen < 1 {
			s.respondWithError(w, http.StatusBadRequest, "Invalid history length",
				errors.NewError(errors.ErrCodeInvalidRequest,
					fmt.Sprintf("Invalid history length: %s. Must be a positive integer.", historyStr)))
			return
		}

		s.memoryMonitor.SetHistoryLength(historyLen)
	}

	s.respondWithJSON(w, http.StatusOK, map[string]string{
		"status":  "ok",
		"message": "Memory monitor configuration updated",
	})
}

func formatUptime(d time.Duration) string {
	// Constants for time unit calculations
	const (
		second = 1
		minute = 60 * second
		hour   = 60 * minute
		day    = 24 * hour
		week   = 7 * day
		month  = 30 * day  // approximate
		year   = 365 * day // approximate
	)

	// Convert duration to seconds
	totalSeconds := int64(d.Seconds())

	// Calculate time components
	years := totalSeconds / int64(year)
	totalSeconds %= int64(year)

	months := totalSeconds / int64(month)
	totalSeconds %= int64(month)

	weeks := totalSeconds / int64(week)
	totalSeconds %= int64(week)

	days := totalSeconds / int64(day)
	totalSeconds %= int64(day)

	hours := totalSeconds / int64(hour)
	totalSeconds %= int64(hour)

	minutes := totalSeconds / int64(minute)
	totalSeconds %= int64(minute)

	seconds := totalSeconds

	// Build the formatted string, omitting zero values
	var parts []string

	if years > 0 {
		if years == 1 {
			parts = append(parts, "1 year")
		} else {
			parts = append(parts, fmt.Sprintf("%d years", years))
		}
	}

	if months > 0 {
		if months == 1 {
			parts = append(parts, "1 month")
		} else {
			parts = append(parts, fmt.Sprintf("%d months", months))
		}
	}

	if weeks > 0 {
		if weeks == 1 {
			parts = append(parts, "1 week")
		} else {
			parts = append(parts, fmt.Sprintf("%d weeks", weeks))
		}
	}

	if days > 0 {
		if days == 1 {
			parts = append(parts, "1 day")
		} else {
			parts = append(parts, fmt.Sprintf("%d days", days))
		}
	}

	if hours > 0 {
		if hours == 1 {
			parts = append(parts, "1 hour")
		} else {
			parts = append(parts, fmt.Sprintf("%d hours", hours))
		}
	}

	if minutes > 0 {
		if minutes == 1 {
			parts = append(parts, "1 minute")
		} else {
			parts = append(parts, fmt.Sprintf("%d minutes", minutes))
		}
	}

	// Always include seconds if no other parts, or if seconds > 0
	if seconds > 0 || len(parts) == 0 {
		if seconds == 1 {
			parts = append(parts, "1 second")
		} else {
			parts = append(parts, fmt.Sprintf("%d seconds", seconds))
		}
	}

	// Join all parts with commas
	return strings.Join(parts, ", ")
}
