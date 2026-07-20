package httpsecurity

import (
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestTrustedProxyUsesOnlyTrustedForwardingChain(t *testing.T) {
	middleware, err := NewTrustedProxy(TrustedProxyConfig{TrustedProxies: []string{"10.0.0.0/8"}})
	if err != nil {
		t.Fatalf("NewTrustedProxy: %v", err)
	}
	var gotIP, gotScheme string
	handler := middleware(http.HandlerFunc(func(_ http.ResponseWriter, request *http.Request) {
		ip, ok := ClientIPFromContext(request.Context())
		if !ok {
			t.Fatal("client IP missing from context")
		}
		gotIP = ip.String()
		gotScheme, _ = SchemeFromContext(request.Context())
	}))

	request := httptest.NewRequest(http.MethodGet, "http://service.test", nil)
	request.RemoteAddr = "10.0.0.2:1234"
	request.Header.Set("X-Forwarded-For", "203.0.113.9, 10.1.0.4")
	request.Header.Set("X-Forwarded-Proto", "http, https")
	handler.ServeHTTP(httptest.NewRecorder(), request)

	if gotIP != "203.0.113.9" || gotScheme != "https" {
		t.Fatalf("effective request = %s %s", gotScheme, gotIP)
	}
}

func TestTrustedProxyIgnoresUntrustedPeerHeaders(t *testing.T) {
	middleware, err := NewTrustedProxy(TrustedProxyConfig{TrustedProxies: []string{"10.0.0.0/8"}})
	if err != nil {
		t.Fatalf("NewTrustedProxy: %v", err)
	}
	var gotIP, gotScheme string
	handler := middleware(http.HandlerFunc(func(_ http.ResponseWriter, request *http.Request) {
		ip, _ := ClientIPFromContext(request.Context())
		gotIP = ip.String()
		gotScheme, _ = SchemeFromContext(request.Context())
	}))
	request := httptest.NewRequest(http.MethodGet, "http://service.test", nil)
	request.RemoteAddr = "192.0.2.20:1234"
	request.Header.Set("X-Forwarded-For", "203.0.113.9")
	request.Header.Set("X-Forwarded-Proto", "https")
	handler.ServeHTTP(httptest.NewRecorder(), request)

	if gotIP != "192.0.2.20" || gotScheme != "http" {
		t.Fatalf("untrusted forwarding headers changed request to %s %s", gotScheme, gotIP)
	}
}

func TestTrustedProxyRejectsMalformedTrustedHeaders(t *testing.T) {
	middleware, err := NewTrustedProxy(TrustedProxyConfig{TrustedProxies: []string{"127.0.0.1"}})
	if err != nil {
		t.Fatalf("NewTrustedProxy: %v", err)
	}
	request := httptest.NewRequest(http.MethodGet, "http://service.test", nil)
	request.RemoteAddr = "127.0.0.1:1234"
	request.Header.Set("X-Forwarded-For", "not-an-ip")
	recorder := httptest.NewRecorder()
	middleware(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("next handler must not run")
	})).ServeHTTP(recorder, request)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
	}
}

func TestIPPolicyUsesEffectiveClientIP(t *testing.T) {
	proxy, err := NewTrustedProxy(TrustedProxyConfig{TrustedProxies: []string{"10.0.0.0/8"}})
	if err != nil {
		t.Fatalf("NewTrustedProxy: %v", err)
	}
	policy, err := NewIPPolicy(IPPolicyConfig{
		Allow: []string{"203.0.113.0/24"},
		Deny:  []string{"203.0.113.7"},
	})
	if err != nil {
		t.Fatalf("NewIPPolicy: %v", err)
	}
	handler := Chain(proxy, policy)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	for _, test := range []struct {
		ip   string
		want int
	}{
		{ip: "203.0.113.9", want: http.StatusNoContent},
		{ip: "203.0.113.7", want: http.StatusForbidden},
		{ip: "198.51.100.4", want: http.StatusForbidden},
	} {
		request := httptest.NewRequest(http.MethodGet, "http://service.test", nil)
		request.RemoteAddr = "10.0.0.2:1234"
		request.Header.Set("X-Forwarded-For", test.ip)
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)
		if recorder.Code != test.want {
			t.Errorf("client %s status = %d, want %d", test.ip, recorder.Code, test.want)
		}
	}
}

func TestProxyAndIPPolicyConfigurationValidation(t *testing.T) {
	if _, err := NewTrustedProxy(TrustedProxyConfig{TrustedProxies: []string{"bad-cidr"}}); err == nil {
		t.Fatal("expected invalid trusted proxy error")
	}
	if _, err := NewIPPolicy(IPPolicyConfig{Allow: []string{"bad-cidr"}}); err == nil {
		t.Fatal("expected invalid allow policy error")
	}
	if _, err := remoteIP("invalid"); err == nil {
		t.Fatal("expected invalid remote address error")
	}
	if got := cloneIP(net.ParseIP("192.0.2.1")); got.String() != "192.0.2.1" {
		t.Fatalf("cloned IP = %s", got)
	}
}
