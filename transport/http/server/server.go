package server

import (
	"net/http"

	"github.com/dreamsxin/go-kit/endpoint"
)

// 包装服务端点，实现 http.Handler 接口
type Server struct {
	e            endpoint.Endpoint
	dec          DecodeRequestFunc
	enc          EncodeResponseFunc
	before       []RequestFunc
	after        []ResponseFunc
	errorEncoder ErrorEncoder
	finalizer    []ResponseFunc
	errorHandler ErrorHandler
}

// 创建服务端，封装端点
func NewServer(
	e endpoint.Endpoint,
	dec DecodeRequestFunc,
	enc EncodeResponseFunc,
	options ...ServerOption,
) *Server {
	s := &Server{
		e:            e,
		dec:          dec,
		enc:          enc,
		errorEncoder: DefaultErrorEncoder,
		// errorHandler: NewLogErrorHandler(zap.NewNop().Sugar()),
	}
	for _, option := range options {
		option(s)
	}
	return s
}

func (s Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	iw := &InterceptingWriter{w, http.StatusOK, 0}
	if len(s.finalizer) > 0 {
		defer func() {
			for _, f := range s.finalizer {
				ctx = f(ctx, r, iw)
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
		if s.errorHandler != nil {
			s.errorHandler.Handle(ctx, err)
		}
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
