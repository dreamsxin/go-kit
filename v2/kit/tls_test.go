package kit_test

import (
	"context"
	"crypto/ecdsa"

	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"io"
	"math/big"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dreamsxin/go-kit/v2/kit"
)

// selfSignedCert writes a certificate and key for 127.0.0.1 to a temporary
// directory and returns their paths plus a pool that trusts the certificate.
func selfSignedCert(t *testing.T) (certPath, keyPath string, pool *x509.CertPool) {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	template := x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "go-kit test"},
		NotBefore:             time.Now().Add(-time.Minute),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1")},
		DNSNames:              []string{"localhost"},
		IsCA:                  true,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create certificate: %v", err)
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatalf("marshal key: %v", err)
	}

	dir := t.TempDir()
	certPath = filepath.Join(dir, "cert.pem")
	keyPath = filepath.Join(dir, "key.pem")
	if err := os.WriteFile(certPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o600); err != nil {
		t.Fatalf("write certificate: %v", err)
	}
	if err := os.WriteFile(keyPath, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}), 0o600); err != nil {
		t.Fatalf("write key: %v", err)
	}

	pool = x509.NewCertPool()
	certificate, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("parse certificate: %v", err)
	}
	pool.AddCert(certificate)
	return certPath, keyPath, pool
}

// TestTLSServesHTTPSAndNegotiatesHTTP2 proves the listener terminates TLS itself,
// and that HTTP/2 arrives with it: over TLS the protocol comes from ALPN, so a
// service that turns on TLS changes protocol version at the same time — which is
// exactly the surprise this milestone exists to write down.
func TestTLSServesHTTPSAndNegotiatesHTTP2(t *testing.T) {
	certPath, keyPath, pool := selfSignedCert(t)
	component := kit.MustNewHTTP("127.0.0.1:0", kit.WithTLS(certPath, keyPath))
	component.HandleFunc("GET /hello", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	if !component.ServesTLS() {
		t.Fatal("ServesTLS() = false after WithTLS")
	}
	if err := component.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer component.Shutdown(context.Background()) //nolint:errcheck

	client := &http.Client{Transport: &http.Transport{
		TLSClientConfig:   &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12},
		ForceAttemptHTTP2: true,
	}}
	resp, err := client.Get("https://" + component.Addr() + "/hello")
	if err != nil {
		t.Fatalf("GET over TLS: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	if resp.Proto != "HTTP/2.0" {
		t.Errorf("proto = %s, want HTTP/2.0 negotiated through ALPN", resp.Proto)
	}
	if resp.TLS == nil || resp.TLS.Version < tls.VersionTLS12 {
		t.Errorf("connection state = %+v, want at least TLS 1.2", resp.TLS)
	}

	// A plaintext request to a TLS listener is answered, not dropped: Go's server
	// recognises the HTTP preamble on a TLS port and replies 400 with a message
	// naming the mistake. Worth pinning, because the behaviour a reader might
	// assume — a closed connection — is far harder to diagnose from the client.
	plain := &http.Client{Timeout: 2 * time.Second}
	plainResp, err := plain.Get("http://" + component.Addr() + "/hello")
	if err != nil {
		t.Fatalf("plaintext request to a TLS listener: %v", err)
	}
	defer plainResp.Body.Close()
	if plainResp.StatusCode != http.StatusBadRequest {
		t.Errorf("plaintext status = %d, want 400", plainResp.StatusCode)
	}
	body, _ := io.ReadAll(plainResp.Body)
	if !strings.Contains(string(body), "HTTPS server") {
		t.Errorf("plaintext body = %q, want it to name the mistake", body)
	}
}

// TestTLSCertificateFailureNamesThePath proves the failure lands where it can be
// acted on. A wrong path that survives construction becomes a handshake error in
// somebody else's client log.
func TestTLSCertificateFailureNamesThePath(t *testing.T) {
	certPath, keyPath, _ := selfSignedCert(t)
	missing := filepath.Join(filepath.Dir(certPath), "absent.pem")

	_, err := kit.NewHTTP("127.0.0.1:0", kit.WithTLS(missing, keyPath))
	if err == nil {
		t.Fatal("NewHTTP accepted a certificate path that does not exist")
	}
	if !strings.Contains(err.Error(), missing) {
		t.Errorf("error = %v, want the offending path", err)
	}

	// A key that does not match its certificate is the other half of the same
	// mistake, and just as invisible until a handshake.
	otherCert, _, _ := selfSignedCert(t)
	if _, err := kit.NewHTTP("127.0.0.1:0", kit.WithTLS(otherCert, keyPath)); err == nil {
		t.Fatal("NewHTTP accepted a key that does not match its certificate")
	}

	if _, err := kit.NewHTTP("127.0.0.1:0", kit.WithTLS("", "")); err == nil {
		t.Fatal("NewHTTP accepted empty certificate paths")
	}
}

// TestTLSConfigIsServedAsGiven proves policy stays with the deployment: what it
// built is what is served, apart from a zero MinVersion that no server means to
// ask for.
func TestTLSConfigIsServedAsGiven(t *testing.T) {
	certPath, keyPath, pool := selfSignedCert(t)
	pair, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		t.Fatalf("LoadX509KeyPair: %v", err)
	}

	supplied := &tls.Config{
		Certificates: []tls.Certificate{pair},
		// A deployment that wants HTTP/1.1 only says so here, and this package
		// does not argue.
		NextProtos: []string{"http/1.1"},
	}
	component := kit.MustNewHTTP("127.0.0.1:0", kit.WithTLSConfig(supplied))
	component.HandleFunc("GET /hello", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	if err := component.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer component.Shutdown(context.Background()) //nolint:errcheck

	client := &http.Client{Transport: &http.Transport{
		TLSClientConfig:   &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12},
		ForceAttemptHTTP2: true,
	}}
	resp, err := client.Get("https://" + component.Addr() + "/hello")
	if err != nil {
		t.Fatalf("GET over TLS: %v", err)
	}
	defer resp.Body.Close()
	if resp.Proto != "HTTP/1.1" {
		t.Errorf("proto = %s, want the HTTP/1.1 the deployment asked for", resp.Proto)
	}

	// Cloning means a later mutation cannot change what is already being served.
	supplied.NextProtos = []string{"h2"}
	resp2, err := client.Get("https://" + component.Addr() + "/hello")
	if err != nil {
		t.Fatalf("second GET: %v", err)
	}
	defer resp2.Body.Close()
	if resp2.Proto != "HTTP/1.1" {
		t.Errorf("proto = %s after mutating the caller's config, want HTTP/1.1", resp2.Proto)
	}
}

