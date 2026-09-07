package server

import (
	"context"
	"errors"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// TestRecordingUnaryInterceptorReportsTheFullMethod pins the label. It is the whole
// reason gRPC needs no equivalent of the HTTP route defence: the method name comes
// from the service definition, so the series a scrape returns cannot grow with
// traffic.
func TestRecordingUnaryInterceptorReportsTheFullMethod(t *testing.T) {
	var seen []Observation
	interceptor := RecordingUnaryInterceptor(RecorderFunc(func(_ context.Context, obs Observation) {
		seen = append(seen, obs)
	}))

	info := &grpc.UnaryServerInfo{FullMethod: "/users.Users/Get"}
	response, err := interceptor(context.Background(), "request", info, func(context.Context, any) (any, error) {
		time.Sleep(time.Millisecond)
		return "response", nil
	})
	if err != nil {
		t.Fatalf("interceptor returned %v", err)
	}
	if response != "response" {
		t.Fatalf("response = %v, want it passed through", response)
	}
	if len(seen) != 1 {
		t.Fatalf("recorded %d observations, want 1", len(seen))
	}
	if seen[0].Method != "/users.Users/Get" {
		t.Errorf("Method = %q, want the full method name", seen[0].Method)
	}
	if seen[0].Code != codes.OK {
		t.Errorf("Code = %s, want OK", seen[0].Code)
	}
	if seen[0].Stream {
		t.Error("Stream = true for a unary call")
	}
	if seen[0].Duration <= 0 {
		t.Error("Duration was not measured")
	}
}

func TestRecordingUnaryInterceptorReportsTheStatusCode(t *testing.T) {
	var seen Observation
	interceptor := RecordingUnaryInterceptor(RecorderFunc(func(_ context.Context, obs Observation) {
		seen = obs
	}))

	info := &grpc.UnaryServerInfo{FullMethod: "/users.Users/Get"}
	_, err := interceptor(context.Background(), nil, info, func(context.Context, any) (any, error) {
		return nil, status.Error(codes.NotFound, "no such user")
	})
	if err == nil {
		t.Fatal("the handler error was swallowed")
	}
	if seen.Code != codes.NotFound {
		t.Errorf("Code = %s, want NotFound", seen.Code)
	}

	// An error that is not a status is Unknown, which is what a handler returning
	// a bare error means to a client, so it is what the metric should say too.
	_, _ = interceptor(context.Background(), nil, info, func(context.Context, any) (any, error) {
		return nil, errors.New("boom")
	})
	if seen.Code != codes.Unknown {
		t.Errorf("Code = %s, want Unknown for a non-status error", seen.Code)
	}
}

func TestRecordingStreamInterceptorRecordsWhenTheStreamEnds(t *testing.T) {
	var seen Observation
	recorded := make(chan struct{})
	interceptor := RecordingStreamInterceptor(RecorderFunc(func(_ context.Context, obs Observation) {
		seen = obs
		close(recorded)
	}))

	info := &grpc.StreamServerInfo{FullMethod: "/users.Users/Watch", IsServerStream: true}
	release := make(chan struct{})
	go func() {
		_ = interceptor(nil, fakeStream{}, info, func(any, grpc.ServerStream) error {
			<-release
			return nil
		})
	}()

	select {
	case <-recorded:
		t.Fatal("the stream was recorded before it ended")
	case <-time.After(50 * time.Millisecond):
	}

	close(release)
	select {
	case <-recorded:
	case <-time.After(2 * time.Second):
		t.Fatal("the stream was never recorded")
	}
	if !seen.Stream {
		t.Error("Stream = false for a streaming call")
	}
	if seen.Method != "/users.Users/Watch" {
		t.Errorf("Method = %q, want the full method name", seen.Method)
	}
}

func TestRecordingInterceptorsRejectANilRecorder(t *testing.T) {
	for name, build := range map[string]func(){
		"unary":  func() { RecordingUnaryInterceptor(nil) },
		"stream": func() { RecordingStreamInterceptor(nil) },
	} {
		t.Run(name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Error("a nil recorder was accepted; misassembly should fail at startup")
				}
			}()
			build()
		})
	}
}

// fakeStream is the minimum grpc.ServerStream a recording interceptor touches: it
// reads the context and nothing else.
type fakeStream struct {
	grpc.ServerStream
}

func (fakeStream) Context() context.Context { return context.Background() }
