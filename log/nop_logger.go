package log

import (
	"go.uber.org/zap"
)

func NewNopLogger() Logger { return *zap.NewNop() }
