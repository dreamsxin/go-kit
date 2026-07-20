package httpsecurity

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSecurityHeadersDefaultsAndHTTPSOnlyHSTS(t *testing.T) {
	middleware, err := NewSecurityHeaders(SecurityHeadersConfig{
		ContentSecurityPolicy:   "default-src 'none'",
		StrictTransportSecurity: "max-age=31536000; includeSubDomains",
	})
	if err != nil {
		t.Fatalf("NewSecurityHeaders: %v", err)
	}
	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	httpRecorder := httptest.NewRecorder()
	handler.ServeHTTP(httpRecorder, httptest.NewRequest(http.MethodGet, "http://service.test", nil))
	if httpRecorder.Header().Get("Strict-Transport-Security") != "" {
		t.Fatal("HSTS must not be emitted over HTTP")
	}
	if httpRecorder.Header().Get("X-Content-Type-Options") != "nosniff" ||
		httpRecorder.Header().Get("X-Frame-Options") != "DENY" ||
		httpRecorder.Header().Get("Content-Security-Policy") != "default-src 'none'" {
		t.Fatalf("default headers = %#v", httpRecorder.Header())
	}

	httpsRecorder := httptest.NewRecorder()
	handler.ServeHTTP(httpsRecorder, httptest.NewRequest(http.MethodGet, "https://service.test", nil))
	if httpsRecorder.Header().Get("Strict-Transport-Security") == "" {
		t.Fatal("HSTS missing over HTTPS")
	}
}

func TestSecurityHeadersTrustEffectiveProxyScheme(t *testing.T) {
	proxy, err := NewTrustedProxy(TrustedProxyConfig{TrustedProxies: []string{"127.0.0.1"}})
	if err != nil {
		t.Fatalf("NewTrustedProxy: %v", err)
	}
	headers, err := NewSecurityHeaders(SecurityHeadersConfig{StrictTransportSecurity: "max-age=60"})
	if err != nil {
		t.Fatalf("NewSecurityHeaders: %v", err)
	}
	request := httptest.NewRequest(http.MethodGet, "http://service.test", nil)
	request.RemoteAddr = "127.0.0.1:1234"
	request.Header.Set("X-Forwarded-Proto", "https")
	recorder := httptest.NewRecorder()
	Chain(proxy, headers)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})).ServeHTTP(recorder, request)
	if recorder.Header().Get("Strict-Transport-Security") != "max-age=60" {
		t.Fatalf("HSTS = %q", recorder.Header().Get("Strict-Transport-Security"))
	}
}

func TestSecurityHeadersRejectHeaderInjection(t *testing.T) {
	if _, err := NewSecurityHeaders(SecurityHeadersConfig{ContentSecurityPolicy: "default-src 'none'\r\nX-Bad: yes"}); err == nil {
		t.Fatal("expected invalid header value error")
	}
}

func TestChainPreservesDeclarationOrder(t *testing.T) {
	var order []string
	middleware := func(name string) Middleware {
		return func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
				order = append(order, name+":before")
				next.ServeHTTP(w, request)
				order = append(order, name+":after")
			})
		}
	}
	Chain(middleware("first"), nil, middleware("second"))(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		order = append(order, "handler")
	})).ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "http://service.test", nil))
	if got := strings.Join(order, ","); got != "first:before,second:before,handler,second:after,first:after" {
		t.Fatalf("order = %s", got)
	}
}

func TestSecurityMiddlewarePreservesStreamingInterfaces(t *testing.T) {
	proxy, err := NewTrustedProxy(TrustedProxyConfig{})
	if err != nil {
		t.Fatalf("NewTrustedProxy: %v", err)
	}
	headers, err := NewSecurityHeaders(SecurityHeadersConfig{})
	if err != nil {
		t.Fatalf("NewSecurityHeaders: %v", err)
	}
	ipPolicy, err := NewIPPolicy(IPPolicyConfig{})
	if err != nil {
		t.Fatalf("NewIPPolicy: %v", err)
	}
	cors, err := NewCORS(CORSConfig{AllowedOrigins: []string{"*"}})
	if err != nil {
		t.Fatalf("NewCORS: %v", err)
	}
	csrf, err := NewCSRF(CSRFConfig{Secret: bytes.Repeat([]byte{0x51}, minCSRFSecretBytes)})
	if err != nil {
		t.Fatalf("NewCSRF: %v", err)
	}

	streamed := false
	handler := Chain(proxy, headers, ipPolicy, cors, csrf)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Fatal("http.Flusher was hidden")
		}
		flusher.Flush()
		streamed = true
	}))
	request := httptest.NewRequest(http.MethodGet, "http://service.test/events", nil)
	request.Header.Set("Origin", "https://app.example.com")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if !streamed || !recorder.Flushed {
		t.Fatalf("streamed/flushed = %v/%v", streamed, recorder.Flushed)
	}
}
