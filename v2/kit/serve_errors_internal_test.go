package kit

import (
	"context"
	"errors"
	"testing"
	"time"
)

// TestHTTPServeErrorWaitsInsteadOfDropping pins the send semantics of a serve
// failure: with the buffer full and nobody consuming, the report waits rather
// than vanishing, and the announced stop ends the wait. A serve failure is
// terminal — a dead server that nobody was told about is worse than a parked
// goroutine.
func TestHTTPServeErrorWaitsInsteadOfDropping(t *testing.T) {
	component := MustNewHTTP("127.0.0.1:0")
	component.serveErrors <- errors.New("first failure")

	sent := make(chan struct{})
	go func() {
		component.reportServeError(errors.New("second failure"))
		close(sent)
	}()

	select {
	case <-sent:
		t.Fatal("the serve error was dropped or accepted past capacity")
	case <-time.After(100 * time.Millisecond):
	}

	if err := component.Drain(context.Background()); err != nil {
		t.Fatalf("Drain: %v", err)
	}
	select {
	case <-sent:
	case <-time.After(time.Second):
		t.Fatal("the announced stop did not end the wait")
	}
}

// TestHTTPServeErrorReachesAConsumer pins the other half: while the process
// runs and a consumer owns the channel, the report is delivered.
func TestHTTPServeErrorReachesAConsumer(t *testing.T) {
	component := MustNewHTTP("127.0.0.1:0")
	reported := make(chan error, 1)
	go func() {
		component.reportServeError(errors.New("serve failed"))
		close(reported)
	}()
	select {
	case err := <-component.Errors():
		if err == nil || err.Error() != "serve failed" {
			t.Fatalf("reported error = %v, want the serve failure", err)
		}
	case <-time.After(time.Second):
		t.Fatal("the serve error never reached the channel")
	}
}
