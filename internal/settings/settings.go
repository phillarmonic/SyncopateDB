// Add this to internal/settings/settings.go

package settings

import (
	"errors"
	"os"
	"strconv"
	"strings"
)

type LogLevel string

const (
	LogLevelDebug LogLevel = "debug"
	LogLevelInfo  LogLevel = "info"
	LogLevelWarn  LogLevel = "warn"
	LogLevelError LogLevel = "error"
)

func (l LogLevel) IsValid() bool {
	switch strings.ToLower(string(l)) {
	case string(LogLevelDebug), string(LogLevelInfo), string(LogLevelWarn), string(LogLevelError):
		return true
	default:
		return false
	}
}

type Configuration struct {
	Port           int      `json:"port"`
	Debug          bool     `json:"debug"`
	LogLevel       LogLevel `json:"log_level"`
	EnableWAL      bool     `json:"enable_wal"`
	EnableZSTD     bool     `json:"enable_zstd"`      // For database compression
	EnableHTTPZSTD bool     `json:"enable_http_zstd"` // New setting for HTTP compression
	ColorizedLogs  bool     `json:"colorized_logs"`   // Setting for colored logs
	ServerStarted  bool     `json:"server_started"`   // Setting for server-started status
	IgnoreLogPaths string   `json:"ignore_log_paths"` // Comma-separated list of paths to ignore in access logs
}

var Config Configuration

func (c *Configuration) Validate() error {
	if !c.LogLevel.IsValid() {
		return errors.New("invalid log_level")
	}
	return nil
}

func loadEnvInt(key string, defaultVal int) int {
	if val := os.Getenv(key); val != "" {
		if i, err := strconv.Atoi(val); err == nil {
			return i
		}
	}
	return defaultVal
}

func loadEnvBool(key string, defaultVal bool) bool {
	if val := os.Getenv(key); val != "" {
		if b, err := strconv.ParseBool(val); err == nil {
			return b
		}
	}
	return defaultVal
}

func loadEnvString(key string, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

func SetServerStarted(status bool) {
	Config.ServerStarted = status
}

func init() {
	logLevel := LogLevel(loadEnvString("LOG_LEVEL", string(LogLevelInfo)))

	Config = Configuration{
		Port:           loadEnvInt("PORT", 8080),
		Debug:          loadEnvBool("DEBUG", false),
		LogLevel:       logLevel,
		EnableWAL:      loadEnvBool("ENABLE_WAL", true),
		EnableZSTD:     loadEnvBool("ENABLE_ZSTD", false),
		EnableHTTPZSTD: loadEnvBool("ENABLE_HTTP_ZSTD", false),
		ColorizedLogs:  loadEnvBool("COLORIZED_LOGS", true),
		ServerStarted:  false,
		IgnoreLogPaths: loadEnvString("IGNORE_LOG_PATHS", "/api/v1/memory,/api/v1/memory/visualization,/health"),
	}

	if err := Config.Validate(); err != nil {
		panic(err)
	}
}
