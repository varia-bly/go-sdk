package variably

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"
)

// LogLevel represents the logging level
type LogLevel int

const (
	DebugLevel LogLevel = iota
	InfoLevel
	WarnLevel
	ErrorLevel
)

func (l LogLevel) String() string {
	switch l {
	case DebugLevel:
		return "DEBUG"
	case InfoLevel:
		return "INFO"
	case WarnLevel:
		return "WARN"
	case ErrorLevel:
		return "ERROR"
	default:
		return "UNKNOWN"
	}
}

// DefaultLogger implements a basic logger with configurable output
type DefaultLogger struct {
	level  LogLevel
	format string
	logger *log.Logger
}

// NewDefaultLogger creates a new default logger
func NewDefaultLogger(config LogConfig) *DefaultLogger {
	level := parseLogLevel(config.Level)
	format := config.Format
	if format == "" {
		format = "text"
	}

	var logger *log.Logger
	switch config.Output {
	case "stderr":
		logger = log.New(os.Stderr, "", 0)
	case "file":
		// For simplicity, we'll use stdout if file is specified but no path given
		logger = log.New(os.Stdout, "", 0)
	default:
		logger = log.New(os.Stdout, "", 0)
	}

	return &DefaultLogger{
		level:  level,
		format: format,
		logger: logger,
	}
}

// Debug logs a debug message
func (l *DefaultLogger) Debug(msg string, fields ...interface{}) {
	if l.level <= DebugLevel {
		l.log(DebugLevel, msg, fields...)
	}
}

// Info logs an info message
func (l *DefaultLogger) Info(msg string, fields ...interface{}) {
	if l.level <= InfoLevel {
		l.log(InfoLevel, msg, fields...)
	}
}

// Warn logs a warning message
func (l *DefaultLogger) Warn(msg string, fields ...interface{}) {
	if l.level <= WarnLevel {
		l.log(WarnLevel, msg, fields...)
	}
}

// Error logs an error message
func (l *DefaultLogger) Error(msg string, fields ...interface{}) {
	if l.level <= ErrorLevel {
		l.log(ErrorLevel, msg, fields...)
	}
}

// log performs the actual logging
func (l *DefaultLogger) log(level LogLevel, msg string, fields ...interface{}) {
	timestamp := time.Now().UTC()

	if l.format == "json" {
		l.logJSON(level, msg, timestamp, fields...)
	} else {
		l.logText(level, msg, timestamp, fields...)
	}
}

// logJSON logs in JSON format
func (l *DefaultLogger) logJSON(level LogLevel, msg string, timestamp time.Time, fields ...interface{}) {
	logEntry := map[string]interface{}{
		"timestamp": timestamp.Format(time.RFC3339),
		"level":     level.String(),
		"message":   msg,
		"source":    "variably-sdk",
	}

	// Add fields as key-value pairs
	for i := 0; i < len(fields); i += 2 {
		if i+1 < len(fields) {
			key := fmt.Sprintf("%v", fields[i])
			value := fields[i+1]
			logEntry[key] = value
		}
	}

	jsonData, err := json.Marshal(logEntry)
	if err != nil {
		// Fallback to simple text if JSON marshaling fails
		l.logger.Printf("[%s] %s %s", level.String(), timestamp.Format(time.RFC3339), msg)
		return
	}

	l.logger.Println(string(jsonData))
}

// logText logs in text format
func (l *DefaultLogger) logText(level LogLevel, msg string, timestamp time.Time, fields ...interface{}) {
	// Build field string
	fieldStr := ""
	for i := 0; i < len(fields); i += 2 {
		if i+1 < len(fields) {
			key := fmt.Sprintf("%v", fields[i])
			value := fmt.Sprintf("%v", fields[i+1])
			fieldStr += fmt.Sprintf(" %s=%s", key, value)
		}
	}

	l.logger.Printf("[%s] %s %s%s",
		level.String(),
		timestamp.Format("2006-01-02 15:04:05"),
		msg,
		fieldStr)
}

// parseLogLevel parses a string log level
func parseLogLevel(level string) LogLevel {
	switch level {
	case "debug":
		return DebugLevel
	case "info":
		return InfoLevel
	case "warn":
		return WarnLevel
	case "error":
		return ErrorLevel
	default:
		return InfoLevel
	}
}

// NoOpLogger is a logger that does nothing (for testing or when logging is disabled)
type NoOpLogger struct{}

func (l *NoOpLogger) Debug(msg string, fields ...interface{}) {}
func (l *NoOpLogger) Info(msg string, fields ...interface{})  {}
func (l *NoOpLogger) Warn(msg string, fields ...interface{})  {}
func (l *NoOpLogger) Error(msg string, fields ...interface{}) {}

// NewNoOpLogger creates a new no-op logger
func NewNoOpLogger() *NoOpLogger {
	return &NoOpLogger{}
}