package transport

import (
	"context"

	"github.com/dreamsxin/go-kit/v2/endpoint"
)

// Binding binds one business endpoint to both HTTP and gRPC transports.
// The domain request and response types are defined once; each protocol owns
// its wire codec, so the same service method serves both without duplicated
// assembly.
//
// Domain request/response types stay protocol-neutral. The protocol codecs
// map between them and the wire types (JSON body, protobuf message):
//
//	binding := transport.Binding[HelloRequest, HelloResponse]{
//	    Endpoint: sayHello,
//	}
//	// HTTP: JSON decode is generated from the typed endpoint.
//	kit.HandleJSONTyped(httpComponent, "POST /hello", binding.TypedEndpoint())
//	// gRPC: register the protobuf-facing codec.
//	pb.RegisterGreeterServer(grpcComponent.Server(), binding.GRPCServer(protoToHello, helloToProto))
//
// A binding is a value; copying it is cheap and shares nothing mutable.
type Binding[Req, Resp any] struct {
	// Endpoint is the business endpoint, already middleware-built if needed.
	Endpoint endpoint.Endpoint
}

// TypedEndpoint returns the typed endpoint for HTTP transports that accept
// concrete request and response types (kit.HandleJSONTyped and the typed
// JSON servers).
func (b Binding[Req, Resp]) TypedEndpoint() endpoint.TypedEndpoint[Req, Resp] {
	return endpoint.Unwrap[Req, Resp](b.Endpoint)
}

// GRPCServer builds a gRPC server handler from the binding. decode maps the
// incoming protobuf message to the domain request; encode maps the domain
// response back to the protobuf message.
//
// The returned handler carries no transport options of its own: error encoding,
// interceptors, and the rest belong to the gRPC server the handler is
// registered on. Wrap Endpoint with middleware before building the binding for
// anything that is per-method.
func (b Binding[Req, Resp]) GRPCServer(
	decode func(ctx context.Context, pbMessage any) (Req, error),
	encode func(ctx context.Context, resp Resp) (any, error),
) *grpcBindingServer[Req, Resp] {
	return &grpcBindingServer[Req, Resp]{
		endpoint: b.Endpoint,
		decode:   decode,
		encode:   encode,
	}
}

// grpcBindingServer is the gRPC-facing adapter produced by Binding.GRPCServer.
// It implements the integrations/grpc/server.Handler contract shape without
// importing the module, so the core transport package stays provider-neutral.
type grpcBindingServer[Req, Resp any] struct {
	endpoint endpoint.Endpoint
	decode   func(context.Context, any) (Req, error)
	encode   func(context.Context, Resp) (any, error)
}

// ServeGRPC satisfies the integrations/grpc/server.Handler shape: it decodes
// the protobuf message, runs the endpoint, and encodes the response.
func (s *grpcBindingServer[Req, Resp]) ServeGRPC(ctx context.Context, request any) (context.Context, any, error) {
	req, err := s.decode(ctx, request)
	if err != nil {
		return ctx, nil, err
	}
	resp, err := s.endpoint(ctx, req)
	if err != nil {
		return ctx, nil, err
	}
	domain, ok := resp.(Resp)
	if !ok {
		return ctx, nil, endpoint.NewTypeAssertError[Resp](resp)
	}
	out, err := s.encode(ctx, domain)
	if err != nil {
		return ctx, nil, err
	}
	return ctx, out, nil
}
