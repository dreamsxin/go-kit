package generator_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dreamsxin/go-kit/v2/cmd/microgen/internal/generator"
)

const authIDL = `package shop

import "context"

type PingRequest struct{}

type PingResponse struct{ Pong string }

type SecuredRequest struct{}

type SecuredResponse struct{ Ok bool }

type AuthedService interface {
	Ping(ctx context.Context, req PingRequest) (PingResponse, error)
	// Secured
	Secured(ctx context.Context, req SecuredRequest) (SecuredResponse, error)
}
`

func generateAuthedProject(t *testing.T) string {
	t.Helper()
	root := newTmpDir(t)
	gen := mustNewGenerator(t, generator.Options{
		OutputDir:   root,
		ImportPath:  "example.com/authed",
		Protocols:   []string{"http"},
		WithDocs:    true,
		WithOpenAPI: true,
		IDLSrcPath:  filepath.Join(root, "idl.go"),
	})
	if err := os.WriteFile(filepath.Join(root, "idl.go"), []byte(authIDL), 0o644); err != nil {
		t.Fatalf("write contract copy: %v", err)
	}
	// The Secured method declares auth: required.
	annotated := strings.Replace(authIDL,
		"// Secured",
		"//\n\t// auth: required\n\t// Secured",
		1)
	if err := os.WriteFile(filepath.Join(root, "idl.go"), []byte(annotated), 0o644); err != nil {
		t.Fatalf("write annotated contract: %v", err)
	}
	project := parseIDLContentProject(t, annotated)
	for _, svc := range project.Services {
		for _, m := range svc.Methods {
			t.Logf("method %s auth=%+v", m.Name, m.Auth)
		}
	}
	if err := gen.GenerateIR(project); err != nil {
		t.Fatalf("GenerateIR: %v", err)
	}
	for _, svc := range project.Services {
		for _, m := range svc.Methods {
			t.Logf("post-gen %s auth declared=%v", m.Name, m.Auth.Declared())
		}
	}
	return root
}

// TestGeneratedTransportEnforcesDeclaredAuth is the enforcement half: the
// secured method's endpoint is wrapped with the service's EnforceAuth, the
// hook exists in the user-owned service file with a fail-closed default, and
// the unsecured method is untouched.
func TestGeneratedTransportEnforcesDeclaredAuth(t *testing.T) {
	root := generateAuthedProject(t)
	endpoints := readFile(t, filepath.Join(root, "endpoint", "authedservice", "endpoints.go"))
	if !strings.Contains(endpoints, `svc.EnforceAuth(ep, "Secured", []string{})`) {
		t.Errorf("the secured method is not wrapped:\n%s", endpoints)
	}
	if strings.Contains(endpoints, `EnforceAuth(ep, "Ping"`) {
		t.Error("the unsecured method is wrapped; enforcement must be per-declaration")
	}
	service := readFile(t, filepath.Join(root, "service", "authedservice", "service.go"))
	if !strings.Contains(service, "func NewAuthenticator() security.Authenticator { return nil }") {
		t.Error("the authenticator hook is missing or not fail-closed by default")
	}
	if !strings.Contains(service, "func EnforceAuth(ep endpoint.Endpoint, method string, roles []string) endpoint.Endpoint {") {
		t.Error("the EnforceAuth helper is missing from the service file")
	}
}
