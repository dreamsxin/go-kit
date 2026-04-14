// Package server provides a gRPC transport server that bridges the
// framework's Endpoint abstraction to the gRPC protocol.
//
// Lifecycle of a single RPC call:
//
//  1. ServerBefore hooks run — extract metadata into the context.
//  2. DecodeRequestFunc decodes the proto request into a domain value.
//  3. The Endpoint is called with the decoded request.
//  4. ServerAfter hooks run — set response metadata headers/trailers.
//  5. EncodeResponseFunc encodes the domain response into a proto.
//
// If any step returns an error, the ErrorHandler is called and the error
// is returned to the gRPC caller.  Finalizer hooks always run last.
package server

import (
	"context"
	"fmt"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"

	"github.com/dreamsxin/go-kit/endpoint"
	"github.com/dreamsxin/go-kit/transport"
)

// Handler is the gRPC server-side interface implemented by Server.
// Register it with a *grpc.Server using the generated pb.Register* function.
type Handler interface {
	ServeGRPC(ctx context.Context, request interface{}) (context.Context, interface{}, error)
}

// Server wraps an Endpoint and implements Handler.
// Use NewServer to construct one.
type Server struct {
	e            endpoint.Endpoint
	dec          DecodeRequestFunc
	enc          EncodeResponseFunc
	before       []RequestFunc
	after        []ResponseFunc
	finalizer    []FinalizerFunc
	errorHandler transport.ErrorHandler
}

// NewServer constructs a gRPC Server for the given Endpoint.
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
		errorHandler: transport.NewLogErrorHandler(nil),
	}
	for _, option := range options {
		option(s)
	}
	return s
}

func (s Server) ServeGRPC(ctx context.Context, req interface{}) (retctx context.Context, resp interface{}, err error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		md = metadata.MD{}
	}

	if len(s.finalizer) > 0 {
		defer func() {
			for _, f := range s.finalizer {
				f(ctx, err)
			}
		}()
	}

	for _, f := range s.before {
		ctx = f(ctx, md)
	}

	var (
		request  interface{}
		response interface{}
		grpcResp interface{}
	)

	request, err = s.dec(ctx, req)
	if err != nil {
		s.handleError(ctx, err)
		return ctx, nil, err
	}

	response, err = s.e(ctx, request)
	if err != nil {
		s.handleError(ctx, err)
		return ctx, nil, err
	}

	var mdHeader, mdTrailer metadata.MD
	for _, f := range s.after {
		ctx = f(ctx, &mdHeader, &mdTrailer)
	}

	grpcResp, err = s.enc(ctx, response)
	if err != nil {
		s.handleError(ctx, err)
		return ctx, nil, err
	}

	if len(mdHeader) > 0 {
		if err = grpc.SendHeader(ctx, mdHeader); err != nil {
			s.handleError(ctx, err)
			return ctx, nil, err
		}
	}

	if len(mdTrailer) > 0 {
		if err = grpc.SetTrailer(ctx, mdTrailer); err != nil {
			s.handleError(ctx, err)
			return ctx, nil, err
		}
	}

	return ctx, grpcResp, nil
}

func (s Server) handleError(ctx context.Context, err error) {
	if s.errorHandler == nil {
		panic(fmt.Sprintf("grpc server error handler is nil: %v", err))
	}
	s.errorHandler.Handle(ctx, err)
}
