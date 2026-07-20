package kit

import (
	"context"
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
	svc := MustNew(":0", WithHTTPServerConfig(want))
	if svc.httpConfig != want {
		t.Fatalf("http config: got %#v, want %#v", svc.httpConfig, want)
	}
}

func TestNewUsesStreamingSafeHTTPDefaults(t *testing.T) {
	svc := MustNew(":0")
	want := DefaultHTTPServerConfig()
	if svc.httpConfig != want {
		t.Fatalf("http config: got %#v, want %#v", svc.httpConfig, want)
	}
	if svc.httpConfig.WriteTimeout != 0 {
		t.Fatalf("WriteTimeout = %v, want 0 for streaming responses", svc.httpConfig.WriteTimeout)
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
			if _, err := New(":0", WithHTTPServerConfig(config)); err == nil {
				t.Fatal("expected invalid HTTP server config error")
			}
		})
	}
}

func TestNewRejectsInvalidBaseConfiguration(t *testing.T) {
	if _, err := New(""); err == nil {
		t.Fatal("expected empty HTTP address error")
	}
	if _, err := New(":0", nil); err == nil {
		t.Fatal("expected nil option error")
	}
}

func TestServiceErrors(t *testing.T) {
	svc := MustNew(":0")
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

	svc := MustNew(":0", WithGRPC(occupied.Addr().String()))
	if err := svc.Start(); err == nil {
		t.Fatal("expected gRPC bind error")
	}
	if svc.srv != nil {
		t.Fatal("HTTP server should not start when gRPC bind fails")
	}
}

func TestRunStopsOnContextCancellation(t *testing.T) {
	svc := MustNew(":0", WithShutdownTimeout(time.Second))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := svc.Run(ctx); err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestRunReturnsAsynchronousServeError(t *testing.T) {
	svc := MustNew(":0", WithShutdownTimeout(time.Second))
	want := errors.New("serve failed")
	done := make(chan error, 1)
	go func() { done <- svc.Run(context.Background()) }()

	deadline := time.Now().Add(time.Second)
	for {
		svc.lifecycleMu.Lock()
		started := svc.started
		svc.lifecycleMu.Unlock()
		if started {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("service did not start")
		}
		time.Sleep(time.Millisecond)
	}
	svc.reportServeError(want)

	select {
	case err := <-done:
		if !errors.Is(err, want) {
			t.Fatalf("Run error: got %v, want %v", err, want)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not stop after serve error")
	}
}

func TestServiceCannotStartTwiceOrRestart(t *testing.T) {
	svc := MustNew(":0")
	if err := svc.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := svc.Start(); err == nil {
		t.Fatal("expected second Start to fail")
	}
	if err := svc.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	if err := svc.Start(); err == nil {
		t.Fatal("expected restart after Shutdown to fail")
	}
}
