package kit_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/dreamsxin/go-kit/v2/kit"
)

// TestStoppingClosesWhenTheComponentDrains proves a long-lived response is told
// the process is going away while it can still finish: the signal closes at drain
// time, before any connection is closed, so a stream ends itself instead of being
// cut.
func TestStoppingClosesWhenTheComponentDrains(t *testing.T) {
	component := kit.MustNewHTTP("127.0.0.1:0")

	observed := make(chan bool, 1)
	component.HandleFunc("GET /stream", func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-kit.Stopping(r.Context()):
			observed <- true
		case <-time.After(3 * time.Second):
			observed <- false
		}
		w.WriteHeader(http.StatusOK)
	})

	// The request is served through the component's own handler, so the same
	// assertion holds whether or not it owns the listener.
	go func() {
		recorder := httptest.NewRecorder()
		component.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/stream", nil))
	}()

	host := kit.MustNewHost(kit.WithLifecycle(component))
	// Give the handler a moment to reach its select before the announcement.
	time.Sleep(50 * time.Millisecond)
	if err := host.Drain(context.Background()); err != nil {
		t.Fatalf("Drain: %v", err)
	}

	select {
	case stopped := <-observed:
		if !stopped {
			t.Fatal("the handler was never told the process is stopping")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("the handler did not return after the drain announcement")
	}

	// The component is still serving: draining announces, it does not close.
	recorder := httptest.NewRecorder()
	component.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/livez", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("livez after draining = %d, want 200: the server should still be answering", recorder.Code)
	}
}

// TestStoppingIsNilOutsideAKitServer proves the seam is honest about not knowing:
// a context that never passed through a kit HTTP component reports no signal, and
// a select on a nil channel simply never fires.
func TestStoppingIsNilOutsideAKitServer(t *testing.T) {
	if stopping := kit.Stopping(context.Background()); stopping != nil {
		t.Fatalf("Stopping(background) = %v, want nil", stopping)
	}
	select {
	case <-kit.Stopping(context.Background()):
		t.Fatal("a nil stopping channel fired")
	case <-time.After(20 * time.Millisecond):
	}
}

// TestShutdownStaysGracefulWhenHandlersFinish proves the hard close is a last
// resort: a handler that returns inside the budget is not interrupted, and
// Shutdown reports success.
func TestShutdownStaysGracefulWhenHandlersFinish(t *testing.T) {
	component := kit.MustNewHTTP("127.0.0.1:0")
	component.HandleFunc("GET /quick", func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(20 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	})
	if err := component.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}

	done := make(chan int, 1)
	go func() {
		resp, err := http.Get("http://" + component.Addr() + "/quick")
		if err != nil {
			done <- 0
			return
		}
		defer resp.Body.Close()
		done <- resp.StatusCode
	}()
	time.Sleep(10 * time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := component.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown = %v, want nil for handlers that finish in time", err)
	}
	if status := <-done; status != http.StatusOK {
		t.Fatalf("in-flight request status = %d, want 200: it should not have been interrupted", status)
	}
}

// TestShutdownClosesWhatTheGracePeriodLeftOpen proves the grace period ends. A
// handler that ignores cancellation used to leave Shutdown returning a deadline
// error with the connection still open and the goroutine still running; now the
// request context is cancelled, the connection is closed, and the error says how
// many requests that was.
func TestShutdownClosesWhatTheGracePeriodLeftOpen(t *testing.T) {
	component := kit.MustNewHTTP("127.0.0.1:0")

	cancelled := make(chan struct{}, 1)
	release := make(chan struct{})
	component.HandleFunc("GET /stubborn", func(w http.ResponseWriter, r *http.Request) {
		// Deliberately watches nothing until its context is cancelled, which is
		// what an unconditional stream or a blocked upstream call looks like.
		select {
		case <-r.Context().Done():
			cancelled <- struct{}{}
		case <-release:
		}
	})
	if err := component.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer close(release)

	client := &http.Client{Timeout: 5 * time.Second}
	requested := make(chan error, 1)
	go func() {
		resp, err := client.Get("http://" + component.Addr() + "/stubborn")
		if err == nil {
			resp.Body.Close()
		}
		requested <- err
	}()
	time.Sleep(100 * time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	err := component.Shutdown(ctx)
	if err == nil {
		t.Fatal("Shutdown = nil, want the interrupted request reported")
	}
	if !errors.Is(err, kit.ErrShutdownIncomplete) {
		t.Fatalf("Shutdown error = %v, want ErrShutdownIncomplete", err)
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Shutdown error = %v, want the graceful deadline reported too", err)
	}
	if !strings.Contains(err.Error(), "request(s) still in flight") {
		t.Fatalf("Shutdown error = %q, want it to say how many requests were interrupted", err)
	}

	select {
	case <-cancelled:
	case <-time.After(2 * time.Second):
		t.Fatal("the handler's request context was never cancelled")
	}
	select {
	case <-requested:
	case <-time.After(3 * time.Second):
		t.Fatal("the client was left hanging on a closed server")
	}

	// The listener is gone: a shutdown that ends leaves nothing accepting.
	if _, err := http.Get("http://" + component.Addr() + "/stubborn"); err == nil {
		t.Fatal("the listener still accepts connections after shutdown")
	}
}

// TestAddrReportsTheBoundPort proves the component can be reached after binding
// port zero, which is the only way a test or a sidecar can learn where it landed.
func TestAddrReportsTheBoundPort(t *testing.T) {
	component := kit.MustNewHTTP("127.0.0.1:0")
	if got := component.Addr(); got != "127.0.0.1:0" {
		t.Fatalf("Addr before Start = %q, want the configured address", got)
	}
	if err := component.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer component.Shutdown(context.Background()) //nolint:errcheck

	addr := component.Addr()
	if addr == "127.0.0.1:0" || !strings.HasPrefix(addr, "127.0.0.1:") {
		t.Fatalf("Addr after Start = %q, want the resolved port", addr)
	}
	resp, err := http.Get(fmt.Sprintf("http://%s/livez", addr))
	if err != nil {
		t.Fatalf("GET /livez on %s: %v", addr, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("livez status = %d", resp.StatusCode)
	}
}
