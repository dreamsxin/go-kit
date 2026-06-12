package client

import (
	"context"
	"testing"

	"google.golang.org/grpc/metadata"
)

func TestEncodeKeyValue_LowercasesKey(t *testing.T) {
	k, v := EncodeKeyValue("X-Custom-Header", "value")
	if k != "x-custom-header" {
		t.Errorf("key: got %q, want %q", k, "x-custom-header")
	}
	if v != "value" {
		t.Errorf("value: got %q, want %q", v, "value")
	}
}

func TestEncodeKeyValue_BinarySuffix_Base64Encodes(t *testing.T) {
	k, v := EncodeKeyValue("X-Token-Bin", "secret")
	if k != "x-token-bin" {
		t.Errorf("key: got %q, want %q", k, "x-token-bin")
	}
	// base64("secret") = "c2VjcmV0"
	if v != "c2VjcmV0" {
		t.Errorf("value: got %q, want %q", v, "c2VjcmV0")
	}
}

func TestEncodeKeyValue_NoBinarySuffix_PlainValue(t *testing.T) {
	k, v := EncodeKeyValue("Authorization", "Bearer xyz")
	if k != "authorization" {
		t.Errorf("key: got %q", k)
	}
	if v != "Bearer xyz" {
		t.Errorf("value: got %q, want %q", v, "Bearer xyz")
	}
}

func TestSetRequestHeader_AddsToMetadata(t *testing.T) {
	fn := SetRequestHeader("X-Request-ID", "abc-123")
	md := metadata.MD{}
	ctx := fn(context.Background(), &md)
	_ = ctx

	vals := md["x-request-id"]
	if len(vals) != 1 || vals[0] != "abc-123" {
		t.Errorf("metadata: got %v, want [abc-123]", vals)
	}
}

func TestSetRequestHeader_AppendsToExisting(t *testing.T) {
	md := metadata.MD{"x-trace": []string{"first"}}
	fn := SetRequestHeader("X-Trace", "second")
	fn(context.Background(), &md)

	vals := md["x-trace"]
	if len(vals) != 2 {
		t.Fatalf("expected 2 values, got %d: %v", len(vals), vals)
	}
	if vals[0] != "first" || vals[1] != "second" {
		t.Errorf("values: got %v, want [first second]", vals)
	}
}

func TestSetRequestHeader_BinaryHeader(t *testing.T) {
	fn := SetRequestHeader("X-Payload-Bin", "raw")
	md := metadata.MD{}
	fn(context.Background(), &md)

	vals := md["x-payload-bin"]
	if len(vals) != 1 {
		t.Fatalf("expected 1 value, got %d", len(vals))
	}
	// base64("raw") = "cmF3"
	if vals[0] != "cmF3" {
		t.Errorf("value: got %q, want %q", vals[0], "cmF3")
	}
}
