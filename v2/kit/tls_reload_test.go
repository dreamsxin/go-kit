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
	"math/big"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/dreamsxin/go-kit/v2/kit"
)

// writeSelfSignedCert writes a certificate and key to the given paths and returns a
// pool that trusts it. The common name is the test's handle on which certificate a
// client was served.
func writeSelfSignedCert(t *testing.T, certPath, keyPath, commonName string) *x509.CertPool {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	template := x509.Certificate{
		SerialNumber:          big.NewInt(time.Now().UnixNano()),
		Subject:               pkix.Name{CommonName: commonName},
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
	if err := os.WriteFile(certPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o600); err != nil {
		t.Fatalf("write certificate: %v", err)
	}
	if err := os.WriteFile(keyPath, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}), 0o600); err != nil {
		t.Fatalf("write key: %v", err)
	}

	certificate, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("parse certificate: %v", err)
	}
	pool := x509.NewCertPool()
	pool.AddCert(certificate)
	return pool
}

// servedCommonName makes one request on a connection of its own — a pooled
// connection would answer with the certificate from its original handshake, which
// would hide a rotation rather than test it.
func servedCommonName(t *testing.T, addr string, pool *x509.CertPool) string {
	t.Helper()
	transport := &http.Transport{TLSClientConfig: &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport}
	resp, err := client.Get("https://" + addr + "/ping")
	if err != nil {
		t.Fatalf("GET /ping: %v", err)
	}
	defer resp.Body.Close()
	if resp.TLS == nil || len(resp.TLS.PeerCertificates) == 0 {
		t.Fatal("no peer certificate on the response")
	}
	return resp.TLS.PeerCertificates[0].Subject.CommonName
}

func startPingComponent(t *testing.T, options ...kit.Option) *kit.HTTP {
	t.Helper()
	component := kit.MustNewHTTP("127.0.0.1:0", options...)
	component.HandleFunc("GET /ping", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	if err := component.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = component.Shutdown(context.Background()) })
	return component
}

// TestCertificateFilesServesAReloadedCertificate is the milestone in one test: the
// files on disk are replaced while the listener is serving, the deployment says when
// to look again, and the next client gets the new certificate. No restart.
func TestCertificateFilesServesAReloadedCertificate(t *testing.T) {
	dir := t.TempDir()
	certPath := filepath.Join(dir, "tls.crt")
	keyPath := filepath.Join(dir, "tls.key")
	beforePool := writeSelfSignedCert(t, certPath, keyPath, "before-rotation")

	files, err := kit.NewCertificateFiles(certPath, keyPath)
	if err != nil {
		t.Fatalf("NewCertificateFiles: %v", err)
	}
	component := startPingComponent(t, kit.WithTLSCertificateSource(files))

	if got := servedCommonName(t, component.Addr(), beforePool); got != "before-rotation" {
		t.Fatalf("served %q before the rotation, want before-rotation", got)
	}

	afterPool := writeSelfSignedCert(t, certPath, keyPath, "after-rotation")
	if got := servedCommonName(t, component.Addr(), beforePool); got != "before-rotation" {
		t.Fatalf("served %q after replacing the files but before Reload; the source is reading the filesystem on the handshake path", got)
	}
	if err := files.Reload(); err != nil {
		t.Fatalf("Reload: %v", err)
	}
	if got := servedCommonName(t, component.Addr(), afterPool); got != "after-rotation" {
		t.Fatalf("served %q after the reload, want after-rotation", got)
	}
}

// TestCertificateSourceIsAskedForEveryHandshake pins what makes rotation possible at
// all: nothing about the certificate is captured when the component starts.
func TestCertificateSourceIsAskedForEveryHandshake(t *testing.T) {
	dir := t.TempDir()
	certPath := filepath.Join(dir, "tls.crt")
	keyPath := filepath.Join(dir, "tls.key")
	pool := writeSelfSignedCert(t, certPath, keyPath, "counted")

	loaded, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		t.Fatalf("load pair: %v", err)
	}
	var asked atomic.Int64
	source := kit.CertificateSourceFunc(func(*tls.ClientHelloInfo) (*tls.Certificate, error) {
		asked.Add(1)
		return &loaded, nil
	})
	component := startPingComponent(t, kit.WithTLSCertificateSource(source))

	for i := 0; i < 2; i++ {
		if got := servedCommonName(t, component.Addr(), pool); got != "counted" {
			t.Fatalf("served %q, want counted", got)
		}
	}
	if asked.Load() < 2 {
		t.Errorf("source asked %d times for 2 handshakes; the certificate is being cached by the server", asked.Load())
	}
}

// TestCertificateFilesKeepsTheOldCertificateWhenReloadFails is the ordering that
// matters in an incident: a half-written secret must not cost the listener its
// certificate.
func TestCertificateFilesKeepsTheOldCertificateWhenReloadFails(t *testing.T) {
	dir := t.TempDir()
	certPath := filepath.Join(dir, "tls.crt")
	keyPath := filepath.Join(dir, "tls.key")
	pool := writeSelfSignedCert(t, certPath, keyPath, "still-serving")

	files, err := kit.NewCertificateFiles(certPath, keyPath)
	if err != nil {
		t.Fatalf("NewCertificateFiles: %v", err)
	}
	component := startPingComponent(t, kit.WithTLSCertificateSource(files))

	if err := os.WriteFile(certPath, []byte("not a certificate"), 0o600); err != nil {
		t.Fatalf("write a broken certificate: %v", err)
	}
	reloadErr := files.Reload()
	if reloadErr == nil {
		t.Fatal("Reload = nil for an unparseable certificate")
	}
	if !strings.Contains(reloadErr.Error(), certPath) {
		t.Errorf("Reload error = %v, want it to name %s", reloadErr, certPath)
	}
	if got := servedCommonName(t, component.Addr(), pool); got != "still-serving" {
		t.Fatalf("served %q after a failed reload, want the certificate already loaded", got)
	}
}

func TestNewCertificateFilesFailsOnABadPath(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "absent.pem")
	_, err := kit.NewCertificateFiles(missing, missing)
	if err == nil {
		t.Fatal("NewCertificateFiles = nil error for a path that does not exist")
	}
	if !strings.Contains(err.Error(), missing) {
		t.Errorf("error = %v, want it to name %s", err, missing)
	}
	if _, err := kit.NewCertificateFiles("", ""); err == nil {
		t.Error("empty paths were accepted")
	}
}

func TestWithTLSCertificateSourceRejectsNil(t *testing.T) {
	if _, err := kit.NewHTTP("127.0.0.1:0", kit.WithTLSCertificateSource(nil)); err == nil {
		t.Fatal("a nil certificate source was accepted")
	}
}
