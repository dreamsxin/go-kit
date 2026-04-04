package transport

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/dreamsxin/go-kit/log"
	"github.com/dreamsxin/go-kit/transport/http/interfaces"
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

// ErrorEncoder writes an error response to the HTTP response writer.
// It is called whenever a decode, endpoint, or encode step returns an error.
type ErrorEncoder func(ctx context.Context, err error, w http.ResponseWriter)

// DefaultErrorEncoder writes a plain-text error body with status 500.
// If the error implements json.Marshaler, the body is JSON-encoded instead.
// If the error implements interfaces.StatusCoder, that status code is used.
// If the error implements interfaces.Headerer, those headers are added.
func DefaultErrorEncoder(_ context.Context, err error, w http.ResponseWriter) {
	contentType, body := "text/plain; charset=utf-8", []byte(err.Error())
	if marshaler, ok := err.(json.Marshaler); ok {
		if jsonBody, marshalErr := marshaler.MarshalJSON(); marshalErr == nil {
			contentType, body = "application/json; charset=utf-8", jsonBody
		}
	}
	w.Header().Set("Content-Type", contentType)
	if headerer, ok := err.(interfaces.Headerer); ok {
		for k, values := range headerer.Headers() {
			for _, v := range values {
				w.Header().Add(k, v)
			}
		}
	}
	code := http.StatusInternalServerError
	if sc, ok := err.(interfaces.StatusCoder); ok {
		code = sc.StatusCode()
	}
	w.WriteHeader(code)
	w.Write(body)
}
