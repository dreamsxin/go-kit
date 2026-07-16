package kit

import (
	"context"
	"net/http"
	"time"

	"google.golang.org/grpc"

	"github.com/dreamsxin/go-kit/endpoint"
	kitlog "github.com/dreamsxin/go-kit/log"
)

// Service is a ready-to-run HTTP + gRPC microservice.
// Create one with New, register handlers with Handle/GRPC, then call Run.
type Service struct {
	addr             string
	mux              *http.ServeMux
	middleware       []endpoint.Middleware
	logger           *kitlog.Logger
	metrics          *endpoint.Metrics
	httpConfig       HTTPServerConfig
	requestID        bool
	jsonMaxBodyBytes int64
	healthTimeout    time.Duration
	livenessChecks   []namedHealthCheck
	readinessChecks  []namedHealthCheck
	srv              *http.Server
	serveErrors      chan error

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
		addr:             addr,
		mux:              http.NewServeMux(),
		logger:           logger,
		jsonMaxBodyBytes: DefaultJSONMaxBodyBytes,
		healthTimeout:    DefaultHealthCheckTimeout,
		serveErrors:      make(chan error, 2),
	}
	for _, o := range opts {
		o(s)
	}
	s.registerHealthEndpoints()
	return s
}

type httpRequestKey struct{}
type httpResponseWriterKey struct{}

func requestFromContext(ctx context.Context) *http.Request {
	r, _ := ctx.Value(httpRequestKey{}).(*http.Request)
	return r
}

func responseWriterFromContext(ctx context.Context) http.ResponseWriter {
	w, _ := ctx.Value(httpResponseWriterKey{}).(http.ResponseWriter)
	return w
}

func withHTTPContext(ctx context.Context, r *http.Request, w http.ResponseWriter) context.Context {
	ctx = context.WithValue(ctx, httpRequestKey{}, r)
	return context.WithValue(ctx, httpResponseWriterKey{}, w)
}
