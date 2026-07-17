package generator

import (
	"path/filepath"
	"reflect"
	"testing"
)

func TestProjectLayoutRequiredDirs(t *testing.T) {
	layout := newProjectLayout("out")
	services := []*serviceView{
		{ServiceName: "UserService"},
		{ServiceName: "OrderService"},
	}

	got := layout.requiredDirs(services, Options{
		WithConfig: true,
		WithModel:  true,
		WithSkill:  true,
	})

	want := []string{
		filepath.Join("out", "cmd"),
		filepath.Join("out", "service", "userservice"),
		filepath.Join("out", "endpoint", "userservice"),
		filepath.Join("out", "transport", "userservice"),
		filepath.Join("out", "client", "userservice"),
		filepath.Join("out", "sdk", "userservicesdk"),
		filepath.Join("out", "service", "orderservice"),
		filepath.Join("out", "endpoint", "orderservice"),
		filepath.Join("out", "transport", "orderservice"),
		filepath.Join("out", "client", "orderservice"),
		filepath.Join("out", "sdk", "orderservicesdk"),
		filepath.Join("out", "config"),
		filepath.Join("out", "model"),
		filepath.Join("out", "repository"),
		filepath.Join("out", "skill"),
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("requiredDirs mismatch:\n got: %#v\nwant: %#v", got, want)
	}
}

func TestProjectLayoutArtifactPaths(t *testing.T) {
	layout := newProjectLayout("out")

	if got, want := layout.docsStub(), filepath.Join("out", "docs", "docs.go"); got != want {
		t.Fatalf("docsStub() = %q, want %q", got, want)
	}
	if got, want := layout.idlCopy(), filepath.Join("out", "idl.go"); got != want {
		t.Fatalf("idlCopy() = %q, want %q", got, want)
	}
	if got, want := layout.clientDemoFile("UserService"), filepath.Join("out", "client", "userservice", "demo.go"); got != want {
		t.Fatalf("clientDemoFile() = %q, want %q", got, want)
	}
	if got, want := layout.protoFile("UserService"), filepath.Join("out", "pb", "userservice", "userservice.proto"); got != want {
		t.Fatalf("protoFile() = %q, want %q", got, want)
	}
}

func TestRoutePrefix(t *testing.T) {
	tests := []struct {
		name       string
		basePrefix string
		service    string
		want       string
	}{
		{name: "empty", basePrefix: "", service: "UserService", want: ""},
		{name: "no slash", basePrefix: "api/v1", service: "UserService", want: "/api/v1/userservice"},
		{name: "leading slash", basePrefix: "/v2", service: "Greeter", want: "/v2/greeter"},
		{name: "trailing slash", basePrefix: "/api/", service: "OrderService", want: "/api/orderservice"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := routePrefix(tt.basePrefix, tt.service); got != tt.want {
				t.Fatalf("routePrefix(%q, %q) = %q, want %q", tt.basePrefix, tt.service, got, tt.want)
			}
		})
	}
}