// TestTLSConfigKeepsAnExplicitMinVersion proves the one imposed value is imposed
// only when absent: a deployment that has to accept TLS 1.0 clients said so on
// purpose.
func TestTLSConfigKeepsAnExplicitMinVersion(t *testing.T) {
	if _, err := kit.NewHTTP("127.0.0.1:0", kit.WithTLSConfig(nil)); err == nil {
		t.Fatal("NewHTTP accepted a nil tls.Config")
	}

	certPath, keyPath, _ := selfSignedCert(t)
	pair, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		t.Fatalf("LoadX509KeyPair: %v", err)
	}
	component := kit.MustNewHTTP("127.0.0.1:0", kit.WithTLSConfig(&tls.Config{
		Certificates: []tls.Certificate{pair},
		MinVersion:   tls.VersionTLS13,
	}))
	if !component.ServesTLS() {
		t.Fatal("ServesTLS() = false after WithTLSConfig")
	}
	if err := component.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer component.Shutdown(context.Background()) //nolint:errcheck

	// A TLS 1.2-only client must be refused by a server told to require 1.3.
	client := &http.Client{Transport: &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true, MaxVersion: tls.VersionTLS12}, //nolint:gosec // the point is the version, not the identity
	}}
	if _, err := client.Get("https://" + component.Addr() + "/hello"); err == nil {
		t.Error("a TLS 1.2 client reached a server configured for 1.3 only")
	}
}

// TestNoTLSByDefault proves the default is unchanged: plaintext, and a component
// that says so.
func TestNoTLSByDefault(t *testing.T) {
	component := kit.MustNewHTTP("127.0.0.1:0")
	if component.ServesTLS() {
		t.Fatal("ServesTLS() = true without a TLS option")
	}
}
