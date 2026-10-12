package parser

import (
	"os"
	"strings"
	"testing"
)

func writeIDLTemp(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	p := dir + "/inline.go"
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return p
}

func parseFull(t *testing.T, content string) *ParseResult {
	t.Helper()
	result, err := ParseFull(writeIDLTemp(t, content))
	if err != nil {
		t.Fatalf("ParseFull: %v", err)
	}
	return result
}

type Method2 [2]string

const routeIDL = `package admin

import "context"

type PageRequest struct {
	Page int ` + "`json:\"page\"`" + `
}

type UserPageResponse struct{}

type User struct{}

type AdminService interface {
	// 用户分页
	//
	// route: POST /user/page
	UserPage(ctx context.Context, req PageRequest) (UserPageResponse, error)

	// 删除用户
	//
	// route: DELETE /user/{id}
	UserDelete(ctx context.Context, req PageRequest) (UserPageResponse, error)

	// 未声明 route: 的方法保持派生路由
	UserCreate(ctx context.Context, req PageRequest) (UserPageResponse, error)
}
`

// TestRouteDirective: route: overrides the derived method and path, so a
// generated service can match an existing frontend's contract exactly.
func TestRouteDirective(t *testing.T) {
	result, err := ParseFull(writeIDLTemp(t, routeIDL))
	if err != nil {
		t.Fatalf("ParseFull: %v", err)
	}
	svc := result.Services[0]
	byName := map[string]Method2{}
	for _, m := range svc.Methods {
		byName[m.Name] = Method2{m.HTTPMethod, m.Route}
	}
	if got := byName["UserPage"]; got[0] != "POST" || got[1] != "/user/page" {
		t.Errorf("UserPage = %v, want [POST /user/page]", got)
	}
	if got := byName["UserDelete"]; got[0] != "DELETE" || got[1] != "/user/{id}" {
		t.Errorf("UserDelete = %v, want [DELETE /user/{id}]", got)
	}
	if got := byName["UserCreate"]; got[0] != "post" || got[1] != "/usercreate" {
		t.Errorf("UserCreate = %v, want the derived [POST /usercreate]", got)
	}
}

// TestRouteDirectiveRefusesBroken: a malformed route: directive is a
// generation error, never silently ignored.
func TestRouteDirectiveRefusesBroken(t *testing.T) {
	for _, broken := range []string{
		strings.Replace(routeIDL, "route: POST /user/page", "route: POST/user/page", 1),
		strings.Replace(routeIDL, "route: POST /user/page", "route: FETCH /user/page", 1),
		strings.Replace(routeIDL, "route: POST /user/page", "route: POST user/page", 1),
	} {
		if _, err := ParseFull(writeIDLTemp(t, broken)); err == nil {
			t.Error("a broken route: directive was not refused")
		}
	}
}
