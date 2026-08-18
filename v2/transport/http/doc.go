// Package http provides the protocol-neutral HTTP contracts shared by the
// HTTP server and client transports: response-shaping interfaces such as
// StatusCoder, Headerer, and ErrorCoder, request-context population, and the
// reflection-based query and path parameter encoders and decoders.
//
// Endpoint adaptation lives in the subpackages: use
// [github.com/dreamsxin/go-kit/v2/transport/http/server] to serve endpoints
// over HTTP and [github.com/dreamsxin/go-kit/v2/transport/http/client] to
// invoke remote endpoints. This package owns only the contracts both sides
// share.
package http
