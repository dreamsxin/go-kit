package log

import (
	"go.uber.org/zap"
)

// NewNopLogger returns a no-op logger that discards all output.
// Use it in tests or when logging is not needed.
func NewNopLogger() *Logger { return zap.NewNop() }
