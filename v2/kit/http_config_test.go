package kit

import (
	"errors"
	"net"
	"testing"
	"time"
)

func TestWithHTTPServerConfig(t *testing.T) {
	want := HTTPServerConfig{
		ReadHeaderTimeout: 2 * time.Second,
		ReadTimeout:       5 * time.Second,
		WriteTimeout:      7 * time.Second,
		IdleTimeout:       30 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}
	svc := New(":0", WithHTTPServerConfig(want))
	if svc.httpConfig != want {
		t.Fatalf("http config: got %#v, want %#v", svc.httpConfig, want)
	}
}

func TestWithHTTPServerConfigRejectsNegativeValues(t *testing.T) {
	tests := []HTTPServerConfig{
		{ReadHeaderTimeout: -time.Second},
		{ReadTimeout: -time.Second},
		{WriteTimeout: -time.Second},
		{IdleTimeout: -time.Second},
		{MaxHeaderBytes: -1},
	}
	for _, config := range tests {
		t.Run("invalid", func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Fatal("expected panic")
				}
			}()
			WithHTTPServerConfig(config)
		})
	}
}

func TestServiceErrors(t *testing.T) {
	svc := New(":0")
	want := errors.New("serve failed")
	svc.reportServeError(want)
	select {
	case got := <-svc.Errors():
		if !errors.Is(got, want) {
			t.Fatalf("error: got %v, want %v", got, want)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for service error")
	}
}

func TestStartClosesHTTPListenerWhenGRPCBindFails(t *testing.T) {
	occupied, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer occupied.Close()

	svc := New(":0", WithGRPC(occupied.Addr().String()))
	if err := svc.Start(); err == nil {
		t.Fatal("expected gRPC bind error")
	}
	if svc.srv != nil {
		t.Fatal("HTTP server should not start when gRPC bind fails")
	}
}
