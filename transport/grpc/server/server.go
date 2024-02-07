package server

import (
	"context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"

	"github.com/dreamsxin/go-kit/endpoint"
	"github.com/dreamsxin/go-kit/transport"
)

type Handler interface {
	ServeGRPC(ctx context.Context, request interface{}) (context.Context, interface{}, error)
}

// 封装 Endpoint 实现 grpc.Handler 接口
type Server struct {
	e            endpoint.Endpoint
	dec          DecodeRequestFunc
	enc          EncodeResponseFunc
	before       []RequestFunc
	after        []ResponseFunc
	finalizer    []FinalizerFunc
	errorHandler transport.ErrorHandler
}

func NewServer(
	e endpoint.Endpoint,
	dec DecodeRequestFunc,
	enc EncodeResponseFunc,
	options ...ServerOption,
) *Server {
	s := &Server{
		e:   e,
		dec: dec,
		enc: enc,
		// errorHandler: NewLogErrorHandler(zap.NewNop().Sugar()),
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
		s.errorHandler.Handle(ctx, err)
		return ctx, nil, err
	}

	response, err = s.e(ctx, request)
	if err != nil {
		s.errorHandler.Handle(ctx, err)
		return ctx, nil, err
	}

	var mdHeader, mdTrailer metadata.MD
	for _, f := range s.after {
		ctx = f(ctx, &mdHeader, &mdTrailer)
	}

	grpcResp, err = s.enc(ctx, response)
	if err != nil {
		s.errorHandler.Handle(ctx, err)
		return ctx, nil, err
	}

	if len(mdHeader) > 0 {
		if err = grpc.SendHeader(ctx, mdHeader); err != nil {
			s.errorHandler.Handle(ctx, err)
			return ctx, nil, err
		}
	}

	if len(mdTrailer) > 0 {
		if err = grpc.SetTrailer(ctx, mdTrailer); err != nil {
			s.errorHandler.Handle(ctx, err)
			return ctx, nil, err
		}
	}

	return ctx, grpcResp, nil
}
