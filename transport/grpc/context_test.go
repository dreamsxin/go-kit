package grpc

import (
	"context"
	"testing"
)

func TestContextKeys_AreDistinct(t *testing.T) {
	keys := []contextKey{
		ContextKeyRequestMethod,
		ContextKeyResponseHeaders,
		ContextKeyResponseTrailers,
	}
	seen := make(map[contextKey]bool, len(keys))
	for _, k := range keys {
		if seen[k] {
			t.Errorf("duplicate context key: %v", k)
		}
		seen[k] = true
	}
}

func TestContextKeys_RoundTrip(t *testing.T) {
	ctx := context.Background()

	// Inject each key and verify retrieval.
	ctx = context.WithValue(ctx, ContextKeyRequestMethod, "/svc/Method")
	ctx = context.WithValue(ctx, ContextKeyResponseHeaders, "header-md")
	ctx = context.WithValue(ctx, ContextKeyResponseTrailers, "trailer-md")

	if got := ctx.Value(ContextKeyRequestMethod); got != "/svc/Method" {
		t.Errorf("RequestMethod: got %v, want /svc/Method", got)
	}
	if got := ctx.Value(ContextKeyResponseHeaders); got != "header-md" {
		t.Errorf("ResponseHeaders: got %v, want header-md", got)
	}
	if got := ctx.Value(ContextKeyResponseTrailers); got != "trailer-md" {
		t.Errorf("ResponseTrailers: got %v, want trailer-md", got)
	}
}

func TestContextKeys_ZeroValue(t *testing.T) {
	ctx := context.Background()
	if v := ctx.Value(ContextKeyRequestMethod); v != nil {
		t.Errorf("expected nil for unset key, got %v", v)
	}
}
