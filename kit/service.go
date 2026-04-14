package kit

import (
	"context"
	"fmt"
	"net/http"

	"google.golang.org/grpc"

	"github.com/dreamsxin/go-kit/endpoint"
	kitlog "github.com/dreamsxin/go-kit/log"
)

// Service is a ready-to-run HTTP + gRPC microservice.
// Create one with New, register handlers with Handle/GRPC, then call Run.
type Service struct {
	addr       string
	mux        *http.ServeMux
	middleware []endpoint.Middleware
	logger     *kitlog.Logger
	metrics    *endpoint.Metrics
	srv        *http.Server

	grpcAddr   string
	grpcServer *grpc.Server
	grpcOpts   []grpc.ServerOption
}

// Option configures a Service.
type Option func(*Service)

// New creates a Service listening on addr (for example ":8080").
func New(addr string, opts ...Option) *Service {
	logger, _ := kitlog.NewDevelopment()
	s := &Service{
		addr:   addr,
		mux:    http.NewServeMux(),
		logger: logger,
	}
	for _, o := range opts {
		o(s)
	}
	s.registerHealthEndpoint()
	return s
}

func (s *Service) registerHealthEndpoint() {
	s.mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if s.metrics != nil {
			fmt.Fprintf(w, `{"status":"ok","requests":%d}`, s.metrics.RequestCount)
			return
		}
		fmt.Fprint(w, `{"status":"ok"}`)
	})
}

type httpRequestKey struct{}

func requestFromContext(ctx context.Context) *http.Request {
	r, _ := ctx.Value(httpRequestKey{}).(*http.Request)
	return r
}
