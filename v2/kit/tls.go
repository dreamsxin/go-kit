package kit

import (
	"crypto/tls"
	"fmt"
	"strings"
)

// Serving TLS.
//
// v2 served plaintext HTTP and nothing else for its first twelve releases, on the
// assumption that a sidecar, an ingress, or a load balancer terminates TLS. That is
// still the default and still a reasonable one — but it was never written down,
// which is how a service reaches production with an assumption in place of a
// decision.
//
// Terminating in the process is now an option. What stays outside it is policy:
// cipher suites, client authentication, certificate rotation, and which
// certificate answers which name are decisions with a compliance requirement
// behind them, so they belong to the deployment and not to this package. The only
// value imposed here is a minimum protocol version, and only when the deployment
// did not set one.

// What turning TLS on also changes is the protocol version. Go's server offers
// HTTP/2 through ALPN as soon as it has a TLS config, so a component that gains a
// certificate gains h2 with it. That is worth stating rather than discovering,
// because two things a service is likely to be doing behave differently under h2:
// a streaming response is framed by the stream layer instead of chunked transfer
// encoding, and an upgraded connection is not available at all — h2 has no
// 101 Switching Protocols, so a handler that hijacks the socket only works on the
// HTTP/1.1 path. Flushing keeps working, which is what SSE and the streaming MCP
// transport rely on; WebSocket over this listener does not, and needs either the
// plaintext path or an h2 extension this package does not implement.
//
// Cleartext HTTP/2 — h2c — is deliberately not offered. It cannot be negotiated
// without either a prior-knowledge client or an upgrade dance, both of which mean
// the deployment already knows what it is talking to; and the usual reason to want
// it, a proxy speaking h2 to the backend, is a decision for the proxy's own
// configuration. Serving it would add a protocol nothing asked for on the port
// every plaintext client already uses.

// DefaultTLSMinVersion is the lowest protocol version a Host serves when the
// deployment did not choose one. TLS 1.2 rather than 1.3 because a service that
// must accept older clients should have to say so explicitly, not discover the
// exclusion from a support ticket.
const DefaultTLSMinVersion = tls.VersionTLS12

// WithTLS serves TLS using a certificate and key from disk.
//
// The pair is loaded here, not at the first handshake: a path that is wrong, a key
// that does not match its certificate, or a file the process cannot read is a
// deployment mistake, and it should stop the process with the path in the error
// rather than become a client's TLS error to report.
//
// It configures nothing else. For a deployment that needs a cipher policy, client
// certificates, or SNI, use WithTLSConfig. For one that renews certificates while the
// process runs, use WithTLSCertificateSource: the pair loaded here is captured, so
// replacing the files underneath this option changes nothing until a restart.
//
// Stable: kit.tls-serves-when-configured — a component configured with a certificate serves TLS on its listener, and HTTP/2 is negotiated through ALPN.
// Covered by: TestTLSServesHTTPSAndNegotiatesHTTP2
//
// Stable: kit.tls-certificate-failure-is-immediate — an unloadable certificate fails construction with the offending path, not the first handshake.
// Covered by: TestTLSCertificateFailureNamesThePath
//
// Stable: kit.streaming-survives-http2 — a flushing handler still streams when the listener negotiated HTTP/2, so SSE and the streaming MCP transport work with TLS on.
// Covered by: TestSSEStillStreamsOverHTTP2
//
// Stable: kit.no-cleartext-http2 — a listener without TLS speaks HTTP/1.1; h2c is not negotiated, so hijack-based upgrades keep working there.
// Covered by: TestPlaintextListenerSpeaksHTTP11
func WithTLS(certFile, keyFile string) Option {
	return func(h *HTTP) error {
		certPath := strings.TrimSpace(certFile)
		keyPath := strings.TrimSpace(keyFile)
		if certPath == "" || keyPath == "" {
			return fmt.Errorf("tls certificate and key paths are both required")
		}
		certificate, err := tls.LoadX509KeyPair(certPath, keyPath)
		if err != nil {
			return fmt.Errorf("load tls certificate %s and key %s: %w", certPath, keyPath, err)
		}
		config := h.cloneTLSConfig()
		config.Certificates = append(config.Certificates, certificate)
		h.tlsConfig = config
		return nil
	}
}

// WithTLSConfig serves TLS using a configuration the deployment built.
//
// The config is used as given, with one exception: when MinVersion is zero it
// becomes DefaultTLSMinVersion, because Go's zero value there means TLS 1.0 for a
// server and no deployment means to ask for that. Everything else — cipher suites,
// ClientAuth, GetCertificate, NextProtos — is left exactly as passed, including the
// choices this package would not have made.
//
// The config is cloned, so a later mutation of the caller's value does not change
// what the server is already serving.
//
// Stable: kit.tls-policy-belongs-to-the-deployment — a supplied tls.Config is served as given except for a zero MinVersion, which becomes TLS 1.2.
// Covered by: TestTLSConfigIsServedAsGiven, TestTLSConfigKeepsAnExplicitMinVersion
func WithTLSConfig(config *tls.Config) Option {
	return func(h *HTTP) error {
		if config == nil {
			return fmt.Errorf("tls config cannot be nil")
		}
		cloned := config.Clone()
		if cloned.MinVersion == 0 {
			cloned.MinVersion = DefaultTLSMinVersion
		}
		h.tlsConfig = cloned
		return nil
	}
}

// cloneTLSConfig returns the config to extend: whatever a previous option
// installed, or a new one with the default minimum version.
func (h *HTTP) cloneTLSConfig() *tls.Config {
	if h.tlsConfig != nil {
		return h.tlsConfig.Clone()
	}
	return &tls.Config{MinVersion: DefaultTLSMinVersion}
}

// ServesTLS reports whether the component will serve TLS on its listener. It is
// what a readiness or startup log line needs to say "https" rather than guess.
func (h *HTTP) ServesTLS() bool {
	h.lifecycleMu.Lock()
	defer h.lifecycleMu.Unlock()
	return h.tlsConfig != nil
}
