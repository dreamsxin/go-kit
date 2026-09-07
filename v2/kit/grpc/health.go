package grpc

import (
	"context"
	"sync"
	"time"

	"github.com/dreamsxin/go-kit/v2/health"
	"google.golang.org/grpc/codes"
	grpc_health_v1 "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/status"
)

// Answering grpc.health.v1.Health from a probe registry.
//
// Milestone 14 declared that Watch was not implemented, on the grounds that the tools
// which orchestrate on gRPC health call Check. That is true of grpc_health_probe and
// of Kubernetes' native gRPC probe, and wrong about the consumer that matters most:
// grpc-go's own client-side health checking — the one a service config turns on with
// healthCheckConfig — calls Watch. When it receives UNIMPLEMENTED it marks the
// connection Ready and stops asking, which means the drain announcement never reaches
// the clients that were told to watch for it. A declaration that is true of two tools
// and false of the library is not a declaration worth keeping, so Watch is implemented
// here and the claim is corrected.

// HealthWatchInterval is how often a Watch stream re-evaluates the readiness checks.
//
// It is the resolution of the stream, and it is a compromise worth naming: readiness
// checks do real work — a database ping, a dependency call — so evaluating them once
// per watcher per message would turn a fleet of clients into load. One evaluation per
// interval is shared by every watcher, and a status change reaches all of them within
// this window.
const HealthWatchInterval = time.Second

// healthService answers grpc.health.v1.Health from a probe registry.
//
// Check evaluates the readiness scope on every call rather than reading a status
// somebody remembered to set, so the answer cannot go stale. Watch sends the current
// status and then one message per change, evaluating the same checks on a shared
// ticker.
type healthService struct {
	grpc_health_v1.UnimplementedHealthServer
	probes *health.Registry

	mu       sync.Mutex
	watchers map[chan grpc_health_v1.HealthCheckResponse_ServingStatus]struct{}
	watching bool
	last     grpc_health_v1.HealthCheckResponse_ServingStatus
}

func (s *healthService) Check(ctx context.Context, request *grpc_health_v1.HealthCheckRequest) (*grpc_health_v1.HealthCheckResponse, error) {
	// The registry describes the process, not one service within it, so a
	// per-service question has no honest answer here. The protocol's own reply
	// for that is NotFound.
	if request != nil && request.GetService() != "" {
		return nil, status.Error(codes.NotFound, "kit/grpc: readiness is reported for the process, not per service")
	}
	return &grpc_health_v1.HealthCheckResponse{Status: s.servingStatus(ctx)}, nil
}

// Watch streams the process's serving status: the current value immediately, then one
// message per change until the client goes away.
//
// Stable: grpc.health-watch — Watch reports the current serving status immediately and then one message per change, so a client doing gRPC health checking learns about a drain instead of being told health checking is unimplemented.
// Covered by: TestHealthWatchReportsTheCurrentStatusThenChanges, TestHealthWatchRejectsAPerServiceRequest
func (s *healthService) Watch(request *grpc_health_v1.HealthCheckRequest, stream grpc_health_v1.Health_WatchServer) error {
	if request != nil && request.GetService() != "" {
		return status.Error(codes.NotFound, "kit/grpc: readiness is reported for the process, not per service")
	}

	updates := s.subscribe()
	defer s.unsubscribe(updates)

	current := s.servingStatus(stream.Context())
	if err := stream.Send(&grpc_health_v1.HealthCheckResponse{Status: current}); err != nil {
		return err
	}
	for {
		select {
		case <-stream.Context().Done():
			// The client hung up or the server is going away; neither is an error
			// worth reporting to a stream nobody is reading.
			return nil
		case next := <-updates:
			if next == current {
				continue
			}
			current = next
			if err := stream.Send(&grpc_health_v1.HealthCheckResponse{Status: current}); err != nil {
				return err
			}
		}
	}
}

func (s *healthService) servingStatus(ctx context.Context) grpc_health_v1.HealthCheckResponse_ServingStatus {
	if s.probes.Report(ctx, health.ScopeReadiness).Status != health.StatusOK {
		return grpc_health_v1.HealthCheckResponse_NOT_SERVING
	}
	return grpc_health_v1.HealthCheckResponse_SERVING
}

// subscribe registers a watcher and starts the shared poller if it is not running.
//
// The channel holds one status: a watcher that has not caught up wants the latest
// value, not a queue of stale ones.
func (s *healthService) subscribe() chan grpc_health_v1.HealthCheckResponse_ServingStatus {
	updates := make(chan grpc_health_v1.HealthCheckResponse_ServingStatus, 1)
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.watchers == nil {
		s.watchers = map[chan grpc_health_v1.HealthCheckResponse_ServingStatus]struct{}{}
	}
	s.watchers[updates] = struct{}{}
	if !s.watching {
		s.watching = true
		s.last = s.unknownStatus()
		go s.poll()
	}
	return updates
}

func (s *healthService) unsubscribe(updates chan grpc_health_v1.HealthCheckResponse_ServingStatus) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.watchers, updates)
}

// unknownStatus is the poller's starting point, so the first evaluation always counts
// as a change and every watcher gets told even if nothing moved while nobody watched.
func (s *healthService) unknownStatus() grpc_health_v1.HealthCheckResponse_ServingStatus {
	return grpc_health_v1.HealthCheckResponse_UNKNOWN
}

// poll evaluates the readiness checks on a ticker and broadcasts changes. It stops
// when the last watcher leaves, so a service nobody watches costs nothing.
func (s *healthService) poll() {
	ticker := time.NewTicker(HealthWatchInterval)
	defer ticker.Stop()
	for range ticker.C {
		s.mu.Lock()
		if len(s.watchers) == 0 {
			s.watching = false
			s.mu.Unlock()
			return
		}
		s.mu.Unlock()

		// Evaluated outside the lock: a readiness check can be slow, and holding the
		// lock across it would block clients arriving and leaving.
		next := s.servingStatus(context.Background())

		s.mu.Lock()
		if next != s.last {
			s.last = next
			for updates := range s.watchers {
				// Replace what a slow watcher has not read yet: the newest status is
				// the only one worth delivering.
				select {
				case <-updates:
				default:
				}
				select {
				case updates <- next:
				default:
				}
			}
		}
		s.mu.Unlock()
	}
}
