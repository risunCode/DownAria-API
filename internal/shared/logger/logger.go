package logger

import (
	"log/slog"
	"os"
)

var defaultLogger *slog.Logger

func init() {
	// JSON handler for structured logging
	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			// Rename "msg" to "message" for consistency
			if a.Key == slog.MessageKey {
				a.Key = "message"
			}
			// Rename "level" to "severity" for better log aggregation
			if a.Key == slog.LevelKey {
				a.Key = "severity"
			}
			return a
		},
	})
	defaultLogger = slog.New(handler)
}

// Default returns the default structured logger
func Default() *slog.Logger {
	return defaultLogger
}

// With creates a new logger with additional attributes
func With(args ...any) *slog.Logger {
	return defaultLogger.With(args...)
}

// Info logs at info level
func Info(msg string, args ...any) {
	defaultLogger.Info(msg, args...)
}

// Warn logs at warning level
func Warn(msg string, args ...any) {
	defaultLogger.Warn(msg, args...)
}

// Error logs at error level
func Error(msg string, args ...any) {
	defaultLogger.Error(msg, args...)
}

// Debug logs at debug level
func Debug(msg string, args ...any) {
	defaultLogger.Debug(msg, args...)
}
