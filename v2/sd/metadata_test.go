package sd_test

import (
	"testing"

	"github.com/dreamsxin/go-kit/v2/sd"
)

func TestMetadataString(t *testing.T) {
	metadata := map[string]any{
		"zone":    "cn-north-1",
		"weight":  10,
		"ratio":   1.5,
		"enabled": true,
		"missing": nil,
	}

	tests := []struct {
		name string
		key  string
		want string
		ok   bool
	}{
		{name: "string passes through", key: "zone", want: "cn-north-1", ok: true},
		{name: "int renders", key: "weight", want: "10", ok: true},
		{name: "float renders", key: "ratio", want: "1.5", ok: true},
		{name: "bool renders", key: "enabled", want: "true", ok: true},
		{name: "nil value is absent", key: "missing", want: "", ok: false},
		{name: "unknown key is absent", key: "nope", want: "", ok: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := sd.MetadataString(metadata, tt.key)
			if got != tt.want || ok != tt.ok {
				t.Fatalf("MetadataString(%q) = %q, %v; want %q, %v", tt.key, got, ok, tt.want, tt.ok)
			}
		})
	}
}

// A registry delivers every label as a string, so a caller reading weight as an
// int must still see the "10" Consul returned.
func TestMetadataInt(t *testing.T) {
	tests := []struct {
		name  string
		value any
		want  int
		ok    bool
	}{
		{name: "int", value: 10, want: 10, ok: true},
		{name: "int64", value: int64(10), want: 10, ok: true},
		{name: "uint", value: uint(10), want: 10, ok: true},
		{name: "float64 truncates", value: 10.9, want: 10, ok: true},
		{name: "registry string", value: "10", want: 10, ok: true},
		{name: "padded string", value: " 10 ", want: 10, ok: true},
		{name: "negative string", value: "-5", want: -5, ok: true},
		{name: "unparsable string", value: "heavy", want: 0, ok: false},
		{name: "wrong type", value: []string{"10"}, want: 0, ok: false},
		{name: "nil", value: nil, want: 0, ok: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := sd.MetadataInt(map[string]any{"weight": tt.value}, "weight")
			if got != tt.want || ok != tt.ok {
				t.Fatalf("MetadataInt(%v) = %d, %v; want %d, %v", tt.value, got, ok, tt.want, tt.ok)
			}
		})
	}

	if _, ok := sd.MetadataInt(nil, "weight"); ok {
		t.Fatal("MetadataInt on a nil map reported a value")
	}
}

func TestMetadataBool(t *testing.T) {
	tests := []struct {
		name  string
		value any
		want  bool
		ok    bool
	}{
		{name: "bool", value: true, want: true, ok: true},
		{name: "registry string", value: "true", want: true, ok: true},
		{name: "numeric string", value: "1", want: true, ok: true},
		{name: "false string", value: "false", want: false, ok: true},
		{name: "unparsable", value: "yes please", want: false, ok: false},
		{name: "wrong type", value: 1, want: false, ok: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := sd.MetadataBool(map[string]any{"tls": tt.value}, "tls")
			if got != tt.want || ok != tt.ok {
				t.Fatalf("MetadataBool(%v) = %v, %v; want %v, %v", tt.value, got, ok, tt.want, tt.ok)
			}
		})
	}
}

func TestAddresses(t *testing.T) {
	instances := sd.Addresses("b:80", "a:80")
	if len(instances) != 2 {
		t.Fatalf("got %d instances, want 2", len(instances))
	}
	// Addresses preserves the given order; sorting is the cache's job.
	if instances[0].Address != "b:80" || instances[1].Address != "a:80" {
		t.Fatalf("Addresses reordered its input: %v", instances)
	}
	if instances[0].Metadata != nil {
		t.Fatalf("Addresses invented metadata: %v", instances[0].Metadata)
	}
	if len(sd.Addresses()) != 0 {
		t.Fatal("Addresses() with no arguments returned instances")
	}
}
