package grpc

import (
	"context"
	"net"
	"testing"
	"time"
)

func TestComponentLifecycle(t *testing.T) {
	component := MustNew("127.0.0.1:0")
	if component.Server() == nil {
		t.Fatal("Server returned nil")
	}
	if err := component.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if component.Addr() == nil {
		t.Fatal("Addr returned nil after Start")
	}
	if err := component.Start(); err == nil {
		t.Fatal("expected duplicate Start error")
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := component.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	if err := component.Start(); err == nil {
		t.Fatal("expected restart error")
	}
}

func TestComponentStartReportsBindError(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer listener.Close()

	component := MustNew(listener.Addr().String())
	if err := component.Start(); err == nil {
		t.Fatal("expected bind error")
	}
}

func TestNewRejectsEmptyAddress(t *testing.T) {
	if _, err := New(""); err == nil {
		t.Fatal("expected empty address error")
	}
}

func TestNewRejectsNilServerOption(t *testing.T) {
	if _, err := New(":0", nil); err == nil {
		t.Fatal("expected nil server option error")
	}
}
