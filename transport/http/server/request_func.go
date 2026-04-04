package server

import (
	"context"
	"net/http"
)

// RequestFunc is called before the request is decoded.  It receives the
// current context and the raw *http.Request, and returns a (possibly enriched)
// context.  Use it to extract headers, inject request IDs, etc.
type RequestFunc func(context.Context, *http.Request) context.Context
