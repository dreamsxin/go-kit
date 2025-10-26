package server

import (
	"net/http"

	"github.com/dreamsxin/go-kit/endpoint"
	"github.com/dreamsxin/go-kit/transport"
)

// 包装服务端点，实现 http.Handler 接口
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

// 创建服务端，封装端点
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
		ctx = f(ctx, nil, iw)
	}

	if err := s.enc(ctx, iw, response); err != nil {
		s.errorHandler.Handle(ctx, err)
		s.errorEncoder(ctx, err, iw)
		return
	}
}
