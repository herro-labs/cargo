package cargo

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"runtime"
	"strings"
	"time"
)

// LogLevel represents logging levels
type LogLevel int

const (
	LogLevelDebug LogLevel = iota
	LogLevelInfo
	LogLevelWarn
	LogLevelError
)

// String returns string representation of log level
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

// LogFormat represents logging output formats
type LogFormat int

const (
	LogFormatText LogFormat = iota
	LogFormatJSON
)

// StructuredLogger interface for structured logging
type StructuredLogger interface {
	Debug(msg string, args ...interface{})
	Info(msg string, args ...interface{})
	Warn(msg string, args ...interface{})
	Error(msg string, args ...interface{})
	With(args ...interface{}) StructuredLogger
	WithContext(ctx context.Context) StructuredLogger
}

// CargoLogger implements structured logging for Cargo
type CargoLogger struct {
	level  LogLevel
	format LogFormat
	output io.Writer
	fields map[string]interface{}
	slog   *slog.Logger
}

// LogEntry represents a single log entry
type LogEntry struct {
	Timestamp string                 `json:"timestamp"`
	Level     string                 `json:"level"`
	Message   string                 `json:"message"`
	Fields    map[string]interface{} `json:"fields,omitempty"`
	Caller    string                 `json:"caller,omitempty"`
	RequestID string                 `json:"request_id,omitempty"`
	UserID    string                 `json:"user_id,omitempty"`
	Method    string                 `json:"method,omitempty"`
	Duration  *time.Duration         `json:"duration,omitempty"`
}

// NewLogger creates a new structured logger
func NewLogger(config LoggingConfig) StructuredLogger {
	var output io.Writer = os.Stdout

	// Configure output
	switch strings.ToLower(config.Output) {
	case "stderr":
		output = os.Stderr
	case "stdout", "":
		output = os.Stdout
	default:
		// Assume it's a file path
		if file, err := os.OpenFile(config.Output, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666); err == nil {
			output = file
		}
	}

	// Parse log level
	level := parseLogLevel(config.Level)

	// Parse format
	format := parseLogFormat(config.Format)

	// Create slog logger for structured logging
	var handler slog.Handler
	if format == LogFormatJSON {
		handler = slog.NewJSONHandler(output, &slog.HandlerOptions{
			Level: slogLevel(level),
		})
	} else {
		handler = slog.NewTextHandler(output, &slog.HandlerOptions{
			Level: slogLevel(level),
		})
	}

	return &CargoLogger{
		level:  level,
		format: format,
		output: output,
		fields: make(map[string]interface{}),
		slog:   slog.New(handler),
	}
}

// parseLogLevel parses string log level to LogLevel
func parseLogLevel(level string) LogLevel {
	switch strings.ToLower(level) {
	case "debug":
		return LogLevelDebug
	case "info", "":
		return LogLevelInfo
	case "warn", "warning":
		return LogLevelWarn
	case "error":
		return LogLevelError
	default:
		return LogLevelInfo
	}
}

// parseLogFormat parses string format to LogFormat
func parseLogFormat(format string) LogFormat {
	switch strings.ToLower(format) {
	case "json":
		return LogFormatJSON
	case "text", "":
		return LogFormatText
	default:
		return LogFormatText
	}
}

