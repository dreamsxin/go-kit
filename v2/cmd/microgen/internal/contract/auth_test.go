package contract

import (
	"testing"

	"github.com/dreamsxin/go-kit/v2/cmd/microgen/internal/ir"
)

// openAPIAuthProject is the smallest contract that exercises declared auth: one
// unsecured method, one secured method, and one secured method with a role.
func openAPIAuthProject() *ir.Project {
	pingRequest := &ir.Message{Name: "PingRequest"}
	pingResponse := &ir.Message{Name: "PingResponse"}
	securedRequest := &ir.Message{Name: "SecuredRequest"}
	securedResponse := &ir.Message{Name: "SecuredResponse"}
	return &ir.Project{
		PackageName: "shop",
		Messages:    []*ir.Message{pingRequest, pingResponse, securedRequest, securedResponse},
		Services: []*ir.Service{{
			Name:        "AuthedService",
			Title:       "Authed API",
			Description: "Auth enforcement",
			Methods: []*ir.Method{
				{
					Name: "Ping", Kind: ir.MethodKindUnary, HTTPMethod: "POST", Route: "/ping",
					InputName: pingRequest.Name, OutputName: pingResponse.Name,
					Input: pingRequest, Output: pingResponse,
				},
				{
					Name: "Secured", Kind: ir.MethodKindUnary, HTTPMethod: "POST", Route: "/secured",
					InputName: securedRequest.Name, OutputName: securedResponse.Name,
					Input: securedRequest, Output: securedResponse,
					Auth: ir.MethodAuth{Required: true},
				},
				{
					Name: "Admin", Kind: ir.MethodKindUnary, HTTPMethod: "POST", Route: "/admin",
					InputName: securedRequest.Name, OutputName: securedResponse.Name,
					Input: securedRequest, Output: securedResponse,
					Auth: ir.MethodAuth{Required: true, Roles: []string{"admin"}},
				},
			},
		}},
	}
}

// TestOpenAPISecurityDeclaredForSecuredMethods is the contract half of the auth
// feature: the secured operation carries the security requirement, the
// components declare the http bearer scheme named auth, and 401 (403 with a
// role) documents the failure the enforcement produces.
func TestOpenAPISecurityDeclaredForSecuredMethods(t *testing.T) {
	doc := BuildOpenAPIDocument(openAPIAuthProject(), "")

	secured := doc.Paths["/secured"]["post"]
	if len(secured.Security) != 1 || len(secured.Security[0]["auth"]) != 0 {
		t.Errorf("secured operation security = %v, want [{auth:[]}]", secured.Security)
	}
	if _, ok := secured.Responses["401"]; !ok {
		t.Error("the secured operation does not document 401")
	}
	if _, ok := secured.Responses["403"]; ok {
		t.Error("a method without roles must not document 403")
	}

	admin := doc.Paths["/admin"]["post"]
	if _, ok := admin.Responses["401"]; !ok {
		t.Error("the role-secured operation does not document 401")
	}
	if _, ok := admin.Responses["403"]; !ok {
		t.Error("the role-secured operation does not document 403")
	}

	scheme, ok := doc.Components.SecuritySchemes["auth"]
	if !ok || scheme.Type != "http" || scheme.Scheme != "bearer" {
		t.Errorf("security scheme = %+v, want the http bearer scheme", scheme)
	}
}

// TestOpenAPISecurityAbsentForUnsecuredMethods is the other half: the
// enforcement is per-method, and so is the documentation — an undeclared method
// carries no security, no 401 and no 403.
func TestOpenAPISecurityAbsentForUnsecuredMethods(t *testing.T) {
	doc := BuildOpenAPIDocument(openAPIAuthProject(), "")

	ping := doc.Paths["/ping"]["post"]
	if len(ping.Security) != 0 {
		t.Errorf("the unsecured operation declares security: %v", ping.Security)
	}
	for _, status := range []string{"401", "403"} {
		if _, ok := ping.Responses[status]; ok {
			t.Errorf("the unsecured operation documents %s", status)
		}
	}
}
