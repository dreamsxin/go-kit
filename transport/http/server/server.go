// Package server provides an HTTP transport server that bridges the
// framework's Endpoint abstraction to the standard net/http package.
//
// Lifecycle of a single request:
//
//  1. ServerBefore hooks run — populate context from the HTTP request.
//  2. DecodeRequestFunc decodes the HTTP body into a domain request value.
//  3. The Endpoint is called with the decoded request.
//  4. ServerAfter hooks run — inspect or modify the response writer.
//  5. EncodeResponseFunc writes the domain response to the HTTP response.
//
// If any step returns an error, the ErrorEncoder writes an error response
// and the ErrorHandler logs or records the error.  Finalizer hooks always
// run at the very end, regardless of success or failure.
package server

import (
	"net/http"

	"github.com/dreamsxin/go-kit/endpoint"
	"github.com/dreamsxin/go-kit/transport"
)

// Server wraps an Endpoint and implements http.Handler.
// Use NewServer to construct one.
type Server struct {
	e            endpoint.Endpoint
	dec          DecodeRequestFunc
	enc          EncodeResponseFunc
	before       []RequestFunc
	after        []ResponseFunc
	errorEncoder transport.ErrorEncoder
	finalizer    []FinalizerFunc
	errorHandler transport.ErrorHandler
}

// NewServer constructs an HTTP Server for the given Endpoint.
//
// e, dec, and enc are required; passing nil panics.
// Use ServerBefore, ServerAfter, ServerErrorEncoder, ServerErrorHandler, and
// ServerFinalizer to customise behaviour.
func NewServer(
	e endpoint.Endpoint,
	dec DecodeRequestFunc,
	enc EncodeResponseFunc,
	options ...ServerOption,
) *Server {
	if e == nil || dec == nil || enc == nil {
		panic("essential parameters cannot be nil")
	}
	s := &Server{
		e:            e,
		dec:          dec,
		enc:          enc,
		errorEncoder: transport.DefaultErrorEncoder,
	}
	for _, option := range options {
		option(s)
	}
	if s.errorHandler == nil {
		s.errorHandler = transport.NewLogErrorHandler(nil)
	}
	return s
}

func (s Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	iw := &InterceptingWriter{w, http.StatusOK, 0}
	if len(s.finalizer) > 0 {
		defer func() {
			for _, f := range s.finalizer {
				f(ctx, r, iw)
			}
		}()
		//w = iw.reimplementInterfaces()
	}

	for _, f := range s.before {
		ctx = f(ctx, r)
	}

	request, err := s.dec(ctx, r)
	if err != nil {
		s.errorHandler.Handle(ctx, err)
		s.errorEncoder(ctx, err, iw)
		return
	}

	response, err := s.e(ctx, request)
	if err != nil {
		s.errorHandler.Handle(ctx, err)
		s.errorEncoder(ctx, err, iw)
		return
	}

	for _, f := range s.after {
		ctx = f(ctx, r, iw)
	}

	if err := s.enc(ctx, iw, response); err != nil {
		s.errorHandler.Handle(ctx, err)
		s.errorEncoder(ctx, err, iw)
		return
	}
}