// slogLevel converts CargoLogger level to slog level
func slogLevel(level LogLevel) slog.Level {
	switch level {
	case LogLevelDebug:
		return slog.LevelDebug
	case LogLevelInfo:
		return slog.LevelInfo
	case LogLevelWarn:
		return slog.LevelWarn
	case LogLevelError:
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// Debug logs a debug message
func (l *CargoLogger) Debug(msg string, args ...interface{}) {
	l.log(LogLevelDebug, msg, args...)
}

// Info logs an info message
func (l *CargoLogger) Info(msg string, args ...interface{}) {
	l.log(LogLevelInfo, msg, args...)
}

// Warn logs a warning message
func (l *CargoLogger) Warn(msg string, args ...interface{}) {
	l.log(LogLevelWarn, msg, args...)
}

// Error logs an error message
func (l *CargoLogger) Error(msg string, args ...interface{}) {
	l.log(LogLevelError, msg, args...)
}

// With returns a new logger with additional fields
func (l *CargoLogger) With(args ...interface{}) StructuredLogger {
	newFields := make(map[string]interface{})

	// Copy existing fields
	for k, v := range l.fields {
		newFields[k] = v
	}

	// Add new fields (key-value pairs)
	for i := 0; i < len(args)-1; i += 2 {
		if key, ok := args[i].(string); ok {
			newFields[key] = args[i+1]
		}
	}

	return &CargoLogger{
		level:  l.level,
		format: l.format,
		output: l.output,
		fields: newFields,
		slog:   l.slog,
	}
}

// WithContext returns a logger with context information
func (l *CargoLogger) WithContext(ctx context.Context) StructuredLogger {
	newLogger := l

	// Extract context values
	if requestID := getRequestID(ctx); requestID != "" {
		newLogger = newLogger.With("request_id", requestID).(*CargoLogger)
	}

	if userID := getUserID(ctx); userID != "" {
		newLogger = newLogger.With("user_id", userID).(*CargoLogger)
	}

	if method := getMethod(ctx); method != "" {
		newLogger = newLogger.With("method", method).(*CargoLogger)
	}

	return newLogger
}

// log performs the actual logging
func (l *CargoLogger) log(level LogLevel, msg string, args ...interface{}) {
	if level < l.level {
		return
	}

	entry := LogEntry{
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Level:     level.String(),
		Message:   msg,
		Fields:    make(map[string]interface{}),
	}

	// Add caller information
	if _, file, line, ok := runtime.Caller(3); ok {
		entry.Caller = fmt.Sprintf("%s:%d", file, line)
	}

	// Copy logger fields
	for k, v := range l.fields {
		entry.Fields[k] = v
	}

	// Add args as key-value pairs
	for i := 0; i < len(args)-1; i += 2 {
		if key, ok := args[i].(string); ok {
			entry.Fields[key] = args[i+1]
		}
	}

	// Extract special fields
	if requestID, exists := entry.Fields["request_id"]; exists {
		if rid, ok := requestID.(string); ok {
			entry.RequestID = rid
			delete(entry.Fields, "request_id")
		}
	}

	if userID, exists := entry.Fields["user_id"]; exists {
		if uid, ok := userID.(string); ok {
			entry.UserID = uid
			delete(entry.Fields, "user_id")
		}
	}

	if method, exists := entry.Fields["method"]; exists {
		if m, ok := method.(string); ok {
			entry.Method = m
			delete(entry.Fields, "method")
		}
	}

	if duration, exists := entry.Fields["duration"]; exists {
		if d, ok := duration.(time.Duration); ok {
			entry.Duration = &d
			delete(entry.Fields, "duration")
		}
	}

	// Output the log entry
	l.writeEntry(entry)
}

// writeEntry writes the log entry to the output
func (l *CargoLogger) writeEntry(entry LogEntry) {
	switch l.format {
	case LogFormatJSON:
		l.writeJSONEntry(entry)
	default:
		l.writeTextEntry(entry)
	}
}

// writeJSONEntry writes the entry as JSON
func (l *CargoLogger) writeJSONEntry(entry LogEntry) {
	data, err := json.Marshal(entry)
	if err != nil {
		fmt.Fprintf(l.output, "Error marshaling log entry: %v\n", err)
		return
	}

	fmt.Fprintln(l.output, string(data))
}

// writeTextEntry writes the entry as human-readable text
func (l *CargoLogger) writeTextEntry(entry LogEntry) {
	var parts []string

	parts = append(parts, entry.Timestamp)
	parts = append(parts, fmt.Sprintf("[%s]", entry.Level))

	if entry.RequestID != "" {
		parts = append(parts, fmt.Sprintf("req:%s", entry.RequestID))
	}

	if entry.UserID != "" {
		parts = append(parts, fmt.Sprintf("user:%s", entry.UserID))
	}

	if entry.Method != "" {
		parts = append(parts, fmt.Sprintf("method:%s", entry.Method))
	}

	parts = append(parts, entry.Message)

	// Add fields
	for k, v := range entry.Fields {
		parts = append(parts, fmt.Sprintf("%s=%v", k, v))
	}

	if entry.Duration != nil {
		parts = append(parts, fmt.Sprintf("duration=%v", *entry.Duration))
	}

	fmt.Fprintln(l.output, strings.Join(parts, " "))
}

// Context helper functions
func getRequestID(ctx context.Context) string {
	if value := ctx.Value("request_id"); value != nil {
		if id, ok := value.(string); ok {
			return id
		}
	}
	return ""
}

func getUserID(ctx context.Context) string {
	if value := ctx.Value("user_id"); value != nil {
		if id, ok := value.(string); ok {
			return id
		}
	}
	return ""
}

func getMethod(ctx context.Context) string {
	if value := ctx.Value("method"); value != nil {
		if method, ok := value.(string); ok {
			return method
		}
	}
	return ""
}

// Global logger instance
var globalLogger StructuredLogger

// SetGlobalLogger sets the global logger instance
func SetGlobalLogger(logger StructuredLogger) {
	globalLogger = logger
}

// GetGlobalLogger returns the global logger instance
func GetGlobalLogger() StructuredLogger {
	if globalLogger == nil {
		globalLogger = NewLogger(LoggingConfig{
			Level:  "info",
			Format: "text",
			Output: "stdout",
		})
	}
	return globalLogger
}

// Convenience functions for global logger
func Debug(msg string, args ...interface{}) {
	GetGlobalLogger().Debug(msg, args...)
}

func Info(msg string, args ...interface{}) {
	GetGlobalLogger().Info(msg, args...)
}

func Warn(msg string, args ...interface{}) {
	GetGlobalLogger().Warn(msg, args...)
}

func Error(msg string, args ...interface{}) {
	GetGlobalLogger().Error(msg, args...)
}
