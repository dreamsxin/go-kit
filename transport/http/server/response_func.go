package server

import (
	"context"
	"net/http"
)

// ResponseFunc is called after the Endpoint returns successfully, before the
// response is encoded.  Use it to set response headers or inspect the writer.
type ResponseFunc func(context.Context, *http.Request, *InterceptingWriter) context.Context
