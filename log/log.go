// Package log provides a thin wrapper around go.uber.org/zap.
//
// Logger is a type alias for *zap.Logger, so all zap methods are available
// directly.  Use NewDevelopment for local development (coloured, human-readable
// output) and zap.NewProduction for production (JSON output).
package log

import (
	"fmt"
	"strings"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Logger is an alias for *zap.Logger.  All framework components that accept
// a logger expect this type.
type Logger = zap.Logger

// NewDevelopment creates a development-mode logger with coloured, human-readable
// output and caller information.  Returns an error only if zap fails to
// initialise (extremely rare).
func NewDevelopment() (*Logger, error) {
	return zap.NewDevelopment()
}

// New creates a logger from a level and format.
// Supported formats are "json" and "console"; supported levels are zap's
// standard debug, info, warn, error, dpanic, panic, and fatal names.
func New(level, format string) (*Logger, error) {
	encoding := strings.ToLower(strings.TrimSpace(format))
	if encoding == "" {
		encoding = "json"
	}
	if encoding != "json" && encoding != "console" {
		return nil, fmt.Errorf("log: unsupported format %q", format)
	}

	var parsed zapcore.Level
	levelText := strings.ToLower(strings.TrimSpace(level))
	if levelText == "" {
		levelText = "info"
	}
	if err := parsed.UnmarshalText([]byte(levelText)); err != nil {
		return nil, fmt.Errorf("log: unsupported level %q: %w", level, err)
	}

	cfg := zap.NewProductionConfig()
	cfg.Encoding = encoding
	cfg.Level = zap.NewAtomicLevelAt(parsed)
	if encoding == "console" {
		cfg = zap.NewDevelopmentConfig()
		cfg.Level = zap.NewAtomicLevelAt(parsed)
	}
	return cfg.Build()
}
