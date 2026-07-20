package httpsecurity_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dreamsxin/go-kit/v2/kit"
	httpsecurity "github.com/dreamsxin/go-kit/v2/security/http"
)

func TestKitInstallsSecurityMiddlewareAcrossRoutes(t *testing.T) {
	headers, err := httpsecurity.NewSecurityHeaders(httpsecurity.SecurityHeadersConfig{})
	if err != nil {
		t.Fatalf("NewSecurityHeaders: %v", err)
	}
	service, err := kit.New(":0", kit.WithHTTPMiddleware(headers))
	if err != nil {
		t.Fatalf("kit.New: %v", err)
	}
	recorder := httptest.NewRecorder()
	service.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/health", nil))
	if recorder.Code != http.StatusOK || recorder.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("health response = %d %#v", recorder.Code, recorder.Header())
	}
}
