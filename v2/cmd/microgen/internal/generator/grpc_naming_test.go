package generator

import (
	"strings"
	"testing"
)

// TestGRPCProtoPackage covers the composition and the refusal rules: the
// prefix's segments become the proto package's leading segments, and a
// segment protoc would reject is refused at generation time.
func TestGRPCProtoPackage(t *testing.T) {
	cases := []struct {
		name        string
		routePrefix string
		service     string
		want        string
		wantErr     string
	}{
		{name: "no prefix", routePrefix: "", service: "userservice", want: "userservice"},
		{name: "single segment", routePrefix: "/api", service: "userservice", want: "api.userservice"},
		{name: "multi segment", routePrefix: "/api/v1", service: "userservice", want: "api.v1.userservice"},
		{name: "no leading slash", routePrefix: "api/v1", service: "userservice", want: "api.v1.userservice"},
		{name: "trailing slash", routePrefix: "/api/v1/", service: "userservice", want: "api.v1.userservice"},
		{name: "underscore segment", routePrefix: "/user_api", service: "userservice", want: "user_api.userservice"},
		{name: "empty segment tolerated", routePrefix: "/api//v1", service: "userservice", want: "api.v1.userservice"},
		{name: "uppercase normalized", routePrefix: "/API", service: "userservice", want: "api.userservice"},
		{name: "leading lowercase digit suffix ok", routePrefix: "/v1api", service: "userservice", want: "v1api.userservice"},
		{name: "hyphen refused", routePrefix: "/api-v1", service: "userservice", wantErr: "not a valid proto identifier"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := grpcProtoPackage(tc.routePrefix, tc.service)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("grpcProtoPackage(%q) error = %v, want substring %q", tc.routePrefix, err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("grpcProtoPackage(%q): %v", tc.routePrefix, err)
			}
			if got != tc.want {
				t.Errorf("package = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestValidateRoutePrefix: the option-level check mirrors the package rule,
// so a prefix that would produce an invalid proto package is refused before
// anything is generated.
func TestValidateRoutePrefix(t *testing.T) {
	for _, broken := range []string{"/API", "/v1-api", "/1api"} {
		if err := validateRoutePrefix(broken); err == nil {
			t.Errorf("validateRoutePrefix(%q) = nil, want a refusal", broken)
		}
	}
	if err := validateRoutePrefix("/api/v1"); err != nil {
		t.Errorf("validateRoutePrefix(/api/v1) = %v", err)
	}
	if err := validateRoutePrefix(""); err != nil {
		t.Errorf("empty prefix = %v", err)
	}
}
