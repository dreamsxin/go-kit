package httpsecurity

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestCORSPreflightAndActualRequest(t *testing.T) {
	middleware, err := NewCORS(CORSConfig{
		AllowedOrigins:   []string{"https://app.example.com"},
		AllowedMethods:   []string{http.MethodGet, http.MethodPost},
		AllowedHeaders:   []string{"Content-Type", "X-CSRF-Token"},
		ExposedHeaders:   []string{"X-Request-ID"},
		AllowCredentials: true,
		MaxAge:           10 * time.Minute,
	})
	if err != nil {
		t.Fatalf("NewCORS: %v", err)
	}
	calls := 0
	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.WriteHeader(http.StatusCreated)
	}))

	preflight := httptest.NewRequest(http.MethodOptions, "https://api.example.com/users", nil)
	preflight.Header.Set("Origin", "https://app.example.com")
	preflight.Header.Set("Access-Control-Request-Method", http.MethodPost)
	preflight.Header.Set("Access-Control-Request-Headers", "content-type, x-csrf-token")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, preflight)
	if recorder.Code != http.StatusNoContent || calls != 0 {
		t.Fatalf("preflight status/calls = %d/%d", recorder.Code, calls)
	}
	if recorder.Header().Get("Access-Control-Allow-Origin") != "https://app.example.com" ||
		recorder.Header().Get("Access-Control-Allow-Credentials") != "true" ||
		recorder.Header().Get("Access-Control-Max-Age") != "600" {
		t.Fatalf("preflight headers = %#v", recorder.Header())
	}
	if vary := strings.Join(recorder.Header().Values("Vary"), ","); !strings.Contains(vary, "Origin") || !strings.Contains(vary, "Access-Control-Request-Headers") {
		t.Fatalf("Vary = %q", vary)
	}

	actual := httptest.NewRequest(http.MethodGet, "https://api.example.com/users", nil)
	actual.Header.Set("Origin", "https://app.example.com")
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, actual)
	if recorder.Code != http.StatusCreated || calls != 1 {
		t.Fatalf("actual status/calls = %d/%d", recorder.Code, calls)
	}
	if recorder.Header().Get("Access-Control-Expose-Headers") != "X-Request-Id" {
		t.Fatalf("exposed headers = %q", recorder.Header().Get("Access-Control-Expose-Headers"))
	}
}

func TestCORSDeniesUnknownOriginMethodAndHeaders(t *testing.T) {
	middleware, err := NewCORS(CORSConfig{AllowedOrigins: []string{"https://app.example.com"}})
	if err != nil {
		t.Fatalf("NewCORS: %v", err)
	}
	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }))

	tests := []*http.Request{
		httpRequestWithHeaders(http.MethodGet, "Origin", "https://evil.example.com"),
		httpRequestWithHeaders(http.MethodDelete, "Origin", "https://app.example.com"),
		httpRequestWithHeaders(http.MethodOptions,
			"Origin", "https://app.example.com",
			"Access-Control-Request-Method", http.MethodPost,
			"Access-Control-Request-Headers", "X-Not-Allowed",
		),
	}
	for _, request := range tests {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusForbidden {
			t.Errorf("%s request status = %d", request.Method, recorder.Code)
		}
	}
}

func TestCORSConfigurationValidation(t *testing.T) {
	tests := []CORSConfig{
		{},
		{AllowedOrigins: []string{"*"}, AllowCredentials: true},
		{AllowedOrigins: []string{"https://app.example.com/path"}},
		{AllowedOrigins: []string{"https://app.example.com"}, AllowedMethods: []string{"bad method"}},
		{AllowedOrigins: []string{"https://app.example.com"}, AllowedHeaders: []string{"bad header"}},
		{AllowedOrigins: []string{"https://app.example.com"}, MaxAge: -time.Second},
	}
	for _, config := range tests {
		if _, err := NewCORS(config); err == nil {
			t.Errorf("expected error for %#v", config)
		}
	}
}

func TestCORSWithoutOriginPassesThrough(t *testing.T) {
	middleware, err := NewCORS(CORSConfig{AllowedOrigins: []string{"*"}})
	if err != nil {
		t.Fatalf("NewCORS: %v", err)
	}
	recorder := httptest.NewRecorder()
	middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusAccepted)
	})).ServeHTTP(recorder, httptest.NewRequest(http.MethodDelete, "http://service.test", nil))
	if recorder.Code != http.StatusAccepted {
		t.Fatalf("status = %d", recorder.Code)
	}
}

func httpRequestWithHeaders(method string, values ...string) *http.Request {
	request := httptest.NewRequest(method, "https://api.example.com", nil)
	for i := 0; i < len(values); i += 2 {
		request.Header.Set(values[i], values[i+1])
	}
	return request
}
