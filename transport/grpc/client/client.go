package client

import (
	"context"
	"fmt"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"

	"github.com/dreamsxin/go-kit/endpoint"
	transportgrpc "github.com/dreamsxin/go-kit/transport/grpc"
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
}

// 创建 grpc 客户端
func NewClient(
	cc *grpc.ClientConn,
	serviceName string,
	method string,
	enc EncodeRequestFunc,
	dec DecodeResponseFunc,
	grpcReply any,
	options ...ClientOption,
) *Client {
	c := &Client{
		client:    cc,
		method:    fmt.Sprintf("/%s/%s", serviceName, method),
		enc:       enc,
		dec:       dec,
		grpcReply: grpcReply,
		before:    []RequestFunc{},
		after:     []ResponseFunc{},
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

		md := &metadata.MD{}
		for _, f := range c.before {
			ctx = f(ctx, md)
		}
		ctx = metadata.NewOutgoingContext(ctx, *md)

		var header, trailer metadata.MD
		if err = c.client.Invoke(
			ctx, c.method, req, c.grpcReply, grpc.Header(&header),
			grpc.Trailer(&trailer),
		); err != nil {
			return nil, err
		}

		for _, f := range c.after {
			ctx = f(ctx, header, trailer)
		}

		response, err = c.dec(ctx, c.grpcReply)
		if err != nil {
			return nil, err
		}
		return response, nil
	}
}
