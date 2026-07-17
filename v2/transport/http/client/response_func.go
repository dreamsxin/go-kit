package client

import (
	"context"
	"net/http"
)

// ResponseFunc is called after a successful HTTP response is received.
// Use it to read response headers or propagate values into the context.
type ResponseFunc func(context.Context, *http.Response, error) context.Context
