package kit

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"google.golang.org/grpc"

	"github.com/dreamsxin/go-kit/v2/endpoint"
	kitlog "github.com/dreamsxin/go-kit/v2/log"
)

// Service is a ready-to-run HTTP + gRPC microservice.
// Create one with New, register handlers with Handle/GRPC, then call Run.
type Service struct {
	addr             string
	mux              *http.ServeMux
	httpHandler      http.Handler
	httpMiddleware   []func(http.Handler) http.Handler
	middleware       []endpoint.Middleware
	routeMiddleware  []func(route string) endpoint.Middleware
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

	lifecycleMu     sync.Mutex
	started         bool
	stopped         bool
	shutdownTimeout time.Duration
}

// Option configures a Service.
type Option func(*Service) error

// New creates a Service listening on addr (for example ":8080").
func New(addr string, opts ...Option) (*Service, error) {
	if strings.TrimSpace(addr) == "" {
		return nil, fmt.Errorf("kit: HTTP address cannot be empty")
	}
	logger, err := kitlog.NewDevelopment()
	if err != nil {
		return nil, fmt.Errorf("kit: create default logger: %w", err)
	}
	s := &Service{
		addr:             addr,
		mux:              http.NewServeMux(),
		logger:           logger,
		httpConfig:       DefaultHTTPServerConfig(),
		jsonMaxBodyBytes: DefaultJSONMaxBodyBytes,
		healthTimeout:    DefaultHealthCheckTimeout,
		serveErrors:      make(chan error, 2),
		shutdownTimeout:  DefaultShutdownTimeout,
	}
	for i, option := range opts {
		if option == nil {
			return nil, fmt.Errorf("kit: option %d is nil", i)
		}
		if err := option(s); err != nil {
			return nil, fmt.Errorf("kit: apply option %d: %w", i, err)
		}
	}
	s.registerHealthEndpoints()
	s.httpHandler = s.applyHTTPMiddleware(s.mux)
	return s, nil
}

// MustNew creates a Service and panics if its configuration is invalid.
// It is intended for tests and small examples; production startup should use New.
func MustNew(addr string, opts ...Option) *Service {
	s, err := New(addr, opts...)
	if err != nil {
		panic(err)
	}
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
