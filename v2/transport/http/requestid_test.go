package http_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dreamsxin/go-kit/v2/endpoint"
	transporthttp "github.com/dreamsxin/go-kit/v2/transport/http"
)

func TestDefaultRequestIDValidator(t *testing.T) {
	accepted := []string{"abc123", "a-b_c.d:e", strings.Repeat("a", transporthttp.MaxRequestIDLength)}
	for _, id := range accepted {
		if !transporthttp.DefaultRequestIDValidator(id) {
			t.Errorf("rejected %q", id)
		}
	}

	// A rejected value is one that would be unsafe or useless to echo back:
	// the CR and LF cases are header injection, which is why validation is not
	// merely cosmetic.
	rejected := []string{
		"",
		strings.Repeat("a", transporthttp.MaxRequestIDLength+1),
		"has space",
		"tab\there",
		"new\nline",
		"carriage\rreturn",
		"null\x00byte",
		"slash/es",
	}
	for _, id := range rejected {
		if transporthttp.DefaultRequestIDValidator(id) {
			t.Errorf("accepted %q", id)
		}
	}
}

func TestRequestIDPrefersAValidCallerHeader(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.Header.Set(transporthttp.RequestIDHeader, "caller-supplied")
	if got := transporthttp.RequestID(request, nil); got != "caller-supplied" {
		t.Fatalf("request ID = %q, want the caller's", got)
	}
}

func TestRequestIDMintsOneWhenTheHeaderIsUnusable(t *testing.T) {
	for name, header := range map[string]string{
		"absent":  "",
		"invalid": "not a valid id",
	} {
		request := httptest.NewRequest(http.MethodGet, "/", nil)
		if header != "" {
			request.Header.Set(transporthttp.RequestIDHeader, header)
		}
		id := transporthttp.RequestID(request, nil)
		if id == header || !transporthttp.DefaultRequestIDValidator(id) {
			t.Errorf("%s: request ID = %q, want a fresh valid one", name, id)
		}
	}
}

func TestRequestIDHonoursACustomValidator(t *testing.T) {
	onlyEven := func(id string) bool { return len(id)%2 == 0 }
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.Header.Set(transporthttp.RequestIDHeader, "odd")
	if got := transporthttp.RequestID(request, onlyEven); got == "odd" {
		t.Fatal("a custom validator did not reject the caller's ID")
	}
	request.Header.Set(transporthttp.RequestIDHeader, "even")
	if got := transporthttp.RequestID(request, onlyEven); got != "even" {
		t.Fatalf("request ID = %q, want the caller's", got)
	}
}

func TestExtractRequestIDKeepsAnIDAlreadyInTheContext(t *testing.T) {
	ctx := endpoint.WithRequestID(context.Background(), "from-upstream")
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.Header.Set(transporthttp.RequestIDHeader, "from-header")

	ctx = transporthttp.ExtractRequestID(ctx, request)
	if got := endpoint.RequestIDFromContext(ctx); got != "from-upstream" {
		t.Fatalf("request ID = %q, want the one already in the context", got)
	}
}

func TestExtractRequestIDReadsTheHeader(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.Header.Set(transporthttp.RequestIDHeader, "from-header")

	ctx := transporthttp.ExtractRequestID(context.Background(), request)
	if got := endpoint.RequestIDFromContext(ctx); got != "from-header" {
		t.Fatalf("request ID = %q, want the header's", got)
	}
}

func TestEchoRequestIDWritesTheHeader(t *testing.T) {
	recorder := httptest.NewRecorder()
	ctx := endpoint.WithRequestID(context.Background(), "correlate-me")
	transporthttp.EchoRequestID(ctx, recorder)
	if got := recorder.Header().Get(transporthttp.RequestIDHeader); got != "correlate-me" {
		t.Fatalf("header = %q, want %q", got, "correlate-me")
	}
}

func TestEchoRequestIDWritesNothingWithoutAnID(t *testing.T) {
	recorder := httptest.NewRecorder()
	transporthttp.EchoRequestID(context.Background(), recorder)
	if got := recorder.Header().Get(transporthttp.RequestIDHeader); got != "" {
		t.Fatalf("header = %q, want it absent", got)
	}
}
