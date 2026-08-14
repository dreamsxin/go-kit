package transport

import (
	"context"

	"github.com/dreamsxin/go-kit/v2/log"
)

// NopErrorHandler is an ErrorHandler that discards all errors silently.
var NopErrorHandler ErrorHandler = ErrorHandlerFunc(func(_ context.Context, _ error) {})

type ErrorHandler interface {
	Handle(ctx context.Context, err error)
}

type LogErrorHandler struct {
	logger *log.Logger
}

func NewLogErrorHandler(logger *log.Logger) *LogErrorHandler {
	if logger == nil {
		logger = log.NewNopLogger()
	}
	return &LogErrorHandler{
		logger: logger,
	}
}

func (h *LogErrorHandler) Handle(ctx context.Context, err error) {
	h.logger.Sugar().Errorln("err", err)
}

type ErrorHandlerFunc func(ctx context.Context, err error)

func (f ErrorHandlerFunc) Handle(ctx context.Context, err error) {
	f(ctx, err)
}
