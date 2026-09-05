package grpc

import (
	"context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"

	"github.com/dreamsxin/go-kit/v2/endpoint"
)

// TraceparentKey is the gRPC metadata key carrying the W3C trace context. It
// is the header name from the specification: metadata keys are lowercase, and
// so is traceparent, so the same value crosses an HTTP hop and a gRPC hop
// unchanged.
const TraceparentKey = "traceparent"

// ExtractTraceparent reads a W3C traceparent from incoming metadata into the
// context, where endpoint.TracingMiddleware continues the trace and
// slogadapter.LoggingMiddleware logs its trace_id.
//
// It is a server RequestFunc, so a hand-assembled server installs it with
// server.ServerBefore(transportgrpc.ExtractTraceparent). A kit/grpc component
// extracts without it: the equivalent interceptors are installed by default.
func ExtractTraceparent(ctx context.Context, md metadata.MD) context.Context {
	values := md.Get(TraceparentKey)
	if len(values) == 0 {
		return ctx
	}
	traceContext, ok := endpoint.ParseTraceparent(values[0])
	if !ok {
		// A malformed traceparent is dropped rather than repaired: continuing
		// a trace that does not exist would attach this service's spans to an
		// invented parent.
		return ctx
	}
	return endpoint.WithTraceContext(ctx, traceContext)
}

// InjectTraceparent writes the context's W3C trace context into outgoing
// metadata, so the next service continues this trace instead of starting its
// own.
//
// It is a client RequestFunc. client.NewClient installs it by default, so a
// client only needs it when its before hooks were replaced.
func InjectTraceparent(ctx context.Context, md *metadata.MD) context.Context {
	if md == nil {
		return ctx
	}
	traceContext := endpoint.TraceContextFromContext(ctx)
	if !traceContext.Valid() {
		return ctx
	}
	md.Set(TraceparentKey, traceContext.String())
	return ctx
}

// TraceparentUnaryServerInterceptor extracts the incoming W3C trace context
// into the handler's context. kit/grpc installs it by default; install it
// yourself on a server assembled with grpc.NewServer directly.
func TraceparentUnaryServerInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		return handler(withIncomingTraceparent(ctx), req)
	}
}

// TraceparentStreamServerInterceptor is the streaming counterpart of
// TraceparentUnaryServerInterceptor. A stream carries its context inside the
// ServerStream, so the trace context reaches the handler through a stream
// wrapper.
func TraceparentStreamServerInterceptor() grpc.StreamServerInterceptor {
	return func(srv any, stream grpc.ServerStream, _ *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		ctx := withIncomingTraceparent(stream.Context())
		if ctx == stream.Context() {
			return handler(srv, stream)
		}
		return handler(srv, tracedServerStream{ServerStream: stream, ctx: ctx})
	}
}

// TraceparentUnaryClientInterceptor writes the context's W3C trace context
// into outgoing metadata. Use it for a connection whose calls are made by
// generated stubs rather than through client.Client, which injects on its own.
func TraceparentUnaryClientInterceptor() grpc.UnaryClientInterceptor {
	return func(
		ctx context.Context,
		method string,
		req, reply any,
		cc *grpc.ClientConn,
		invoker grpc.UnaryInvoker,
		opts ...grpc.CallOption,
	) error {
		traceContext := endpoint.TraceContextFromContext(ctx)
		if traceContext.Valid() {
			md, _ := metadata.FromOutgoingContext(ctx)
			md = md.Copy()
			md.Set(TraceparentKey, traceContext.String())
			ctx = metadata.NewOutgoingContext(ctx, md)
		}
		return invoker(ctx, method, req, reply, cc, opts...)
	}
}

func withIncomingTraceparent(ctx context.Context) context.Context {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return ctx
	}
	return ExtractTraceparent(ctx, md)
}

// tracedServerStream carries a context the interceptor derived, which is the
// only way a stream handler can see it.
type tracedServerStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (s tracedServerStream) Context() context.Context { return s.ctx }
