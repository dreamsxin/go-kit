package kit

import (
	"context"
	"net/http"

	httpserver "github.com/dreamsxin/go-kit/transport/http/server"
)

// JSON creates a typed JSON http.Handler without needing a Service.
func JSON[Req any](handler func(ctx context.Context, req Req) (any, error)) http.Handler {
	return httpserver.NewJSONServer[Req](handler,
		httpserver.ServerErrorEncoder(httpserver.JSONErrorEncoder),
	)
}
