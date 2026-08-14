package http

import "net/http"

// StatusCoder overrides the default HTTP response status.
type StatusCoder interface {
	StatusCode() int
}

// Headerer adds HTTP headers to a request or response.
type Headerer interface {
	Headers() http.Header
}

// ErrorCoder provides a stable machine-readable application error code.
type ErrorCoder interface {
	ErrorCode() string
}

// PublicMessager provides the error message that may be exposed to clients.
type PublicMessager interface {
	PublicMessage() string
}
