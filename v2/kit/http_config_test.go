package kit

import (
	"context"
	"errors"
	"testing"
	"time"
)

type failingLifecycle struct {
	err error
}

func (f failingLifecycle) Start() error                   { return f.err }
func (f failingLifecycle) Errors() <-chan error           { return nil }
func (f failingLifecycle) Shutdown(context.Context) error { return nil }

func TestWithHTTPServerConfig(t *testing.T) {
	want := HTTPServerConfig{
		ReadHeaderTimeout: 2 * time.Second,
		ReadTimeout:       5 * time.Second,
		WriteTimeout:      7 * time.Second,
		IdleTimeout:       30 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}
	h := MustNewHTTP(":0", WithHTTPServerConfig(want))
	if h.httpConfig != want {
		t.Fatalf("http config: got %#v, want %#v", h.httpConfig, want)
	}
}

func TestNewHTTPUsesStreamingSafeDefaults(t *testing.T) {
	h := MustNewHTTP(":0")
	want := DefaultHTTPServerConfig()
	if h.httpConfig != want {
		t.Fatalf("http config: got %#v, want %#v", h.httpConfig, want)
	}
	if h.httpConfig.WriteTimeout != 0 {
		t.Fatalf("WriteTimeout = %v, want 0 for streaming responses", h.httpConfig.WriteTimeout)
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
			if _, err := NewHTTP(":0", WithHTTPServerConfig(config)); err == nil {
				t.Fatal("expected invalid HTTP server config error")
			}
		})
	}
}

func TestNewHTTPRejectsInvalidBaseConfiguration(t *testing.T) {
	if _, err := NewHTTP(""); err == nil {
		t.Fatal("expected empty HTTP address error")
	}
	if _, err := NewHTTP(":0", nil); err == nil {
		t.Fatal("expected nil option error")
	}
}

func TestHTTPErrors(t *testing.T) {
	h := MustNewHTTP(":0")
	want := errors.New("serve failed")
	h.reportServeError(want)
	select {
	case got := <-h.Errors():
		if !errors.Is(got, want) {
			t.Fatalf("error: got %v, want %v", got, want)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for serve error")
	}
}

func TestHostStartFailsWhenComponentStartFails(t *testing.T) {
	host := MustNewHost(WithLifecycle(failingLifecycle{err: errors.New("start failed")}))
	if err := host.Start(); err == nil {
		t.Fatal("expected lifecycle start error")
	}
}

func TestHTTPStartAfterFailedHostStart(t *testing.T) {
	h := MustNewHTTP("127.0.0.1:0")
	host := MustNewHost(WithLifecycle(h, failingLifecycle{err: errors.New("start failed")}))
	if err := host.Start(); err == nil {
		t.Fatal("expected lifecycle start error")
	}
	h.lifecycleMu.Lock()
	started := h.started
	h.lifecycleMu.Unlock()
	if started {
		t.Fatal("HTTP component should not stay started when a later component fails")
	}
}

func TestHostRunStopsOnContextCancellation(t *testing.T) {
	host := MustNewHost(WithShutdownTimeout(time.Second))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := host.Run(ctx); err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestHostRunReturnsAsynchronousComponentError(t *testing.T) {
	worker := failingLifecycle{}
	host := MustNewHost(WithLifecycle(worker), WithShutdownTimeout(time.Second))
	want := errors.New("component failed")
	done := make(chan error, 1)
	go func() { done <- host.Run(context.Background()) }()

	host.reportServeError(want)

	select {
	case err := <-done:
		if !errors.Is(err, want) {
			t.Fatalf("Run error: got %v, want %v", err, want)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not stop after component error")
	}
}

func TestHostCannotStartTwiceOrRestart(t *testing.T) {
	host := MustNewHost()
	if err := host.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := host.Start(); err == nil {
		t.Fatal("expected second Start to fail")
	}
	if err := host.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	if err := host.Start(); err == nil {
		t.Fatal("expected restart after Shutdown to fail")
	}
}

func TestHTTPCannotStartTwiceOrRestart(t *testing.T) {
	h := MustNewHTTP(":0")
	if err := h.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := h.Start(); err == nil {
		t.Fatal("expected second Start to fail")
	}
	if err := h.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	if err := h.Start(); err == nil {
		t.Fatal("expected restart after Shutdown to fail")
	}
}
