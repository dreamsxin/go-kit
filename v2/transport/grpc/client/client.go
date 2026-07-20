// Package client provides a gRPC transport client that wraps a gRPC
// connection as an endpoint.Endpoint.
//
// Example:
//
//	conn, _ := grpc.NewClient("localhost:8081",
//	    grpc.WithTransportCredentials(insecure.NewCredentials()))
//
//	ep := grpcclient.NewClient(
//	    conn, "MyService", "CreateUser",
//	    encodeRequest, decodeResponse, &pb.CreateUserResponse{},
//	).Endpoint()
package client

import (
	"context"
	"fmt"
	"reflect"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"

	"github.com/dreamsxin/go-kit/v2/endpoint"
	transportgrpc "github.com/dreamsxin/go-kit/v2/transport/grpc"
)

type Client struct {
	client      *grpc.ClientConn
	serviceName string
	method      string
	enc         EncodeRequestFunc
	dec         DecodeResponseFunc
	grpcReply   any
	before      []RequestFunc
	after       []ResponseFunc
	finalizer   []FinalizerFunc
	replyType   reflect.Type
}

// NewClient constructs a gRPC client for a single RPC method.
//
// cc is the underlying gRPC connection.
// serviceName and method identify the RPC (e.g. "UserService", "CreateUser").
// grpcReply must be a pointer to the expected proto response type
// (e.g. &pb.CreateUserResponse{}) — it is used as the target for
// proto.Unmarshal.
func NewClient(
	cc *grpc.ClientConn,
	serviceName string,
	method string,
	enc EncodeRequestFunc,
	dec DecodeResponseFunc,
	grpcReply any,
	options ...ClientOption,
) *Client {
	if cc == nil || enc == nil || dec == nil || grpcReply == nil {
		panic("essential parameters cannot be nil")
	}
	replyType := reflect.TypeOf(grpcReply)
	if replyType.Kind() != reflect.Pointer || reflect.ValueOf(grpcReply).IsNil() {
		panic("grpcReply must be a non-nil pointer")
	}
	c := &Client{
		client:    cc,
		method:    fmt.Sprintf("/%s/%s", serviceName, method),
		enc:       enc,
		dec:       dec,
		grpcReply: grpcReply,
		before:    []RequestFunc{},
		after:     []ResponseFunc{},
		replyType: replyType,
	}
	for _, option := range options {
		option(c)
	}
	return c
}

func (c Client) Endpoint() endpoint.Endpoint {
	return func(ctx context.Context, request interface{}) (response interface{}, err error) {
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

		req, err := c.enc(ctx, request)
		if err != nil {
			return nil, err
		}

		md := metadata.MD{}
		if outgoing, ok := metadata.FromOutgoingContext(ctx); ok {
			md = outgoing.Copy()
		}
		for _, f := range c.before {
			ctx = f(ctx, &md)
		}
		ctx = metadata.NewOutgoingContext(ctx, md)

		var header, trailer metadata.MD
		grpcReply := reflect.New(c.replyType.Elem()).Interface()
		if err = c.client.Invoke(
			ctx, c.method, req, grpcReply, grpc.Header(&header),
			grpc.Trailer(&trailer),
		); err != nil {
			return nil, err
		}

		ctx = context.WithValue(ctx, transportgrpc.ContextKeyResponseHeaders, header)
		ctx = context.WithValue(ctx, transportgrpc.ContextKeyResponseTrailers, trailer)

		for _, f := range c.after {
			ctx = f(ctx, header, trailer)
		}

		response, err = c.dec(ctx, grpcReply)
		if err != nil {
			return nil, err
		}
		return response, nil
	}
}
