// Package log provides a deprecated standard-library logging facade.
// New code should accept *slog.Logger directly.
package log

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
)

// Logger wraps slog.Logger for source compatibility with earlier generated
// projects. Framework runtime packages use *slog.Logger directly.
//
// Deprecated: Use slog.Logger directly.
type Logger struct {
	logger *slog.Logger
}

// NewDevelopment creates a text logger at debug level.
//
// Deprecated: Construct an slog.Logger in the application entry point.
func NewDevelopment() (*Logger, error) {
	return &Logger{logger: slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))}, nil
}

// New creates a logger from a level and format.
// Supported formats are "json" and "console"; levels follow slog.
//
// Deprecated: Construct an slog.Logger in the application entry point.
func New(level, format string) (*Logger, error) {
	encoding := strings.ToLower(strings.TrimSpace(format))
	if encoding == "" {
		encoding = "json"
	}
	if encoding != "json" && encoding != "console" {
		return nil, fmt.Errorf("log: unsupported format %q", format)
	}

	var parsed slog.Level
	levelText := strings.ToLower(strings.TrimSpace(level))
	if levelText == "" {
		levelText = "info"
	}
	if err := parsed.UnmarshalText([]byte(levelText)); err != nil {
		return nil, fmt.Errorf("log: unsupported level %q: %w", level, err)
	}

	options := &slog.HandlerOptions{Level: parsed}
	var handler slog.Handler
	if encoding == "console" {
		handler = slog.NewTextHandler(os.Stderr, options)
	} else {
		handler = slog.NewJSONHandler(os.Stderr, options)
	}
	return &Logger{logger: slog.New(handler)}, nil
}

// NewNopLogger returns a logger that discards every record.
//
// Deprecated: Use slog.New(slog.DiscardHandler).
func NewNopLogger() *Logger {
	return &Logger{logger: slog.New(slog.DiscardHandler)}
}

// Slog returns the underlying standard-library logger.
func (l *Logger) Slog() *slog.Logger {
	if l == nil || l.logger == nil {
		return slog.New(slog.DiscardHandler)
	}
	return l.logger
}

// With returns a logger with additional attributes.
func (l *Logger) With(args ...any) *Logger {
	return &Logger{logger: l.Slog().With(args...)}
}

// Sugar returns the compatibility formatting facade.
func (l *Logger) Sugar() *SugaredLogger {
	return &SugaredLogger{logger: l.Slog()}
}

// Sync is retained for generated cleanup code; slog handlers are synchronous.
func (l *Logger) Sync() error { return nil }

// SugaredLogger supports historical generated logging calls.
//
// Deprecated: Use slog.Logger directly.
type SugaredLogger struct {
	logger *slog.Logger
}

func (l *SugaredLogger) Info(args ...any)  { l.logger.Info(fmt.Sprint(args...)) }
func (l *SugaredLogger) Warn(args ...any)  { l.logger.Warn(fmt.Sprint(args...)) }
func (l *SugaredLogger) Error(args ...any) { l.logger.Error(fmt.Sprint(args...)) }
func (l *SugaredLogger) Debug(args ...any) { l.logger.Debug(fmt.Sprint(args...)) }

func (l *SugaredLogger) Infof(format string, args ...any) {
	l.logger.Info(fmt.Sprintf(format, args...))
}
func (l *SugaredLogger) Warnf(format string, args ...any) {
	l.logger.Warn(fmt.Sprintf(format, args...))
}
func (l *SugaredLogger) Errorf(format string, args ...any) {
	l.logger.Error(fmt.Sprintf(format, args...))
}
func (l *SugaredLogger) Debugf(format string, args ...any) {
	l.logger.Debug(fmt.Sprintf(format, args...))
}
func (l *SugaredLogger) Fatalf(format string, args ...any) {
	message := fmt.Sprintf(format, args...)
	l.logger.ErrorContext(context.Background(), message)
	panic(message)
}

func (l *SugaredLogger) Infow(message string, args ...any)  { l.logger.Info(message, args...) }
func (l *SugaredLogger) Warnw(message string, args ...any)  { l.logger.Warn(message, args...) }
func (l *SugaredLogger) Errorw(message string, args ...any) { l.logger.Error(message, args...) }
func (l *SugaredLogger) Debugw(message string, args ...any) { l.logger.Debug(message, args...) }
