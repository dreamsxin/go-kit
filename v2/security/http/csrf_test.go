package httpsecurity

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCSRFSignedDoubleSubmitFlow(t *testing.T) {
	middleware, err := NewCSRF(CSRFConfig{
		Secret:        bytes.Repeat([]byte{0x42}, minCSRFSecretBytes),
		RequireOrigin: true,
	})
	if err != nil {
		t.Fatalf("NewCSRF: %v", err)
	}
	calls := 0
	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.WriteHeader(http.StatusNoContent)
	}))

	getRequest := httptest.NewRequest(http.MethodGet, "https://api.example.com/session", nil)
	getRecorder := httptest.NewRecorder()
	handler.ServeHTTP(getRecorder, getRequest)
	if getRecorder.Code != http.StatusNoContent || calls != 1 {
		t.Fatalf("GET status/calls = %d/%d", getRecorder.Code, calls)
	}
	cookies := getRecorder.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("cookies = %d, want 1", len(cookies))
	}
	tokenCookie := cookies[0]
	if tokenCookie.HttpOnly || tokenCookie.SameSite != http.SameSiteLaxMode || tokenCookie.Path != "/" {
		t.Fatalf("cookie = %#v", tokenCookie)
	}

	postRequest := httptest.NewRequest(http.MethodPost, "https://api.example.com/session", nil)
	postRequest.Header.Set("Origin", "https://api.example.com")
	postRequest.Header.Set(defaultCSRFHeader, tokenCookie.Value)
	postRequest.AddCookie(tokenCookie)
	postRecorder := httptest.NewRecorder()
	handler.ServeHTTP(postRecorder, postRequest)
	if postRecorder.Code != http.StatusNoContent || calls != 2 {
		t.Fatalf("POST status/calls = %d/%d", postRecorder.Code, calls)
	}
}

func TestCSRFDeniesMissingTamperedAndCrossOriginTokens(t *testing.T) {
	middleware, err := NewCSRF(CSRFConfig{Secret: bytes.Repeat([]byte{0x23}, minCSRFSecretBytes)})
	if err != nil {
		t.Fatalf("NewCSRF: %v", err)
	}
	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	tokenCookie := csrfCookieFromSafeRequest(t, handler)

	tests := []*http.Request{
		httptest.NewRequest(http.MethodPost, "https://api.example.com", nil),
		unsafeCSRFRequest(tokenCookie, "tampered", "https://api.example.com"),
		unsafeCSRFRequest(tokenCookie, tokenCookie.Value, "https://evil.example.com"),
	}
	for _, request := range tests {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusForbidden {
			t.Errorf("status = %d, want %d", recorder.Code, http.StatusForbidden)
		}
	}
}

func TestCSRFAllowsConfiguredTrustedOrigin(t *testing.T) {
	middleware, err := NewCSRF(CSRFConfig{
		Secret:         bytes.Repeat([]byte{0x19}, minCSRFSecretBytes),
		TrustedOrigins: []string{"https://admin.example.com"},
		RequireOrigin:  true,
	})
	if err != nil {
		t.Fatalf("NewCSRF: %v", err)
	}
	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	cookie := csrfCookieFromSafeRequest(t, handler)
	request := unsafeCSRFRequest(cookie, cookie.Value, "https://admin.example.com")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status = %d", recorder.Code)
	}
}

func TestCSRFConfigurationValidation(t *testing.T) {
	tests := []CSRFConfig{
		{},
		{Secret: bytes.Repeat([]byte{1}, minCSRFSecretBytes), CookieName: "bad cookie"},
		{Secret: bytes.Repeat([]byte{1}, minCSRFSecretBytes), HeaderName: "bad header"},
		{Secret: bytes.Repeat([]byte{1}, minCSRFSecretBytes), TrustedOrigins: []string{"null"}},
		{Secret: bytes.Repeat([]byte{1}, minCSRFSecretBytes), SameSite: http.SameSiteNoneMode},
		{Secret: bytes.Repeat([]byte{1}, minCSRFSecretBytes), CookieName: "__Secure-csrf"},
	}
	for _, config := range tests {
		if _, err := NewCSRF(config); err == nil {
			t.Errorf("expected error for %#v", config)
		}
	}
}

func csrfCookieFromSafeRequest(t *testing.T, handler http.Handler) *http.Cookie {
	t.Helper()
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "https://api.example.com", nil))
	cookies := recorder.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("cookies = %d, want 1", len(cookies))
	}
	return cookies[0]
}

func unsafeCSRFRequest(cookie *http.Cookie, headerToken, origin string) *http.Request {
	request := httptest.NewRequest(http.MethodPost, "https://api.example.com", nil)
	request.AddCookie(cookie)
	request.Header.Set(defaultCSRFHeader, headerToken)
	request.Header.Set("Origin", origin)
	return request
}
