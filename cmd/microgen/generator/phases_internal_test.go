package generator

import (
	"path/filepath"
	"testing"

	"github.com/dreamsxin/go-kit/cmd/microgen/ir"
)

func TestShouldCopyIDLSource(t *testing.T) {
	tests := []struct {
		name    string
		idlPath string
		want    bool
	}{
		{name: "empty", idlPath: "", want: false},
		{name: "go idl", idlPath: "service.go", want: true},
		{name: "proto", idlPath: "service.proto", want: false},
		{name: "nested go idl", idlPath: "api/idl.go", want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := &Generator{config: Options{IDLSrcPath: tt.idlPath}}
			if got := g.shouldCopyIDLSource(); got != tt.want {
				t.Fatalf("shouldCopyIDLSource() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRootRelativePath(t *testing.T) {
	tests := []struct {
		name      string
		outputDir string
		want      string
	}{
		{name: "external", outputDir: t.TempDir(), want: ""},
	}
	if root := findGoKitModuleRoot("."); root != "" {
		tests = append(tests,
			struct {
				name      string
				outputDir string
				want      string
			}{name: "repo examples", outputDir: filepath.Join(root, "examples", "demo"), want: "../.."},
			struct {
				name      string
				outputDir string
				want      string
			}{name: "repo tools testdata", outputDir: filepath.Join(root, "tools", "testdata", "gen"), want: "../../.."},
		)
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := &Generator{outputDir: tt.outputDir}
			if got := g.rootRelativePath(); got != tt.want {
				t.Fatalf("rootRelativePath() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestServiceRoutes(t *testing.T) {
	g := &Generator{config: Options{RoutePrefix: "/api/v1"}}
	project := &ir.Project{
		Services: []*ir.Service{
			{Name: "UserService", PackageName: "userservice"},
			{Name: "OrderService", PackageName: "orderservice"},
		},
	}

	routes := g.serviceRoutes(project)
	if len(routes) != 2 {
		t.Fatalf("len(routes) = %d, want 2", len(routes))
	}

	if routes[0].Service != project.Services[0] || routes[0].FullPrefix != "/api/v1/userservice" {
		t.Fatalf("first route = %+v, want service UserService with /api/v1/userservice", routes[0])
	}
	if routes[1].Service != project.Services[1] || routes[1].FullPrefix != "/api/v1/orderservice" {
		t.Fatalf("second route = %+v, want service OrderService with /api/v1/orderservice", routes[1])
	}
}
