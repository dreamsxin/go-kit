package grpc

import (
	"context"
	"errors"
	"testing"
	"time"
)

// TestGRPCServeErrorWaitsInsteadOfDropping pins the send semantics of a serve
// failure: with the buffer full and nobody consuming, the report waits rather
// than vanishing, and the announced stop ends the wait.
func TestGRPCServeErrorWaitsInsteadOfDropping(t *testing.T) {
	component := MustNew("127.0.0.1:0")
	component.errors <- errors.New("first failure")

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

// TestGRPCServeErrorReachesAConsumer pins the other half: while the process
// runs and a consumer owns the channel, the report is delivered.
func TestGRPCServeErrorReachesAConsumer(t *testing.T) {
	component := MustNew("127.0.0.1:0")
	go func() {
		component.reportServeError(errors.New("serve failed"))
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
