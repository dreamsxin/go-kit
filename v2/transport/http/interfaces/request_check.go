package interfaces

import "net/http"

// StatusCoder may be implemented by a response or error value to override
// the default HTTP status code (200 for responses, 500 for errors).
type StatusCoder interface {
	StatusCode() int
}

// Headerer may be implemented by a response or error value to add extra
// HTTP headers to the response.
type Headerer interface {
	Headers() http.Header
}

// ErrorCoder may be implemented by an error value to set a stable
// machine-readable application error code in JSON error responses.
type ErrorCoder interface {
	ErrorCode() string
}

// PublicMessager may be implemented by an error value to override the message
// exposed to clients in JSON error responses.
type PublicMessager interface {
	PublicMessage() string
}
