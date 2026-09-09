package variably

import (
	"fmt"
	"log"
	"os"
)

// LogLevel represents the logging level
type LogLevel int

const (
	LogLevelDebug LogLevel = iota
	LogLevelInfo
	LogLevelWarn
	LogLevelError
)

// String returns the string representation of the log level
func (l LogLevel) String() string {
	switch l {
	case LogLevelDebug:
		return "DEBUG"
	case LogLevelInfo:
		return "INFO"
	case LogLevelWarn:
		return "WARN"
	case LogLevelError:
		return "ERROR"
	default:
		return "UNKNOWN"
	}
}

// Logger defines the logging interface used by the SDK
type Logger interface {
	Debug(message string, fields map[string]interface{})
	Info(message string, fields map[string]interface{})
	Warn(message string, fields map[string]interface{})
	Error(message string, fields map[string]interface{})
	SetLevel(level LogLevel)
}

// DefaultLogger is a simple logger implementation using Go's standard log package
type DefaultLogger struct {
	level  LogLevel
	logger *log.Logger
}

// NewDefaultLogger creates a new default logger
func NewDefaultLogger(level LogLevel) *DefaultLogger {
	return &DefaultLogger{
		level:  level,
		logger: log.New(os.Stdout, "[Variably SDK] ", log.LstdFlags),
	}
}

// Debug logs a debug message
func (l *DefaultLogger) Debug(message string, fields map[string]interface{}) {
	if l.level <= LogLevelDebug {
		l.logWithFields(LogLevelDebug, message, fields)
	}
}

// Info logs an info message
func (l *DefaultLogger) Info(message string, fields map[string]interface{}) {
	if l.level <= LogLevelInfo {
		l.logWithFields(LogLevelInfo, message, fields)
	}
}

// Warn logs a warning message
func (l *DefaultLogger) Warn(message string, fields map[string]interface{}) {
	if l.level <= LogLevelWarn {
		l.logWithFields(LogLevelWarn, message, fields)
	}
}

// Error logs an error message
func (l *DefaultLogger) Error(message string, fields map[string]interface{}) {
	if l.level <= LogLevelError {
		l.logWithFields(LogLevelError, message, fields)
	}
}

// SetLevel sets the logging level
func (l *DefaultLogger) SetLevel(level LogLevel) {
	l.level = level
}

// logWithFields logs a message with structured fields
func (l *DefaultLogger) logWithFields(level LogLevel, message string, fields map[string]interface{}) {
	logMessage := fmt.Sprintf("[%s] %s", level.String(), message)
	
	if fields != nil && len(fields) > 0 {
		logMessage += " "
		for key, value := range fields {
			logMessage += fmt.Sprintf("%s=%v ", key, value)
		}
	}
	
	l.logger.Println(logMessage)
}

// NoOpLogger is a logger that doesn't log anything
type NoOpLogger struct{}

// NewNoOpLogger creates a new no-op logger
func NewNoOpLogger() *NoOpLogger {
	return &NoOpLogger{}
}

// Debug does nothing
func (l *NoOpLogger) Debug(message string, fields map[string]interface{}) {}

// Info does nothing
func (l *NoOpLogger) Info(message string, fields map[string]interface{}) {}

// Warn does nothing
func (l *NoOpLogger) Warn(message string, fields map[string]interface{}) {}

// Error does nothing
func (l *NoOpLogger) Error(message string, fields map[string]interface{}) {}

// SetLevel does nothing
func (l *NoOpLogger) SetLevel(level LogLevel) {}