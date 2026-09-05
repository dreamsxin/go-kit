package httpsecurity

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// sessionFromCookie is the accessor a cookie-authenticated application supplies:
// the session cookie names the session a CSRF token is minted for.
func sessionFromCookie(request *http.Request) (string, bool) {
	cookie, err := request.Cookie("session")
	if err != nil || cookie.Value == "" {
		return "", false
	}
	return cookie.Value, true
}

func TestCSRFSignedDoubleSubmitFlow(t *testing.T) {
	middleware, err := NewCSRF(CSRFConfig{
		Secret:        bytes.Repeat([]byte{0x42}, minCSRFSecretBytes),
		SessionID:     sessionFromCookie,
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
	getRequest.AddCookie(&http.Cookie{Name: "session", Value: "session-a"})
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
	postRequest.AddCookie(&http.Cookie{Name: "session", Value: "session-a"})
	postRecorder := httptest.NewRecorder()
	handler.ServeHTTP(postRecorder, postRequest)
	if postRecorder.Code != http.StatusNoContent || calls != 2 {
		t.Fatalf("POST status/calls = %d/%d", postRecorder.Code, calls)
	}
}

// TestCSRFTokenAuthorizesOneSession is the property that makes a signed token
// worth signing: knowing a valid token is not enough, it has to be this
// caller's token.
func TestCSRFTokenAuthorizesOneSession(t *testing.T) {
	handler := csrfHandler(t, CSRFConfig{
		Secret:        bytes.Repeat([]byte{0x51}, minCSRFSecretBytes),
		SessionID:     sessionFromCookie,
		RequireOrigin: true,
	})
	tokenCookie := csrfCookieFromSafeRequest(t, handler, "session-a")

	request := unsafeCSRFRequest(tokenCookie, tokenCookie.Value, "https://api.example.com")
	request.AddCookie(&http.Cookie{Name: "session", Value: "session-b"})
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusForbidden)
	}
}

// TestCSRFUnsafeRequestWithoutSessionIsRefused pins the fail-closed direction:
// a token cannot be checked against a session that is not there.
func TestCSRFUnsafeRequestWithoutSessionIsRefused(t *testing.T) {
	handler := csrfHandler(t, CSRFConfig{
		Secret:    bytes.Repeat([]byte{0x52}, minCSRFSecretBytes),
		SessionID: sessionFromCookie,
	})
	tokenCookie := csrfCookieFromSafeRequest(t, handler, "session-a")

	request := unsafeCSRFRequest(tokenCookie, tokenCookie.Value, "https://api.example.com")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusForbidden)
	}
}

// TestCSRFSafeRequestWithoutSessionMintsNoToken: before sign-in there is no
// session to bind a token to, so the token arrives with the first request that
// has one.
func TestCSRFSafeRequestWithoutSessionMintsNoToken(t *testing.T) {
	handler := csrfHandler(t, CSRFConfig{
		Secret:    bytes.Repeat([]byte{0x53}, minCSRFSecretBytes),
		SessionID: sessionFromCookie,
	})
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "https://api.example.com", nil))
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status = %d", recorder.Code)
	}
	if cookies := recorder.Result().Cookies(); len(cookies) != 0 {
		t.Fatalf("cookies = %d, want 0", len(cookies))
	}
}

// TestCSRFMintedTokenIsNotStoredForAnotherUser: the response that carries a
// per-session Set-Cookie is not a response a shared cache may replay.
func TestCSRFMintedTokenIsNotStoredForAnotherUser(t *testing.T) {
	handler := csrfHandler(t, CSRFConfig{
		Secret:    bytes.Repeat([]byte{0x54}, minCSRFSecretBytes),
		SessionID: sessionFromCookie,
	})
	request := httptest.NewRequest(http.MethodGet, "https://api.example.com", nil)
	request.AddCookie(&http.Cookie{Name: "session", Value: "session-a"})
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	header := recorder.Result().Header
	if got := header.Get("Cache-Control"); got != "no-store" {
		t.Errorf("Cache-Control = %q, want %q", got, "no-store")
	}
	if got := header.Get("Vary"); !strings.Contains(got, "Cookie") {
		t.Errorf("Vary = %q, want it to contain Cookie", got)
	}
}

// TestCSRFTokenExpires bounds the replay window of a leaked token.
func TestCSRFTokenExpires(t *testing.T) {
	policy := &csrfPolicy{
		secret:   bytes.Repeat([]byte{0x55}, minCSRFSecretBytes),
		tokenTTL: time.Hour,
	}
	now := time.Now()
	token, err := policy.newToken("session-a", now.Add(-2*time.Hour))
	if err != nil {
		t.Fatalf("newToken: %v", err)
	}
	if policy.validToken(token, "session-a", now) {
		t.Error("a token older than the configured lifetime is still accepted")
	}

	fresh, err := policy.newToken("session-a", now)
	if err != nil {
		t.Fatalf("newToken: %v", err)
	}
	if !policy.validToken(fresh, "session-a", now) {
		t.Error("a token inside its lifetime is refused")
	}
	if policy.validToken(fresh, "session-b", now) {
		t.Error("a token minted for one session is accepted for another")
	}
	if policy.validToken(fresh, "session-a", now.Add(-time.Hour)) {
		t.Error("a token from the future is accepted beyond the clock-skew allowance")
	}
}

