// Package log provides a thin wrapper around go.uber.org/zap.
//
// Logger is a type alias for *zap.Logger, so all zap methods are available
// directly.  Use NewDevelopment for local development (coloured, human-readable
// output) and zap.NewProduction for production (JSON output).
package log

import (
	"go.uber.org/zap"
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
