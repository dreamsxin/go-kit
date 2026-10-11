package generator_test

import (
	"encoding/json"
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

// TestOpenAPISecurityDeclaredForSecuredMethods is the contract half of the
// auth feature: the secured operation carries the security requirement, the
// components declare the bearer scheme, and 401 documents the failure the
// enforcement produces.
func TestOpenAPISecurityDeclaredForSecuredMethods(t *testing.T) {
	root := generateAuthedProject(t)
	var document struct {
		Paths map[string]map[string]struct {
			Security  []map[string][]string `json:"security"`
			Responses map[string]struct {
				Description string `json:"description"`
			} `json:"responses"`
		} `json:"paths"`
		Components struct {
			SecuritySchemes map[string]struct {
				Type   string `json:"type"`
				Scheme string `json:"scheme"`
			} `json:"securitySchemes"`
		} `json:"components"`
	}
	if err := json.Unmarshal([]byte(readFile(t, filepath.Join(root, "docs", "openapi.json"))), &document); err != nil {
		t.Fatalf("parse openapi document: %v", err)
	}
	secured := document.Paths["/secured"]["post"]
	if len(secured.Security) != 1 || len(secured.Security[0]["auth"]) != 0 {
		t.Errorf("secured operation security = %v, want [{auth:[]}]", secured.Security)
	}
	if _, ok := secured.Responses["401"]; !ok {
		t.Error("the secured operation does not document 401")
	}
	if scheme, ok := document.Components.SecuritySchemes["auth"]; !ok || scheme.Type != "http" || scheme.Scheme != "bearer" {
		t.Errorf("security scheme = %+v, want the http bearer scheme", scheme)
	}
}

// TestOpenAPISecurityAbsentForUnsecuredMethods is the other half: the
// enforcement is per-method, and so is the documentation — an undeclared
// method carries no security and no 401.
func TestOpenAPISecurityAbsentForUnsecuredMethods(t *testing.T) {
	root := generateAuthedProject(t)
	doc := readFile(t, filepath.Join(root, "docs", "openapi.json"))
	secured := strings.Index(doc, `/secured`)
	unsecured := strings.Index(doc, `/ping`)
	if secured < 0 || unsecured < 0 {
		t.Fatalf("operations missing from the document")
	}
	// The Ping operation appears before Secured; scan its slice of the
	// document for a security requirement.
	segment := doc[unsecured:secured]
	if strings.Contains(segment, `"security"`) {
		t.Errorf("the unsecured operation declares security:\n%s", segment)
	}
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
