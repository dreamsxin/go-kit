package server

import (
	"context"
	"net/http"
)

// FinalizerFunc is called at the very end of every request, regardless of
// success or failure.  Use it to record latency, log access, or release
// resources.
type FinalizerFunc func(context.Context, *http.Request, *InterceptingWriter)
