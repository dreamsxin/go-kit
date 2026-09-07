package kit

import (
	"crypto/tls"
	"fmt"
	"strings"
	"sync"
)

// Rotating a certificate.
//
// Milestone 13 loaded a certificate pair once, at construction, and PRODUCTION.md
// said the rest out loud: files are read once, so rotation means a restart. That is
// an honest sentence and a poor answer — a certificate that expires is a scheduled
// outage, and the platform that rotates it (a cert-manager secret, a Vault agent, an
// operator's script) replaces files while the process is running.
//
// What the framework owns here is the seam and nothing else. Where a certificate
// comes from is a deployment's decision, and so is when to look again: this package
// does not watch the filesystem, poll a timer, or install a signal handler, because
// each of those is a policy some deployment would have to work around.

// CertificateSource supplies the certificate for a handshake.
//
// Implement it to serve a certificate from wherever the deployment keeps one — a
// file, a secret manager, an ACME client, a per-name map for SNI. Certificate runs
// on the handshake path, so it must not block: a source that fetches should cache
// and refresh behind its own lock, the way CertificateFiles does.
//
// A nil hello is possible on some paths; a source that keys on the requested name
// must handle it rather than dereference it.
type CertificateSource interface {
	Certificate(hello *tls.ClientHelloInfo) (*tls.Certificate, error)
}

// WithTLSCertificateSource serves TLS, asking the source for the certificate on
// every handshake.
//
// This is the option for a deployment that rotates: nothing is captured at startup
// except the source itself, so replacing what the source returns changes what the
// next client is served. Everything else WithTLSConfig says still applies — the
// minimum version defaults to DefaultTLSMinVersion and nothing else is imposed.
//
// Stable: kit.tls-certificate-per-handshake — when a certificate source is configured, every handshake asks it, so a replaced certificate is served without restarting the process.
// Covered by: TestCertificateSourceIsAskedForEveryHandshake, TestCertificateFilesServesAReloadedCertificate
func WithTLSCertificateSource(source CertificateSource) Option {
	return func(h *HTTP) error {
		if source == nil {
			return fmt.Errorf("tls certificate source cannot be nil")
		}
		config := h.cloneTLSConfig()
		config.GetCertificate = func(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
			return source.Certificate(hello)
		}
		h.tlsConfig = config
		return nil
	}
}

// CertificateFiles is a CertificateSource backed by a certificate and key on disk,
// re-read when the deployment says so.
//
// The pair is loaded by NewCertificateFiles, so a bad path still stops startup with
// the path in the error rather than failing a client's handshake. Reload re-reads
// both files; the caller decides when — on SIGHUP, on a timer it owns, on a
// filesystem event from a library it chose. Choosing for it here would be the
// framework deciding how a deployment rotates.
type CertificateFiles struct {
	certPath string
	keyPath  string

	mu      sync.RWMutex
	current *tls.Certificate
}

// NewCertificateFiles loads a certificate and key and returns a source that can
// re-read them.
func NewCertificateFiles(certFile, keyFile string) (*CertificateFiles, error) {
	certPath := strings.TrimSpace(certFile)
	keyPath := strings.TrimSpace(keyFile)
	if certPath == "" || keyPath == "" {
		return nil, fmt.Errorf("kit: tls certificate and key paths are both required")
	}
	files := &CertificateFiles{certPath: certPath, keyPath: keyPath}
	if err := files.Reload(); err != nil {
		return nil, err
	}
	return files, nil
}

// Reload re-reads the pair from disk.
//
// A failure leaves the certificate already being served in place and returns the
// error. That ordering is the point: a half-written secret or a key that does not
// match its certificate should be a logged failure and a stale certificate, not a
// listener that stops answering.
//
// Stable: kit.tls-reload-keeps-serving — a failed certificate reload returns the error and keeps serving the pair already loaded, rather than leaving the listener without one.
// Covered by: TestCertificateFilesKeepsTheOldCertificateWhenReloadFails
func (f *CertificateFiles) Reload() error {
	certificate, err := tls.LoadX509KeyPair(f.certPath, f.keyPath)
	if err != nil {
		return fmt.Errorf("kit: reload tls certificate %s and key %s: %w", f.certPath, f.keyPath, err)
	}
	f.mu.Lock()
	f.current = &certificate
	f.mu.Unlock()
	return nil
}

// Certificate implements CertificateSource. It never touches the filesystem: the
// handshake path reads what the last successful Reload left behind.
func (f *CertificateFiles) Certificate(*tls.ClientHelloInfo) (*tls.Certificate, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	if f.current == nil {
		return nil, fmt.Errorf("kit: no tls certificate loaded from %s and %s", f.certPath, f.keyPath)
	}
	return f.current, nil
}

// Paths reports the certificate and key paths, which is what a log line about a
// reload needs to say.
func (f *CertificateFiles) Paths() (certFile, keyFile string) {
	return f.certPath, f.keyPath
}

var _ CertificateSource = (*CertificateFiles)(nil)

// CertificateSourceFunc adapts a function to CertificateSource, for a deployment
// whose lookup is a closure rather than a type.
type CertificateSourceFunc func(hello *tls.ClientHelloInfo) (*tls.Certificate, error)

// Certificate implements CertificateSource.
func (fn CertificateSourceFunc) Certificate(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
	if fn == nil {
		return nil, fmt.Errorf("kit: nil certificate source")
	}
	return fn(hello)
}

var _ CertificateSource = CertificateSourceFunc(nil)