func TestCSRFDeniesMissingTamperedAndCrossOriginTokens(t *testing.T) {
	handler := csrfHandler(t, CSRFConfig{
		Secret:    bytes.Repeat([]byte{0x23}, minCSRFSecretBytes),
		SessionID: sessionFromCookie,
	})
	tokenCookie := csrfCookieFromSafeRequest(t, handler, "session-a")

	missingToken := httptest.NewRequest(http.MethodPost, "https://api.example.com", nil)
	missingToken.AddCookie(&http.Cookie{Name: "session", Value: "session-a"})
	tests := []*http.Request{
		missingToken,
		unsafeCSRFRequest(tokenCookie, "tampered", "https://api.example.com", "session-a"),
		unsafeCSRFRequest(tokenCookie, tokenCookie.Value, "https://evil.example.com", "session-a"),
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
	handler := csrfHandler(t, CSRFConfig{
		Secret:         bytes.Repeat([]byte{0x19}, minCSRFSecretBytes),
		SessionID:      sessionFromCookie,
		TrustedOrigins: []string{"https://admin.example.com"},
		RequireOrigin:  true,
	})
	cookie := csrfCookieFromSafeRequest(t, handler, "session-a")
	request := unsafeCSRFRequest(cookie, cookie.Value, "https://admin.example.com", "session-a")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status = %d", recorder.Code)
	}
}

// TestCSRFSameOriginHoldsBehindATLSTerminatingProxy: the process serves
// plaintext while the browser reports an https origin, and AssumeHTTPS is how
// the deployment says so.
func TestCSRFSameOriginHoldsBehindATLSTerminatingProxy(t *testing.T) {
	handler := csrfHandler(t, CSRFConfig{
		Secret:        bytes.Repeat([]byte{0x56}, minCSRFSecretBytes),
		SessionID:     sessionFromCookie,
		AssumeHTTPS:   true,
		RequireOrigin: true,
	})
	getRequest := httptest.NewRequest(http.MethodGet, "http://api.example.com", nil)
	getRequest.TLS = nil
	getRequest.AddCookie(&http.Cookie{Name: "session", Value: "session-a"})
	getRecorder := httptest.NewRecorder()
	handler.ServeHTTP(getRecorder, getRequest)
	cookies := getRecorder.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("cookies = %d, want 1", len(cookies))
	}

	request := httptest.NewRequest(http.MethodPost, "http://api.example.com", nil)
	request.TLS = nil
	request.AddCookie(cookies[0])
	request.AddCookie(&http.Cookie{Name: "session", Value: "session-a"})
	request.Header.Set(defaultCSRFHeader, cookies[0].Value)
	request.Header.Set("Origin", "https://api.example.com")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNoContent)
	}
}

func TestCSRFConfigurationValidation(t *testing.T) {
	tests := []CSRFConfig{
		{},
		{Secret: bytes.Repeat([]byte{1}, minCSRFSecretBytes)},
		{Secret: bytes.Repeat([]byte{1}, minCSRFSecretBytes), SessionID: sessionFromCookie, TokenTTL: -time.Second},
		{Secret: bytes.Repeat([]byte{1}, minCSRFSecretBytes), SessionID: sessionFromCookie, CookieName: "bad cookie"},
		{Secret: bytes.Repeat([]byte{1}, minCSRFSecretBytes), SessionID: sessionFromCookie, HeaderName: "bad header"},
		{Secret: bytes.Repeat([]byte{1}, minCSRFSecretBytes), SessionID: sessionFromCookie, TrustedOrigins: []string{"null"}},
		{Secret: bytes.Repeat([]byte{1}, minCSRFSecretBytes), SessionID: sessionFromCookie, SameSite: http.SameSiteNoneMode},
		{Secret: bytes.Repeat([]byte{1}, minCSRFSecretBytes), SessionID: sessionFromCookie, CookieName: "__Secure-csrf"},
	}
	for _, config := range tests {
		if _, err := NewCSRF(config); err == nil {
			t.Errorf("expected error for %#v", config)
		}
	}
}

func csrfHandler(t *testing.T, config CSRFConfig) http.Handler {
	t.Helper()
	middleware, err := NewCSRF(config)
	if err != nil {
		t.Fatalf("NewCSRF: %v", err)
	}
	return middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
}

func csrfCookieFromSafeRequest(t *testing.T, handler http.Handler, session string) *http.Cookie {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, "https://api.example.com", nil)
	if session != "" {
		request.AddCookie(&http.Cookie{Name: "session", Value: session})
	}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	cookies := recorder.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("cookies = %d, want 1", len(cookies))
	}
	return cookies[0]
}

func unsafeCSRFRequest(cookie *http.Cookie, headerToken, origin string, sessions ...string) *http.Request {
	request := httptest.NewRequest(http.MethodPost, "https://api.example.com", nil)
	request.AddCookie(cookie)
	for _, session := range sessions {
		request.AddCookie(&http.Cookie{Name: "session", Value: session})
	}
	request.Header.Set(defaultCSRFHeader, headerToken)
	request.Header.Set("Origin", origin)
	return request
}
