package log

import (
	"go.uber.org/zap"
)

type Logger = zap.Logger

func NewDevelopment() (*Logger, error) {
	return zap.NewDevelopment()
}
