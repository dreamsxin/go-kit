package client

import (
	"context"
	"fmt"
	"reflect"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/proto"

	transportgrpc "github.com/dreamsxin/go-kit/v2/integrations/grpc"
)

// TypedClient is one gRPC method whose request and response proto types are
// carried by the Go type system: Invoke takes the request message and returns
// the response message, so a caller never touches endpoint.Endpoint's any,
// and never wraps the result with endpoint.Unwrap. It embeds Client, so the
// option surface — ClientBefore, ClientAfter, ClientFinalizer — and the
// default traceparent injection are exactly the untyped client's.
//
// Req and Resp are the proto message pointer types themselves (for example
// *pb.CreateUserRequest and *pb.CreateUserResponse), the same values grpc's
// Invoke marshals; there is no encode/decode pair, because there is nothing
// between the caller and the wire to convert. The response message is
// allocated per call, and a transport failure comes back through
// transportgrpc.ClassifyError so retry, breaker and encoders classify it the
// same way they classify an HTTP failure.
type TypedClient[Req, Resp proto.Message] struct {
	*Client
	replyElem reflect.Type
}

// NewTypedClient constructs a typed gRPC client for a single RPC method.
// Resp must be a pointer type, because the wire needs a message to unmarshal
// into; the client allocates one per call.
func NewTypedClient[Req, Resp proto.Message](
	cc *grpc.ClientConn,
	serviceName string,
	method string,
	options ...ClientOption,
) *TypedClient[Req, Resp] {
	if cc == nil {
		panic("essential parameters cannot be nil")
	}
	var zero Resp
	replyType := reflect.TypeOf(zero)
	if replyType == nil || replyType.Kind() != reflect.Pointer {
		panic("grpcclient: Resp must be a pointer to a proto message")
	}
	c := &TypedClient[Req, Resp]{
		Client: &Client{
			client: cc,
			method: fmt.Sprintf("/%s/%s", serviceName, method),
			// Trace context propagates by default: a client that had to
			// opt in would silently break a trace at the first hop
			// somebody forgot. ClientBefore hooks run after this one and
			// can overwrite the key.
			before: []RequestFunc{transportgrpc.InjectTraceparent},
			after:  []ResponseFunc{},
		},
		replyElem: replyType.Elem(),
	}
	for _, option := range options {
		option(c.Client)
	}
	return c
}

// Invoke performs the RPC and returns the decoded response message.
//
// Stable: grpc.typed-client — Invoke sends the caller's request message on
// the method the client was built for and returns a freshly allocated Resp,
// classifying transport failures the same way the untyped client does;
// ClientOption hooks and the default traceparent injection apply.
// Covered by: TestTypedClientRoundTrip, TestTypedClientClassifiesTheStatus,
// TestTypedClientInjectsTraceparent
func (c *TypedClient[Req, Resp]) Invoke(ctx context.Context, request Req) (response Resp, err error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	if c.finalizer != nil {
		defer func() {
			for _, f := range c.finalizer {
				f(ctx, err)
			}
		}()
	}

	ctx = context.WithValue(ctx, transportgrpc.ContextKeyRequestMethod, c.method)

	md := metadata.MD{}
	if outgoing, ok := metadata.FromOutgoingContext(ctx); ok {
		md = outgoing.Copy()
	}
	for _, f := range c.before {
		ctx = f(ctx, &md)
	}
	ctx = metadata.NewOutgoingContext(ctx, md)

	var header, trailer metadata.MD
	reply, ok := reflect.New(c.replyElem).Interface().(Resp)
	if !ok {
		return response, fmt.Errorf("grpcclient: Resp does not implement proto.Message")
	}
	if err := c.client.Invoke(
		ctx, c.method, request, reply, grpc.Header(&header),
		grpc.Trailer(&trailer),
	); err != nil {
		return response, transportgrpc.ClassifyError(err)
	}

	ctx = context.WithValue(ctx, transportgrpc.ContextKeyResponseHeaders, header)
	ctx = context.WithValue(ctx, transportgrpc.ContextKeyResponseTrailers, trailer)

	for _, f := range c.after {
		ctx = f(ctx, header, trailer)
	}
	return reply, nil
}
